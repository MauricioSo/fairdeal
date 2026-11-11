package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/nashpay/nashpay/internal/ai"
	"github.com/nashpay/nashpay/internal/store"
)

type CompilePolicyRequest struct {
	Text string `json:"text"`
}

type MerchantHandler struct {
	store     *store.Store
	llmClient *ai.LLMClient
	demoMode  bool
	auth      *MerchantAuth
}

// NewMerchantHandler requires a merchant login session for every merchant endpoint.
func NewMerchantHandler(st *store.Store, llm *ai.LLMClient, demoMode bool, auth *MerchantAuth) *MerchantHandler {
	return &MerchantHandler{store: st, llmClient: llm, demoMode: demoMode, auth: auth}
}

// guard rejects the request unless the merchant token matches (constant-time compare).
func (h *MerchantHandler) guard(next http.HandlerFunc) http.HandlerFunc {
	return h.auth.Guard(next)
}

func (h *MerchantHandler) Register(mux *http.ServeMux) {
	h.auth.Register(mux)
	mux.HandleFunc("GET /api/merchant/policy", h.guard(h.handleGetPolicy))
	mux.HandleFunc("POST /api/merchant/policy/compile", h.guard(h.handleCompilePolicy))
	mux.HandleFunc("POST /api/merchant/policy/{id}/activate", h.guard(h.handleActivatePolicy))
	mux.HandleFunc("GET /api/merchant/metrics", h.guard(h.handleGetMetrics))
	mux.HandleFunc("GET /api/merchant/agreements", h.guard(h.handleGetAgreements))
	mux.HandleFunc("POST /api/merchant/demo/reset", h.guard(h.handleResetDemo))
}

func (h *MerchantHandler) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	active, err := h.store.GetActivePolicy(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	drafts, _ := h.store.ListPolicyDrafts(r.Context())
	if drafts == nil {
		drafts = []store.MerchantPolicy{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": active, "drafts": drafts})
}

func (h *MerchantHandler) handleCompilePolicy(w http.ResponseWriter, r *http.Request) {
	var req CompilePolicyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Text) == 0 {
		writeJSONError(w, http.StatusBadRequest, "INVALID", "text is required")
		return
	}

	active, err := h.store.GetActivePolicy(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	hardFloor := active.HardFloorMarginPct
	if hardFloor <= 0 {
		hardFloor = 0.15
	}

	isDeadstock := func(sku string) bool {
		p, err := h.store.GetProduct(r.Context(), sku)
		return err == nil && p.IsDeadstock
	}
	resp := ai.CompileMerchantPolicy(r.Context(), req.Text, active.Policy, hardFloor, isDeadstock, h.llmClient)

	draft, err := h.store.CreatePolicyDraft(r.Context(), req.Text, resp.Source, resp.Draft, resp.Adjustments, hardFloor)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"draft": draft, "adjustments": resp.Adjustments, "source": resp.Source})
}

func (h *MerchantHandler) handleActivatePolicy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID", "invalid policy id")
		return
	}
	active, err := h.store.ActivatePolicy(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": active})
}

func (h *MerchantHandler) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := h.store.GetMetrics(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *MerchantHandler) handleGetAgreements(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	list, err := h.store.GetRecentAgreements(r.Context(), limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if list == nil {
		list = []store.AgreementRecord{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *MerchantHandler) handleResetDemo(w http.ResponseWriter, r *http.Request) {
	if !h.demoMode {
		writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "demo mode is not enabled")
		return
	}
	if err := h.store.ResetDemoData(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "demo data reset successfully"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
