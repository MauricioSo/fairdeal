package negotiation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/store"
)

var (
	ErrProductNotFound = errors.New("product not found")
	ErrPolicyNotFound  = errors.New("active policy not found")
	ErrLiveAgreement   = errors.New("a checkout for this product is already in progress")
)

type InstallmentsDTO struct {
	Count  int    `json:"count"`
	Amount string `json:"amount"`
}

type BundleItemDTO struct {
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	RetailValue string `json:"retail_value"`
}

type ConcessionsDTO struct {
	ShippingTier   string           `json:"shipping_tier"`
	ShippingDays   string           `json:"shipping_days"`
	ShippingCharge string           `json:"shipping_charge"`
	PaymentMethod  string           `json:"payment_method"`
	Installments   *InstallmentsDTO `json:"installments"`
	Bundle         *BundleItemDTO   `json:"bundle"`
}

type AgreementDetailsDTO struct {
	FinalPrice        string         `json:"final_price"`
	Currency          string         `json:"currency"`
	StandardTotal     string         `json:"standard_total"`
	SavingsVsStandard string         `json:"savings_vs_standard"`
	Concessions       ConcessionsDTO `json:"concessions"`
}

type NashAgreementResponseDTO struct {
	Event           string              `json:"event"`
	Seq             int                 `json:"seq"`
	NegotiationID   string              `json:"negotiation_id"`
	AgreementID     string              `json:"agreement_id"`
	Status          string              `json:"status"`
	SolverLatencyUs int64               `json:"solver_latency_us"`
	Agreement       AgreementDetailsDTO `json:"agreement"`
	ClearingToken   string              `json:"clearing_token"`
	ExpiresAt       int64               `json:"expires_at"`
}

// CommitResult bundles the public agreement with its already-normalised solver trace.
// The trace never carries dollar amounts (BR-20): see NormalizeTrace.
type CommitResult struct {
	*NashAgreementResponseDTO
	Trace *TraceDTO `json:"-"`
}

type PreviewDTO struct {
	Feasible       bool            `json:"feasible"`
	EstimatedPrice string          `json:"estimated_price,omitempty"`
	Concessions    *ConcessionsDTO `json:"concessions,omitempty"`
	Reason         string          `json:"reason,omitempty"`
}

type Service struct {
	store      *store.Store
	hmacSecret []byte
	ttlSeconds int64
}

func NewService(s *store.Store, hmacSecret []byte, ttlSeconds int64) *Service {
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return &Service{
		store:      s,
		hmacSecret: hmacSecret,
		ttlSeconds: ttlSeconds,
	}
}

