package httpapi

import (
	"net/http"

	"github.com/nashpay/nashpay/internal/ai"
)

type ParseIntentRequest struct {
	Text string `json:"text"`
}

type AIHandler struct {
	llmClient *ai.LLMClient
}

func NewAIHandler(llm *ai.LLMClient) *AIHandler {
	return &AIHandler{llmClient: llm}
}

func (h *AIHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/intent/parse", h.handleParseIntent)
}

func (h *AIHandler) handleParseIntent(w http.ResponseWriter, r *http.Request) {
	var req ParseIntentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Text) == 0 {
		writeJSONError(w, http.StatusBadRequest, "INVALID", "input text cannot be empty")
		return
	}
	writeJSON(w, http.StatusOK, ai.ParseBuyerIntent(r.Context(), req.Text, h.llmClient))
}
