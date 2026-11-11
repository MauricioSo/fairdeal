package httpapi

import (
	"context"
	"encoding/json"
	"github.com/nashpay/nashpay/internal/store"
	"io"
	"net/http"
)

type webhookVerifier interface {
	VerifyWebhook(context.Context, http.Header, json.RawMessage, string) (bool, error)
}

func (m *Marketplace) webhook(w http.ResponseWriter, r *http.Request) {
	verifier, ok := m.client.(webhookVerifier)
	if !ok || m.WebhookID == "" {
		writeJSONError(w, 503, "WEBHOOK_UNAVAILABLE", "webhook verification is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
	if err != nil || !json.Valid(body) {
		writeJSONError(w, 400, "INVALID", "invalid event")
		return
	}
	valid, err := verifier.VerifyWebhook(r.Context(), r.Header, body, m.WebhookID)
	if err != nil {
		writeJSONError(w, 503, "UPSTREAM", "verification unavailable")
		return
	}
	if !valid {
		writeJSONError(w, 403, "INVALID_SIGNATURE", "event signature rejected")
		return
	}
	var event struct {
		Type     string `json:"event_type"`
		Resource struct {
			Data struct {
				IDs struct {
					OrderID string `json:"order_id"`
				} `json:"related_ids"`
			} `json:"supplementary_data"`
		} `json:"resource"`
	}
	if json.Unmarshal(body, &event) != nil {
		writeJSONError(w, 400, "INVALID", "invalid event")
		return
	}
	if event.Type != "PAYMENT.CAPTURE.COMPLETED" {
		w.WriteHeader(204)
		return
	}
	for _, t := range m.tenants {
		a, e := t.server.Store.GetAgreementByPayPalOrderID(r.Context(), event.Resource.Data.IDs.OrderID)
		if e != nil {
			continue
		}
		if a.Status == store.StatusCaptured {
			w.WriteHeader(204)
			return
		}
		if a.Status != store.StatusOrderCreated {
			writeJSONError(w, 409, "INVALID_STATE", "payment requires review")
			return
		}
		if e = t.server.Store.MarkPaymentUncertain(r.Context(), a.ID); e != nil {
			writeServiceError(w, e)
			return
		}
		t.server.PPHandler.reconcileCompleted(r.Context(), a)
		w.WriteHeader(204)
		return
	}
	writeJSONError(w, 404, "NOT_FOUND", "unknown order")
}
