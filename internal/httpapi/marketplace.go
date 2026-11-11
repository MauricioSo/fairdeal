package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nashpay/nashpay/internal/ai"
	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/config"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/paypal"
	"github.com/nashpay/nashpay/internal/store"
)

// Each merchant has a separate SQLite database, session store and signing key.
// The coordinator is trusted with buyer constraints; merchant handlers never are.
type MerchantDefinition struct {
	ID                                                                                string  `json:"id"`
	Name                                                                              string  `json:"name"`
	Incentive                                                                         string  `json:"incentive"`
	List, Cost, Stock, AccessoryCost, AccessoryValue, AccessoryStock, Express, Ground int64   `json:"-"`
	Margin, Discount                                                                  float64 `json:"-"`
	Accessory                                                                         string  `json:"-"`
}

func deliveryRegions(id string) []string {
	if id == "glacier" || id == "polaris" {
		return []string{"US"}
	}
	return []string{"US", "CL"}
}
func canDeliver(id, country string) bool {
	for _, region := range deliveryRegions(id) {
		if region == country {
			return true
		}
	}
	return false
}

var DemoMerchants = []MerchantDefinition{
	{"alpine", "Alpine Gear Co.", "Accessory surplus", 12000, 5000, 40, 900, 2500, 500, 1500, 400, .20, .30, "Cashmere Scarf"},
	{"nordic", "Nordic Peak Outfitters", "Prepaid express capacity", 12500, 5800, 30, 600, 2000, 80, 300, 500, .22, .25, "Thermal Pack"},
	{"glacier", "Glacier Basecamp Direct", "Low cash price", 11000, 5200, 60, 0, 0, 0, 1000, 400, .20, .30, ""},
	{"sierra", "Sierra Eco-Apparel", "Ground shipping and accessory turnover", 13000, 6500, 25, 1000, 2800, 70, 1200, 500, .25, .20, "Winter Beanie"},
	{"polaris", "Polaris Urban Techwear", "Payment flexibility", 15000, 7200, 35, 800, 2400, 90, 1500, 500, .22, .30, "Winter Gloves"},
}

type MerchantCredential struct {
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
}
type marketTenant struct {
	definition MerchantDefinition
	server     *Server
}
type Marketplace struct {
	client    paypal.Client
	WebhookID string
	llmWork   chan struct{}
	tenants   []marketTenant
	limits    *requestLimits
}

