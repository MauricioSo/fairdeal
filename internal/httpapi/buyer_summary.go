package httpapi

import (
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/store"
	"net/http"
)

func (m *Marketplace) buyerSummary(w http.ResponseWriter, r *http.Request) {
	sid := GetOrCreateSessionID(w, r)
	var saved, gifts money.Cents
	count := 0
	for _, t := range m.tenants {
		agreements, err := t.server.Store.GetRecentAgreements(r.Context(), 100)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		for _, a := range agreements {
			if a.SessionID != sid || a.Status != store.StatusCaptured {
				continue
			}
			count++
			if a.StandardTotalCents > a.PriceCents {
				saved += money.Cents(a.StandardTotalCents - a.PriceCents)
			}
			if a.BundleSKU != nil {
				if p, e := t.server.Store.GetProduct(r.Context(), *a.BundleSKU); e == nil {
					gifts += money.Cents(p.RetailValueCents)
				}
			}
		}
	}
	writeJSON(w, 200, map[string]any{"buyer_name": "Demo shopper", "buyer_tier": "Current session · latest 100 agreements per merchant", "completed_orders": count, "total_cash_saved": saved.Format(), "free_gifts_value": gifts.Format(), "total_benefit": (saved + gifts).Format(), "private_interests": []string{}})
}
