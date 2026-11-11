package httpapi

import (
	"net/http"
)

func (h *MerchantHandler) events(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.DB().QueryContext(r.Context(), "SELECT id,kind,agreement_id,created_at FROM merchant_events ORDER BY id DESC LIMIT 100")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer rows.Close()
	events := []map[string]any{}
	for rows.Next() {
		var id, created int64
		var kind string
		var agreement *string
		if err = rows.Scan(&id, &kind, &agreement, &created); err != nil {
			writeServiceError(w, err)
			return
		}
		events = append(events, map[string]any{"id": id, "kind": kind, "agreement_id": agreement, "created_at": created})
	}
	if rows.Err() != nil {
		writeServiceError(w, rows.Err())
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}