// NewMarketplace preserves Alpine's existing database. Other tenants are sibling
// files on the same persistent disk; credentials contain verifiers, never passwords.
func NewMarketplace(primary *Server, cfg *config.Config, client paypal.Client, ai *AIHandler, baseURL, credentialsJSON string) (*Marketplace, error) {
	credentials := map[string]MerchantCredential{}
	if credentialsJSON != "" {
		if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
			return nil, errors.New("invalid merchant credentials configuration")
		}
	}
	if c, ok := credentials["alpine"]; ok {
		*primary.MchtHandler.auth = *NewMerchantAuth(primary.Store, c.Email, c.PasswordHash, baseURL)
		cfg.MerchantEmail = c.Email
	}
	m := &Marketplace{limits: newRequestLimits(), client: client, llmWork: make(chan struct{}, 5)}
	seen := map[string]bool{}
	if cfg.MerchantEmail != "" {
		seen[strings.ToLower(cfg.MerchantEmail)] = true
	}
	for _, d := range DemoMerchants {
		if d.ID == "alpine" {
			if err := primary.Store.BindMerchant(context.Background(), d.ID); err != nil {
				return nil, err
			}
			primary.MchtHandler.auth.MerchantName = d.Name
			m.tenants = append(m.tenants, marketTenant{d, primary})
			continue
		}
		c := credentials[d.ID]
		if c.Email != "" {
			key := strings.ToLower(strings.TrimSpace(c.Email))
			if seen[key] {
				m.Close()
				return nil, errors.New("duplicate merchant email")
			}
			seen[key] = true
		}
		path := filepath.Join(filepath.Dir(cfg.DBPath), "fairdeal-"+d.ID+".db")
		st, err := store.Open(context.Background(), path)
		if err != nil {
			m.Close()
			return nil, err
		}
		if err = st.ConfigureDemoMerchant(context.Background(), d.ID, d.List, d.Cost, d.Stock, d.Accessory, d.AccessoryCost, d.AccessoryValue, d.AccessoryStock, d.Express, d.Ground, d.Margin, d.Discount); err != nil {
			st.Close()
			m.Close()
			return nil, err
		}
		if err := st.BindMerchant(context.Background(), d.ID); err != nil {
			st.Close()
			m.Close()
			return nil, err
		}
		mac := hmac.New(sha256.New, cfg.HMACSecret)
		mac.Write([]byte("FairDeal/merchant/v2/" + d.ID))
		svc := negotiation.NewService(st, mac.Sum(nil), 300)
		pp := NewPayPalHandler(st, client, svc, cfg.PayPalMode, baseURL)
		auth := NewMerchantAuth(st, c.Email, c.PasswordHash, baseURL)
		auth.MerchantName = d.Name
		merchant := NewMerchantHandler(st, ai.llmClient, false, auth)
		server := NewServer(st, cfg, nil, pp, ai, merchant)
		m.tenants = append(m.tenants, marketTenant{d, server})
	}
	return m, nil
}
func (m *Marketplace) Close() {
	for _, t := range m.tenants {
		if t.definition.ID != "alpine" {
			t.server.Store.Close()
		}
	}
}
func (m *Marketplace) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p := r.URL.Path
	if r.Method == http.MethodPost {
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(10 * time.Second))
		defer controller.SetReadDeadline(time.Time{})
	}
	if p == "/api/buyer/summary" && r.Method == "GET" {
		m.buyerSummary(w, r)
		return
	}
	if p == "/api/paypal/webhook" && r.Method == "POST" {
		if !m.limits.allow(r) {
			writeJSONError(w, 429, "RATE_LIMITED", "retry later")
			return
		}
		m.webhook(w, r)
		return
	}
	if p == "/api/merchants" && r.Method == "GET" {
		writeJSON(w, 200, map[string]any{"merchants": DemoMerchants, "simulation": true})
		return
	}
	if r.Method == "POST" && (strings.HasPrefix(p, "/api/") || p == "/v1/clearing/negotiate") {
		select {
		case m.limits.active <- struct{}{}:
			defer func() { <-m.limits.active }()
		default:
			writeJSONError(w, 429, "BUSY", "please retry shortly")
			return
		}
		if !m.limits.allow(r) {
			writeJSONError(w, 429, "RATE_LIMITED", "request limit reached; try later")
			return
		}
	}
	if r.Method == "POST" && (p == "/api/fairdeal/match" || p == "/api/wasabi/match" || p == "/api/pact/match" || p == "/api/winwin/match") {
		m.match(w, r)
		return
	}
	if p == "/api/merchant/login" && r.Method == "POST" {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil {
			writeJSONError(w, 400, "INVALID", "invalid login")
			return
		}
		var c struct {
			Email string `json:"email"`
		}
		_ = json.Unmarshal(body, &c)
		for _, t := range m.tenants {
			if strings.EqualFold(strings.TrimSpace(c.Email), t.server.MchtHandler.auth.email) {
				r.Body = io.NopCloser(bytes.NewReader(body))
				t.server.Mux.ServeHTTP(w, r)
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		m.tenants[0].server.Mux.ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(p, "/api/merchant/") || p == "/merchant" || p == "/merchant.html" {
		for _, t := range m.tenants {
			if _, err := t.server.MchtHandler.auth.session(r); err == nil {
				t.server.Mux.ServeHTTP(w, r)
				return
			}
		}
	}
	// Resolve payment and receipt ownership from persisted IDs, never a supplied tenant ID.
	if strings.HasPrefix(p, "/api/paypal/orders") || strings.HasPrefix(p, "/api/agreements/") {
		var id string
		if p == "/api/paypal/orders" && r.Method == "POST" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
			if err != nil {
				writeJSONError(w, 400, "INVALID", "invalid order")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var a CreateOrderRequest
			_ = json.Unmarshal(body, &a)
			id = a.AgreementID
		} else {
			parts := strings.Split(strings.Trim(p, "/"), "/")
			if len(parts) > 3 && parts[1] == "paypal" {
				id = parts[3]
			} else if len(parts) > 2 {
				id = parts[2]
			}
		}
		for _, t := range m.tenants {
			var err error
			if strings.HasPrefix(p, "/api/paypal/orders/") {
				_, err = t.server.Store.GetAgreementByPayPalOrderID(r.Context(), id)
			} else {
				_, err = t.server.Store.GetAgreement(r.Context(), id)
			}
			if err == nil {
				t.server.Mux.ServeHTTP(w, r)
				return
			}
		}
		writeJSONError(w, 404, "NOT_FOUND", "agreement not found")
		return
	}
	m.tenants[0].server.Mux.ServeHTTP(w, r)
}
func (m *Marketplace) match(w http.ResponseWriter, r *http.Request) {
	var req WinWinMatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	budget, err := money.Parse(req.Budget)
	if err != nil || budget < 100 || budget > 1000000 || req.ShippingFlexibility < 0 || req.ShippingFlexibility > 1 {
		writeJSONError(w, 400, "INVALID", "invalid budget or shipping preference")
		return
	}
	req.DeliveryCountry = strings.ToUpper(strings.TrimSpace(req.DeliveryCountry))
	if req.DeliveryCountry == "" {
		req.DeliveryCountry = "US"
	}
	if len(req.DeliveryCountry) != 2 {
		writeJSONError(w, 400, "INVALID", "choose a two-letter delivery country")
		return
	}
	if req.SKU == "" {
		req.SKU = "CH-WINTER-PARKA-01"
	}
	if req.SKU != "CH-WINTER-PARKA-01" {
		writeJSONError(w, 422, "UNSUPPORTED_PRODUCT", "confirm a compatible winter-parka demo product")
		return
	}
	ctx, cancel := context.WithTimeout(negotiation.WithPrivateReference(r.Context()), 20*time.Second)
	defer cancel()
	sid := GetOrCreateSessionID(w, r)
	type result struct {
		response WinWinMatchResponse
		rounds   int
		source   string
	}
	results := make(chan result, len(m.tenants))
	for _, tenant := range m.tenants {
		go func(t marketTenant) {
			if !canDeliver(t.definition.ID, req.DeliveryCountry) {
				results <- result{}
				return
			}
			policy, e := t.server.Store.GetActivePolicy(ctx)
			if e != nil {
				results <- result{}
				return
			}
			candidates := []WinWinMatchRequest{req}
			source := "deterministic"
			rounds := 0
			var previous *ai.SellerProposal
			for round := 1; round <= 2; round++ {
				if ctx.Err() != nil {
					break
				}
				change := "Propose your initial permitted package"
				if previous != nil {
					change = "Compare the alternative accessory configuration while keeping buyer permissions and your policy"
				}
				proposal := m.proposeSeller(ctx, t.server.AIHandler.llmClient, ai.SellerRequest{Merchant: t.definition.Name, Product: "Winter parka, demo canonical variant", AcceptsAccessory: req.WillingToBundle, ExploresPayLater: req.PrefersPayLater, ShippingFlexibility: req.ShippingFlexibility, Policy: policy.Policy, Round: round, Previous: previous, RequestedChange: change})
				previousCopy := proposal
				previous = &previousCopy
				candidate := req
				candidate.WillingToBundle = proposal.IncludeAccessory
				candidate.PrefersPayLater = proposal.ExplorePayLater
				candidates = append(candidates, candidate)
				rounds++
				eventKind := "SELLER_RULE_PROPOSAL"
				if proposal.Source == "llm" {
					eventKind = "SELLER_LLM_PROPOSAL"
				}
				_ = t.server.Store.RecordEvent(ctx, eventKind)
				if proposal.Source == "llm" {
					source = "llm"
				} else {
					break
				}
			}
			best := req
			var bestPrice money.Cents
			bestObjective := math.Inf(-1)
			for _, candidate := range candidates {
				input, _, e := t.server.PPHandler.negService.BuildSolverInput(ctx, req.SKU, clearing.BuyerConstraints{Budget: budget, ShippingFlexibility: candidate.ShippingFlexibility, WillingToBundle: candidate.WillingToBundle, PrefersPayLater: candidate.PrefersPayLater}, false)
				if e != nil {
					continue
				}
				// Preauthorized public concession tiers are independent of the private budget.
				floor := money.Cents((int64(input.Product.ListPriceCents)*(10000-int64(math.Round(input.Policy.MaxDiscountPct*10000))) + 9999) / 10000)
				caps := []money.Cents{input.Product.ListPriceCents, input.Product.ListPriceCents * 90 / 100, input.Product.ListPriceCents * 80 / 100, floor}
				seen := map[money.Cents]bool{}
				for _, cap := range caps {
					if cap < floor || seen[cap] {
						continue
					}
					seen[cap] = true
					input.Buyer.Budget = cap
					solved, e := clearing.Solve(*input)
					if e != nil || solved.PriceCents > budget {
						continue
					}
					objective := input.Policy.BuyerPower*math.Log(float64(solved.BuyerSurplusCents)) + (1-input.Policy.BuyerPower)*math.Log(float64(solved.SellerSurplusCents))
					if objective > bestObjective+1e-12 {
						bestObjective = objective
						bestPrice = solved.PriceCents
						best = candidate
						best.Budget = cap.Format()
					}
				}

			}
			if bestPrice == 0 {
				results <- result{}
				return
			}
			body, _ := json.Marshal(best)
			clone := r.Clone(negotiation.WithPrivateMaximum(ctx, budget))
			clone.Body = io.NopCloser(bytes.NewReader(body))
			clone.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sid})
			rec := httptest.NewRecorder()
			t.server.PPHandler.handleWinWinMatch(rec, clone)
			var response WinWinMatchResponse
			if rec.Code == 200 {
				_ = json.Unmarshal(rec.Body.Bytes(), &response)
			}
			for i := range response.TopOffers {
				response.TopOffers[i].MerchantName = t.definition.Name + " (Fictional demo merchant)"
				response.TopOffers[i].Badge = "Validated · " + source
			}
			results <- result{response, rounds, source}
		}(tenant)
	}
	trace := []ai.DialogueTurn{}
	var out WinWinMatchResponse
	out.AllOffers = []WinWinOfferDTO{}
	for range m.tenants {
		select {
		case result := <-results:
			if len(result.response.TopOffers) > 0 {
				trace = append(trace, ai.DialogueTurn{Turn: len(trace) + 1, Speaker: result.response.TopOffers[0].MerchantName, Role: "seller", Message: fmt.Sprintf("Evaluated initial package and %d bounded concession revision(s), source: %s. No private budget sent to seller agent.", result.rounds, result.source)})
			}
			if len(result.response.TopOffers) == 0 {
				continue
			}
			if out.SKU == "" {
				out = result.response
				out.AllOffers = nil
			}
			out.AllOffers = append(out.AllOffers, result.response.TopOffers...)
		case <-ctx.Done():
			goto complete
		}
	}
complete:
	if len(out.AllOffers) == 0 {
		writeJSONError(w, 422, "NO_ZOPA", "no merchant has a feasible package within the negotiation deadline")
		return
	}
	sort.SliceStable(out.AllOffers, func(i, j int) bool {
		a, b := numericPrice(out.AllOffers[i].FinalPrice), numericPrice(out.AllOffers[j].FinalPrice)
		if a == b {
			return out.AllOffers[i].MerchantName < out.AllOffers[j].MerchantName
		}
		return a < b
	})
	out.TopOffers = out.AllOffers
	out.TotalEvaluated = len(m.tenants)
	out.NegotiationLog = &ai.AgentNegotiationResult{Dialogue: trace, AgreedPrice: out.TopOffers[0].FinalPrice, ModelUsed: "Five isolated seller contexts · deterministic validation", SummaryReason: "Actual evaluation trace. Offers sorted by total price; compare the included conditions."}
	out.PrivacyGuarantee = "FairDeal processes your private budget. Sellers and seller agents never receive it. Five fictional merchants. Offers sorted by total price; compare shipping and accessories before selecting."
	writeJSON(w, 200, out)
}
func numericPrice(s string) money.Cents { v, _ := money.Parse(s); return v }

type requestLimits struct {
	mu     sync.Mutex
	window time.Time
	count  int
	active chan struct{}
}

func newRequestLimits() *requestLimits {
	return &requestLimits{window: time.Now(), active: make(chan struct{}, 8)}
}
func (l *requestLimits) allow(r *http.Request) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Since(l.window) >= time.Minute {
		l.count = 0
		l.window = time.Now()
	}
	l.count++
	return l.count <= 120
}

func (m *Marketplace) proposeSeller(ctx context.Context, client *ai.LLMClient, request ai.SellerRequest) ai.SellerProposal {
	select {
	case m.llmWork <- struct{}{}:
		callContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		defer func() { <-m.llmWork }()
		return ai.ProposeSeller(callContext, client, request)
	default:
		return ai.ProposeSeller(ctx, nil, request)
	}
}
