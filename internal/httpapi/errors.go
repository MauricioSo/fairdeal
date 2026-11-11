package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/store"
)

// writeJSONError emits the single error envelope of the API (§4.3).
func writeJSONError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// writeServiceError maps domain errors to HTTP codes. Unknown errors are logged
// server-side and returned as a generic INTERNAL so internals never leak to clients.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrOutOfStock):
		writeJSONError(w, http.StatusConflict, "OUT_OF_STOCK", "product is no longer available")
	case errors.Is(err, clearing.ErrBadToken):
		writeJSONError(w, http.StatusUnauthorized, "BAD_TOKEN", "invalid clearing token")
	case errors.Is(err, clearing.ErrExpired):
		writeJSONError(w, http.StatusGone, "EXPIRED", "the offer has expired, please negotiate again")
	case errors.Is(err, store.ErrAlreadyUsed), errors.Is(err, store.ErrInvalidTransition):
		writeJSONError(w, http.StatusConflict, "ALREADY_USED", "this agreement is no longer available")
	case errors.Is(err, store.ErrNotFound), errors.Is(err, negotiation.ErrProductNotFound):
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, negotiation.ErrLiveAgreement):
		writeJSONError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error())
	case errors.Is(err, clearing.ErrNoZOPA):
		writeJSONError(w, http.StatusUnprocessableEntity, "NO_ZOPA", "no zone of possible agreement")
	default:
		slog.Error("unhandled service error", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
	}
}

// decodeJSON reads a size-limited JSON body (4 KB) into v.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID", "malformed request payload")
		return false
	}
	return true
}
