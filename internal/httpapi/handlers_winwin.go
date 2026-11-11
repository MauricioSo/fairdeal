package httpapi

import (
	"fmt"
	"math"
	"net/http"

	"github.com/nashpay/nashpay/internal/ai"
	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/kernel"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/store"
)

// Only explicitly submitted deal constraints are used. Browsing history is not collected.
type WinWinMatchRequest struct {
	SKU                 string  `json:"sku"`
	Budget              string  `json:"budget"`
	ShippingFlexibility float64 `json:"shipping_flexibility"`
	PrefersPayLater     bool    `json:"prefers_pay_later"`
	WillingToBundle     bool    `json:"willing_to_bundle"`
}

type WinWinOfferDTO struct {
	OfferID           string                     `json:"offer_id"`
	AgreementID       string                     `json:"agreement_id"`
	ClearingToken     string                     `json:"clearing_token"`
	MerchantName      string                     `json:"merchant_name"`
	Badge             string                     `json:"badge"`
	FinalPrice        string                     `json:"final_price"`
	StandardTotal     string                     `json:"standard_total"`
	SavingsVsStandard string                     `json:"savings_vs_standard"`
	BuyerWinSummary   string                     `json:"buyer_win_summary"`
	SellerWinSummary  string                     `json:"seller_win_summary"`
	RelevanceScore    int                        `json:"relevance_score"`
	MatchedInterest   string                     `json:"matched_interest,omitempty"`
	Concessions       negotiation.ConcessionsDTO `json:"concessions"`
	ExpiresAt         int64                      `json:"expires_at"`
}

type WinWinMatchResponse struct {
	SKU              string                     `json:"sku"`
	ProductName      string                     `json:"product_name"`
	TopOffers        []WinWinOfferDTO           `json:"top_offers"`
	AllOffers        []WinWinOfferDTO           `json:"all_offers"`
	TotalEvaluated   int                        `json:"total_evaluated"`
	PrivacyGuarantee string                     `json:"privacy_guarantee"`
	KernelWorker     string                     `json:"kernel_worker"`
	MarketAudit      *kernel.MarketVerification `json:"market_audit,omitempty"`
	NegotiationLog   *ai.AgentNegotiationResult `json:"negotiation_log,omitempty"`
}

func (h *PayPalHandler) handleWinWinMatch(w http.ResponseWriter, r *http.Request) {
	var req WinWinMatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SKU == "" {
		req.SKU = "CH-WINTER-PARKA-01"
	}
	budget, err := money.Parse(req.Budget)
	if err != nil || budget < 100 || budget > 1000000 || math.IsNaN(req.ShippingFlexibility) || math.IsInf(req.ShippingFlexibility, 0) || req.ShippingFlexibility < 0 || req.ShippingFlexibility > 1 {
		writeJSONError(w, http.StatusBadRequest, "INVALID", "budget must be between $1 and $10,000 and delivery flexibility between 0 and 1")
		return
	}
	buyer := clearing.BuyerConstraints{Budget: budget, ShippingFlexibility: req.ShippingFlexibility, PrefersPayLater: req.PrefersPayLater, WillingToBundle: req.WillingToBundle}
	sid := GetOrCreateSessionID(w, r)
	result, err := h.negService.Commit(r.Context(), sid, "", req.SKU, buyer, false)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	product, err := h.store.GetProduct(r.Context(), req.SKU)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	a := result.Agreement
	offer := WinWinOfferDTO{
		OfferID: result.AgreementID, AgreementID: result.AgreementID, ClearingToken: result.ClearingToken,
		MerchantName: "Alpine Gear Co. (Demo merchant)", Badge: "Validated offer",
		FinalPrice: a.FinalPrice, StandardTotal: a.StandardTotal, SavingsVsStandard: a.SavingsVsStandard,
		BuyerWinSummary:  fmt.Sprintf("Save $%s vs. this demo catalog's standard checkout. Within your maximum budget.", a.SavingsVsStandard),
		SellerWinSummary: "Contribution margin validated after product, accessory, shipping and configured payment costs. Demo inventory and costs.",
		Concessions:      a.Concessions, ExpiresAt: result.ExpiresAt,
	}
	audit := &kernel.MarketVerification{Status: "Not verified: no market source configured"}
	if h.kernelClient != nil {
		audit, _ = h.kernelClient.VerifyMarketPrice(r.Context(), req.SKU, a.FinalPrice)
	}
	offers := []WinWinOfferDTO{offer}
	writeJSON(w, http.StatusOK, WinWinMatchResponse{
		SKU: req.SKU, ProductName: product.Name, TopOffers: offers, AllOffers: offers, TotalEvaluated: len(result.Trace.Frontiers),
		PrivacyGuarantee: "No browsing history is collected. Submitted budget and delivery preferences are processed by the Fairdeal server. Natural-language requests are sent to the configured AI provider when enabled.",
		KernelWorker:     audit.Status, MarketAudit: audit, NegotiationLog: ai.ExplainAgreement(a, budget.Format()),
	})
}

// Buyer metrics are limited to the current browser session and actual stored captures.
func (h *PayPalHandler) handleBuyerSummary(w http.ResponseWriter, r *http.Request) {
	sid := GetOrCreateSessionID(w, r)
	list, err := h.store.GetRecentAgreements(r.Context(), 100)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	var saved, gifts money.Cents
	count := 0
	for _, a := range list {
		if a.SessionID != sid || a.Status != store.StatusCaptured {
			continue
		}
		count++
		if a.StandardTotalCents > a.PriceCents {
			saved += money.Cents(a.StandardTotalCents - a.PriceCents)
		}
		if a.BundleSKU != nil {
			if p, e := h.store.GetProduct(r.Context(), *a.BundleSKU); e == nil {
				gifts += money.Cents(p.RetailValueCents)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"buyer_name": "Demo shopper", "buyer_tier": "Current session · latest 100 agreements", "completed_orders": count, "total_cash_saved": saved.Format(), "free_gifts_value": gifts.Format(), "total_benefit": (saved + gifts).Format(), "private_interests": []string{}})
}
