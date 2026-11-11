package httpapi

import (
	"github.com/nashpay/nashpay/internal/store"
	"net/http"
	"regexp"
)

var catalogSKU = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{2,63}$`)

func (h *MerchantHandler) registerCatalog(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/merchant/catalog", h.guard(h.catalogList))
	mux.HandleFunc("PUT /api/merchant/catalog/{sku}", h.guard(h.catalogSave))
	mux.HandleFunc("DELETE /api/merchant/catalog/{sku}", h.guard(h.catalogArchive))
}
func (h *MerchantHandler) catalogList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.DB().QueryContext(r.Context(), "SELECT sku,name,list_price_cents,cost_cents,retail_value_cents,holding_cost_cents,is_deadstock,stock FROM products ORDER BY sku")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer rows.Close()
	products := []store.Product{}
	for rows.Next() {
		var p store.Product
		if err = rows.Scan(&p.SKU, &p.Name, &p.ListPriceCents, &p.CostCents, &p.RetailValueCents, &p.HoldingCostCents, &p.IsDeadstock, &p.Stock); err != nil {
			writeServiceError(w, err)
			return
		}
		products = append(products, p)
	}
	if rows.Err() != nil {
		writeServiceError(w, rows.Err())
		return
	}
	writeJSON(w, 200, map[string]any{"products": products, "stock_semantics": "available units; active reservations cannot be edited"})
}
func (h *MerchantHandler) catalogSave(w http.ResponseWriter, r *http.Request) {
	var p store.Product
	if !decodeJSON(w, r, &p) {
		return
	}
	p.SKU = r.PathValue("sku")
	if !catalogSKU.MatchString(p.SKU) || len(p.Name) < 1 || len(p.Name) > 160 || p.ListPriceCents < 100 || p.ListPriceCents > 1000000 || p.CostCents < 0 || p.CostCents > p.ListPriceCents || p.RetailValueCents < 0 || p.RetailValueCents > 1000000 || p.HoldingCostCents < 0 || p.HoldingCostCents > 100000 || p.Stock < 0 || p.Stock > 100000 {
		writeJSONError(w, 400, "INVALID", "invalid catalog values; all monetary values are integer cents")
		return
	}
	if err := h.store.SaveCatalogProduct(r.Context(), p); err != nil {
		writeJSONError(w, 409, "CATALOG_BUSY", "active inventory reservation prevents changes")
		return
	}
	writeJSON(w, 200, p)
}
func (h *MerchantHandler) catalogArchive(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.GetProduct(r.Context(), r.PathValue("sku"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	p.Stock = 0
	if err = h.store.SaveCatalogProduct(r.Context(), *p); err != nil {
		writeJSONError(w, 409, "CATALOG_BUSY", "active inventory reservation prevents changes")
		return
	}
	w.WriteHeader(204)
}
