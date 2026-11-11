package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/httpapi"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/store"
)

const (
	defaultSKU     = "CH-WINTER-PARKA-01"
	pingInterval   = 20 * time.Second
	idleTimeout    = 5 * time.Minute
	maxMessageSize = 4096
)

type InboundMessage struct {
	Event       string            `json:"event"`
	Seq         int               `json:"seq"`
	SKU         string            `json:"sku,omitempty"`
	Constraints *ConstraintsInput `json:"constraints,omitempty"`
	AgreementID string            `json:"agreement_id,omitempty"`
}

type ConstraintsInput struct {
	Budget              string  `json:"budget"`
	ShippingFlexibility float64 `json:"shipping_flexibility"`
	PrefersPayLater     bool    `json:"prefers_pay_later"`
	WillingToBundle     bool    `json:"willing_to_bundle"`
}

type OutboundError struct {
	Event   string `json:"event"`
	Seq     int    `json:"seq"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type OutboundSessionReady struct {
	Event         string `json:"event"`
	Seq           int    `json:"seq"`
	NegotiationID string `json:"negotiation_id"`
	StandardTotal string `json:"standard_total"`
}

type OutboundPreview struct {
	Event          string                      `json:"event"`
	Seq            int                         `json:"seq"`
	Feasible       bool                        `json:"feasible"`
	EstimatedPrice string                      `json:"estimated_price,omitempty"`
	Concessions    *negotiation.ConcessionsDTO `json:"concessions,omitempty"`
	Reason         string                      `json:"reason,omitempty"`
}

type OutboundTrace struct {
	Event string `json:"event"`
	Seq   int    `json:"seq"`
	*negotiation.TraceDTO
}

type Handler struct {
	service     *negotiation.Service
	rateLimiter *negotiation.RateLimiter
	store       *store.Store
}

func NewHandler(svc *negotiation.Service, rl *negotiation.RateLimiter, st *store.Store) *Handler {
	return &Handler{service: svc, rateLimiter: rl, store: st}
}

// session holds the per-connection negotiation state.
type session struct {
	h             *Handler
	c             *websocket.Conn
	id            string
	negotiationID string
	sku           string
	constraints   clearing.BuyerConstraints
}

// ServeHTTP upgrades the HTTP request to a WebSocket connection.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sid := httpapi.GetSessionID(r)
	if !httpapi.ValidSessionID(sid) {
		http.Error(w, "missing or invalid session cookie", http.StatusUnauthorized)
		return
	}

	// Default origin check (same host) is kept: no InsecureSkipVerify.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"nashpay-clearing-v1"},
	})
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}
	defer c.Close(websocket.StatusInternalError, "connection closed")
	c.SetReadLimit(maxMessageSize)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	s := &session{h: h, c: c, id: sid, sku: defaultSKU, negotiationID: httpapi.RandomToken("neg_", 8)}
	go s.keepAlive(ctx)
	s.loop(ctx)
}

// keepAlive pings the peer every 20 s; a failed ping tears the connection down.
func (s *session) keepAlive(ctx context.Context) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := s.c.Ping(pctx)
			cancel()
			if err != nil {
				_ = s.c.Close(websocket.StatusGoingAway, "ping timeout")
				return
			}
		}
	}
}

func (s *session) loop(ctx context.Context) {
	for {
		rctx, cancel := context.WithTimeout(ctx, idleTimeout)
		_, data, err := s.c.Read(rctx)
		cancel()
		if err != nil {
			slog.Debug("websocket read ended", "error", err)
			return
		}

		var in InboundMessage
		if err := json.Unmarshal(data, &in); err != nil {
			_ = s.sendError(ctx, 0, "INVALID", "malformed JSON message")
			continue
		}
		s.dispatch(ctx, in)
	}
}

func (s *session) dispatch(ctx context.Context, in InboundMessage) {
	switch in.Event {
	case "CLIENT_INITIATE":
		s.onInitiate(ctx, in)
	case "CONSTRAINTS_UPDATE":
		s.onUpdate(ctx, in)
	case "COMMIT":
		s.onCommit(ctx, in)
	case "REQUOTE_WITHOUT_PAY_LATER":
		s.onRequote(ctx, in)
	default:
		_ = s.sendError(ctx, in.Seq, "INVALID", "unsupported event: "+in.Event)
	}
}

func (s *session) onInitiate(ctx context.Context, in InboundMessage) {
	if in.SKU != "" {
		s.sku = in.SKU
	}
	s.constraints = clearing.BuyerConstraints{Budget: 10500, ShippingFlexibility: 0.85, PrefersPayLater: true, WillingToBundle: true}
	if in.Constraints != nil {
		parsed, err := parseConstraints(in.Constraints)
		if err != nil {
			_ = s.sendError(ctx, in.Seq, "INVALID", err.Error())
			return
		}
		s.constraints = parsed
	}

	prod, err := s.h.store.GetProduct(ctx, s.sku)
	mkt, mErr := s.h.store.GetMarketSettings(ctx)
	if err != nil || mErr != nil {
		_ = s.sendError(ctx, in.Seq, "INVALID", "unknown product")
		return
	}
	std := money.Cents(prod.ListPriceCents) + money.Cents(mkt.StandardShippingChargeCents)
	_ = wsjson.Write(ctx, s.c, OutboundSessionReady{Event: "SESSION_READY", Seq: in.Seq, NegotiationID: s.negotiationID, StandardTotal: std.Format()})
	s.sendPreview(ctx, in.Seq)
}

func (s *session) onUpdate(ctx context.Context, in InboundMessage) {
	if !s.h.rateLimiter.AllowPreview(s.id) {
		_ = s.sendError(ctx, in.Seq, "RATE_LIMITED", "too many preview updates, please slow down")
		return
	}
	if in.Constraints == nil {
		_ = s.sendError(ctx, in.Seq, "INVALID", "missing constraints in update")
		return
	}
	parsed, err := parseConstraints(in.Constraints)
	if err != nil {
		_ = s.sendError(ctx, in.Seq, "INVALID", err.Error())
		return
	}
	s.constraints = parsed
	s.sendPreview(ctx, in.Seq)
}

func (s *session) sendPreview(ctx context.Context, seq int) {
	p, err := s.h.service.Preview(ctx, s.sku, s.constraints, false)
	if err != nil {
		_ = s.sendError(ctx, seq, "INTERNAL", "preview unavailable")
		return
	}
	_ = wsjson.Write(ctx, s.c, OutboundPreview{
		Event: "PREVIEW", Seq: seq, Feasible: p.Feasible,
		EstimatedPrice: p.EstimatedPrice, Concessions: p.Concessions, Reason: p.Reason,
	})
}

func (s *session) onCommit(ctx context.Context, in InboundMessage) {
	if !s.h.rateLimiter.AllowCommit(s.id) {
		_ = s.sendError(ctx, in.Seq, "RATE_LIMITED", "please wait a few seconds before negotiating again")
		return
	}
	resp, ok := s.commit(ctx, in.Seq, false)
	if !ok {
		return
	}
	s.h.rateLimiter.MarkCommit(s.id)

	if resp.Trace != nil {
		_ = wsjson.Write(ctx, s.c, OutboundTrace{Event: "SOLVER_TRACE", Seq: in.Seq, TraceDTO: resp.Trace})
	}
	_ = wsjson.Write(ctx, s.c, resp)
}

// onRequote handles Pay Later ineligibility: the previous agreement is expired with a reason
// and a fresh one is solved without Pay Later. It does not consume the commit cooldown.
func (s *session) onRequote(ctx context.Context, in InboundMessage) {
	if in.AgreementID == "" {
		_ = s.sendError(ctx, in.Seq, "INVALID", "agreement_id is required")
		return
	}
	if err := s.h.service.ExpireAgreement(ctx, s.id, in.AgreementID, "PAY_LATER_INELIGIBLE"); err != nil {
		_ = s.sendError(ctx, in.Seq, "INVALID", "agreement cannot be requoted")
		return
	}
	resp, ok := s.commit(ctx, in.Seq, true)
	if !ok {
		return
	}
	_ = wsjson.Write(ctx, s.c, resp)
}

func (s *session) commit(ctx context.Context, seq int, forbidPayLater bool) (*negotiation.CommitResult, bool) {
	resp, err := s.h.service.Commit(ctx, s.id, s.negotiationID, s.sku, s.constraints, forbidPayLater)
	switch {
	case err == nil:
		resp.Seq = seq
		return resp, true
	case errors.Is(err, clearing.ErrNoZOPA):
		_ = s.sendError(ctx, seq, "NO_ZOPA", "no zone of possible agreement for these constraints")
	case errors.Is(err, negotiation.ErrLiveAgreement):
		_ = s.sendError(ctx, seq, "RATE_LIMITED", err.Error())
	default:
		slog.Error("commit failed", "error", err)
		_ = s.sendError(ctx, seq, "INTERNAL", "could not complete negotiation")
	}
	return nil, false
}

func (s *session) sendError(ctx context.Context, seq int, code, msg string) error {
	return wsjson.Write(ctx, s.c, OutboundError{Event: "ERROR", Seq: seq, Code: code, Message: msg})
}

func parseConstraints(ci *ConstraintsInput) (clearing.BuyerConstraints, error) {
	budget, err := money.Parse(ci.Budget)
	if err != nil {
		return clearing.BuyerConstraints{}, errors.New("invalid budget format")
	}
	if budget < 100 || budget > 1000000 { // 1.00 to 10,000.00 USD
		return clearing.BuyerConstraints{}, errors.New("budget must be between 1.00 and 10000.00 USD")
	}

	flex := ci.ShippingFlexibility
	if flex < 0.0 {
		flex = 0.0
	}
	if flex > 1.0 {
		flex = 1.0
	}

	return clearing.BuyerConstraints{
		Budget:              budget,
		ShippingFlexibility: flex,
		PrefersPayLater:     ci.PrefersPayLater,
		WillingToBundle:     ci.WillingToBundle,
	}, nil
}
