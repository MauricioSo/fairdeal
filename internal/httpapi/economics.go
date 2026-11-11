package httpapi

import (
	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/economics"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/store"
	"net/http"
)

func (h *MerchantHandler) registerEconomics(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/merchant/economics", h.guard(h.getEconomics))
	mux.HandleFunc("POST /api/merchant/policy/manual", h.guard(h.manualPolicy))
	mux.HandleFunc("POST /api/merchant/economics/simulate", h.guard(h.simulateEconomics))
	mux.HandleFunc("GET /api/merchant/agreements/{id}/economics", h.guard(h.agreementEconomics))
}
func (h *MerchantHandler) getEconomics(w http.ResponseWriter, r *http.Request) {
	policy, err := h.store.GetActivePolicy(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	market, err := h.store.GetMarketSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"active": policy, "market": market, "currency": "USD", "cost_basis": "Configured costs or conservative upper bounds, not carrier or tax quotations. Zero/unset product cost blocks automatic offers. Holding-cost relief is a ranking benefit, never margin revenue. Platform percentage applies to net revenue; payment percentage to the complete charge. Tax is included in the quoted total. Legacy optional costs default to zero: review before real use.", "pack_scope": "One main item and one optional merchant-selected accessory; one unit each. Express 1–2 days or ground 5–7 days in this demo. No external fulfillment guarantee."})
}
func (h *MerchantHandler) manualPolicy(w http.ResponseWriter, r *http.Request) {
	var cfg store.PolicyConfig
	if !decodeJSON(w, r, &cfg) {
		return
	}
	active, err := h.store.GetActivePolicy(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err = store.ValidatePolicy(cfg, active.HardFloorMarginPct); err != nil {
		writeJSONError(w, 400, "INVALID_POLICY", err.Error())
		return
	}
	if !h.validCatalogRules(r, cfg) {
		writeJSONError(w, 400, "INVALID_POLICY", "bundle and product rules must reference available catalog products; bundle must be a distinct accessory")
		return
	}
	cfg.Rationale = "Merchant-reviewed commercial rules and cost bounds"
	draft, err := h.store.CreatePolicyDraft(r.Context(), "Explicit merchant configuration", "manual", cfg, []string{}, active.HardFloorMarginPct)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"draft": draft})
}
func (h *MerchantHandler) validCatalogRules(r *http.Request, c store.PolicyConfig) bool {
	if c.BundleSKU != nil && *c.BundleSKU != "" {
		p, err := h.store.GetProduct(r.Context(), *c.BundleSKU)
		if err != nil || !p.IsDeadstock {
			return false
		}
	}
	for _, rule := range c.Economics.ProductRules {
		if _, err := h.store.GetProduct(r.Context(), rule.SKU); err != nil {
			return false
		}
	}
	return true
}
func (h *MerchantHandler) simulateEconomics(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SKU    string              `json:"sku"`
		Price  money.Cents         `json:"price_cents"`
		Policy *store.PolicyConfig `json:"policy,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Price < 0 || req.Price > 1000000 || req.SKU == "" {
		writeJSONError(w, 400, "INVALID", "invalid product or price")
		return
	}
	active, err := h.store.GetActivePolicy(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if req.Policy != nil {
		if err = store.ValidatePolicy(*req.Policy, active.HardFloorMarginPct); err != nil || !h.validCatalogRules(r, *req.Policy) {
			writeJSONError(w, 400, "INVALID_POLICY", "invalid proposed rules or costs")
			return
		}
		active.Policy = *req.Policy
	}
	svc := negotiation.NewService(h.store, nil, 300)
	in, _, err := svc.BuildSolverInputWithPolicy(r.Context(), req.SKU, clearing.BuyerConstraints{}, false, active)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	prices := []money.Cents{req.Price}
	if req.Price == 0 {
		prices = []money.Cents{in.Product.ListPriceCents, in.Product.ListPriceCents * 90 / 100, in.Product.ListPriceCents * 80 / 100}
	}
	type row struct {
		Price      money.Cents          `json:"price_cents"`
		Shipping   string               `json:"shipping"`
		BundleSKU  *string              `json:"bundle_sku"`
		Evaluation economics.Evaluation `json:"evaluation"`
	}
	rows := []row{}
	for _, price := range prices {
		for _, shipping := range []string{clearing.ShippingExpress, clearing.ShippingConsolidatedGround} {
			for _, bundle := range []bool{false, true} {
				if bundle && in.Bundle == nil {
					continue
				}
				var sku *string
				if bundle {
					sku = &in.Bundle.SKU
				}
				rows = append(rows, row{price, shipping, sku, clearing.EvaluateOffer(*in, price, shipping, bundle)})
			}
		}
	}
	writeJSON(w, 200, map[string]any{"binding": false, "policy_id": active.ID, "rows": rows})
}
func (h *MerchantHandler) agreementEconomics(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.store.GetEconomics(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSONError(w, 404, "NOT_FOUND", "no economic snapshot; historical agreements retain their original metrics")
		return
	}
	writeJSON(w, 200, snapshot)
}