func randomHex(prefix string, bytesLen int) string {
	b := make([]byte, bytesLen)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// BuildSolverInput prepares the pure domain input for the clearing solver.
func (s *Service) BuildSolverInput(ctx context.Context, sku string, buyer clearing.BuyerConstraints, forbidPayLater bool) (*clearing.Input, *store.MerchantPolicy, error) {
	prod, err := s.store.GetProduct(ctx, sku)
	if err != nil {
		return nil, nil, ErrProductNotFound
	}

	mkt, err := s.store.GetMarketSettings(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load market settings: %w", err)
	}

	pol, err := s.store.GetActivePolicy(ctx)
	if err != nil {
		return nil, nil, ErrPolicyNotFound
	}

	var bundleProd *clearing.Product
	if pol.Policy.BundleSKU != nil && *pol.Policy.BundleSKU != "" {
		bp, err := s.store.GetProduct(ctx, *pol.Policy.BundleSKU)
		if err == nil && bp != nil && bp.IsDeadstock {
			bundleProd = &clearing.Product{
				SKU:              bp.SKU,
				ListPriceCents:   money.Cents(bp.ListPriceCents),
				CostCents:        money.Cents(bp.CostCents),
				RetailValueCents: money.Cents(bp.RetailValueCents),
				HoldingCostCents: money.Cents(bp.HoldingCostCents),
			}
		}
	}

	in := &clearing.Input{
		Product: clearing.Product{
			SKU:              prod.SKU,
			ListPriceCents:   money.Cents(prod.ListPriceCents),
			CostCents:        money.Cents(prod.CostCents),
			RetailValueCents: money.Cents(prod.RetailValueCents),
			HoldingCostCents: money.Cents(prod.HoldingCostCents),
		},
		Bundle: bundleProd,
		Market: clearing.Market{
			ExpressShipCostCents:        money.Cents(mkt.ExpressShipCostCents),
			GroundShipCostCents:         money.Cents(mkt.GroundShipCostCents),
			StandardShippingChargeCents: money.Cents(mkt.StandardShippingChargeCents),
			WaitCostMaxCents:            money.Cents(mkt.WaitCostMaxCents),
			PayLaterBuyerValueCents:     money.Cents(mkt.PayLaterBuyerValueCents),
			BundleValuePct:              mkt.BundleValuePct,
			FeeBps:                      mkt.FeeBps,
			FeeFixedCents:               money.Cents(mkt.FeeFixedCents),
			PayLaterMinCents:            money.Cents(mkt.PayLaterMinCents),
			PayLaterMaxCents:            money.Cents(mkt.PayLaterMaxCents),
		},
		Policy: clearing.Policy{
			MinMarginPct:      pol.Policy.MinMarginPct,
			MaxDiscountPct:    pol.Policy.MaxDiscountPct,
			BuyerPower:        pol.Policy.BuyerPower,
			AllowSlowShipping: pol.Policy.AllowSlowShipping,
			PayLaterEnabled:   pol.Policy.PayLaterEnabled,
		},
		Buyer:          buyer,
		ForbidPayLater: forbidPayLater,
	}

	return in, pol, nil
}

// Preview computes a non-binding prospective agreement for real-time slider feedback.
func (s *Service) Preview(ctx context.Context, sku string, buyer clearing.BuyerConstraints, forbidPayLater bool) (*PreviewDTO, error) {
	in, _, err := s.BuildSolverInput(ctx, sku, buyer, forbidPayLater)
	if err != nil {
		return nil, err
	}

	res, err := clearing.Solve(*in)
	if err != nil {
		if errors.Is(err, clearing.ErrNoZOPA) {
			return &PreviewDTO{Feasible: false, Reason: "NO_ZOPA"}, nil
		}
		return nil, err
	}

	concessions := s.buildConcessions(ctx, res, in.Bundle)

	return &PreviewDTO{
		Feasible:       true,
		EstimatedPrice: res.PriceCents.Format(),
		Concessions:    &concessions,
	}, nil
}

// Commit runs the solver, measures solver latency, stores the OFFERED record, and signs the clearing token.
// At most one live agreement exists per (session, sku): a previous OFFERED one is superseded, while an
// in-flight ORDER_CREATED one blocks the new commit with ErrLiveAgreement.
func (s *Service) Commit(ctx context.Context, sessionID, negotiationID, sku string, buyer clearing.BuyerConstraints, forbidPayLater bool) (*CommitResult, error) {
	in, pol, err := s.BuildSolverInput(ctx, sku, buyer, forbidPayLater)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	res, err := clearing.Solve(*in)
	latencyUs := time.Since(start).Microseconds()
	if latencyUs == 0 {
		latencyUs = 1
	}
	if err != nil {
		return nil, err
	}

	if err := s.supersedeLive(ctx, sessionID, sku); err != nil {
		return nil, err
	}

	agreementID := randomHex("agr_", 10)
	if negotiationID == "" {
		negotiationID = randomHex("neg_", 8)
	}
	now := store.NowUnix()
	expiresAt := now + s.ttlSeconds

	var bundleSKU *string
	if res.Bundle && in.Bundle != nil {
		bundleSKU = &in.Bundle.SKU
	}

	rec := &store.AgreementRecord{
		ID:                 agreementID,
		NegotiationID:      negotiationID,
		SessionID:          sessionID,
		SKU:                sku,
		BundleSKU:          bundleSKU,
		PriceCents:         int64(res.PriceCents),
		ShippingTier:       res.Shipping,
		PayLater:           res.PayLater,
		BuyerBudgetCents:   int64(buyer.Budget),
		StandardTotalCents: int64(res.StandardTotalCents),
		FeeCents:           int64(res.FeeCents),
		ContributionCents:  int64(res.ContributionCents),
		BuyerSurplusCents:  int64(res.BuyerSurplusCents),
		SellerSurplusCents: int64(res.SellerSurplusCents),
		PolicyID:           pol.ID,
		SolverLatencyUs:    latencyUs,
		ExpiresAt:          expiresAt,
	}

	if err := s.store.CreateAgreement(ctx, rec); err != nil {
		return nil, fmt.Errorf("persist agreement: %w", err)
	}

	// Sign HMAC clearing token
	tokenPayload := clearing.AgreementTokenPayload{
		AgreementID:   agreementID,
		PriceCents:    int64(res.PriceCents),
		Shipping:      res.Shipping,
		PayLater:      res.PayLater,
		BundleSKU:     bundleSKU,
		ExpiresAtUnix: expiresAt,
	}
	clearingToken := clearing.Sign(tokenPayload, s.hmacSecret)

	savings := res.StandardTotalCents - res.PriceCents
	if savings < 0 {
		savings = 0
	}

	concessions := s.buildConcessions(ctx, res, in.Bundle)

	dto := &NashAgreementResponseDTO{
		Event:           "NASH_AGREEMENT",
		NegotiationID:   negotiationID,
		AgreementID:     agreementID,
		Status:          store.StatusOffered,
		SolverLatencyUs: latencyUs,
		Agreement: AgreementDetailsDTO{
			FinalPrice:        res.PriceCents.Format(),
			Currency:          "USD",
			StandardTotal:     res.StandardTotalCents.Format(),
			SavingsVsStandard: savings.Format(),
			Concessions:       concessions,
		},
		ClearingToken: clearingToken,
		ExpiresAt:     expiresAt,
	}

	return &CommitResult{
		NashAgreementResponseDTO: dto,
		Trace:                    NormalizeTrace(res.Trace),
	}, nil
}

// supersedeLive enforces "at most one live agreement per (session, sku)".
// A previous OFFERED agreement is expired as SUPERSEDED; an ORDER_CREATED one blocks the commit.
func (s *Service) supersedeLive(ctx context.Context, sessionID, sku string) error {
	live, err := s.store.GetLiveAgreementBySessionAndSKU(ctx, sessionID, sku, store.NowUnix())
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if live.Status == store.StatusOrderCreated {
		return ErrLiveAgreement
	}
	reason := "SUPERSEDED"
	return s.store.TransitionAgreement(ctx, live.ID, store.StatusOffered, store.StatusExpired, &reason, nil, nil)
}

// ExpireAgreement marks an OFFERED agreement of the given session as EXPIRED with a reason.
// Used by REQUOTE_WITHOUT_PAY_LATER (reason PAY_LATER_INELIGIBLE).
func (s *Service) ExpireAgreement(ctx context.Context, sessionID, agreementID, reason string) error {
	agr, err := s.store.GetAgreement(ctx, agreementID)
	if err != nil {
		return err
	}
	if agr.SessionID != sessionID {
		return store.ErrNotFound // never reveal other sessions' agreements
	}
	return s.store.TransitionAgreement(ctx, agreementID, store.StatusOffered, store.StatusExpired, &reason, nil, nil)
}

// Redeem verifies the clearing token and atomically transitions OFFERED -> ORDER_CREATED.
func (s *Service) Redeem(ctx context.Context, agreementID, tokenHex string) (*store.AgreementRecord, error) {
	agr, err := s.store.GetAgreement(ctx, agreementID)
	if err != nil {
		return nil, err
	}

	// Verify cryptographic token
	payload := clearing.AgreementTokenPayload{
		AgreementID:   agr.ID,
		PriceCents:    agr.PriceCents,
		Shipping:      agr.ShippingTier,
		PayLater:      agr.PayLater,
		BundleSKU:     agr.BundleSKU,
		ExpiresAtUnix: agr.ExpiresAt,
	}
	now := store.NowUnix()
	if err := clearing.Verify(payload, tokenHex, s.hmacSecret, now); err != nil {
		return nil, err
	}

	// Atomic transition OFFERED -> ORDER_CREATED
	reason := "CHECKOUT_INITIATED"
	if err := s.store.TransitionAgreement(ctx, agreementID, store.StatusOffered, store.StatusOrderCreated, &reason, nil, nil); err != nil {
		return nil, err
	}

	return s.store.GetAgreement(ctx, agreementID)
}

func (s *Service) buildConcessions(ctx context.Context, res *clearing.Result, bundle *clearing.Product) ConcessionsDTO {
	shippingDays := "5-7"
	if res.Shipping == clearing.ShippingExpress {
		shippingDays = "1-2"
	}

	paymentMethod := "PAYPAL"
	var installments *InstallmentsDTO
	if res.PayLater {
		paymentMethod = "PAY_LATER"
		// 4 installments: round half up (res.PriceCents + 2) / 4
		quarter := (res.PriceCents + 2) / 4
		installments = &InstallmentsDTO{
			Count:  4,
			Amount: quarter.Format(),
		}
	}

	var bundleItem *BundleItemDTO
	if res.Bundle && bundle != nil {
		name := "Cashmere Scarf"
		p, err := s.store.GetProduct(ctx, bundle.SKU)
		if err == nil && p != nil {
			name = p.Name
		}
		bundleItem = &BundleItemDTO{
			SKU:         bundle.SKU,
			Name:        name,
			RetailValue: bundle.RetailValueCents.Format(),
		}
	}

	return ConcessionsDTO{
		ShippingTier:   res.Shipping,
		ShippingDays:   shippingDays,
		ShippingCharge: "0.00",
		PaymentMethod:  paymentMethod,
		Installments:   installments,
		Bundle:         bundleItem,
	}
}
