package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/nashpay/nashpay/internal/ai"
	"github.com/nashpay/nashpay/internal/kernel"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/paypal"
	"github.com/nashpay/nashpay/internal/store"
)

type CreateOrderRequest struct {
	AgreementID   string `json:"agreement_id"`
	ClearingToken string `json:"clearing_token"`
}

type CreateOrderResponse struct {
	OrderID string `json:"order_id"`
	Mode    string `json:"mode"`
}

type CaptureOrderResponse struct {
	Status      string `json:"status"`
	CaptureID   string `json:"capture_id"`
	AgreementID string `json:"agreement_id"`
	Amount      string `json:"amount"`
}

type PayPalHandler struct {
	store        *store.Store
	paypalClient paypal.Client
	negService   *negotiation.Service
	mode         string
	baseURL      string
	llmClient    *ai.LLMClient
	kernelClient *kernel.Client
}

func NewPayPalHandler(st *store.Store, pp paypal.Client, svc *negotiation.Service, mode, baseURL string) *PayPalHandler {
	return &PayPalHandler{store: st, paypalClient: pp, negService: svc, mode: mode, baseURL: baseURL}
}

func (h *PayPalHandler) SetLLMClient(llm *ai.LLMClient) {
	h.llmClient = llm
}

func (h *PayPalHandler) SetKernelClient(kc *kernel.Client) {
	h.kernelClient = kc
}

func (h *PayPalHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/paypal/orders", h.handleCreateOrder)
	mux.HandleFunc("POST /api/paypal/orders/{id}/capture", h.handleCaptureOrder)
	mux.HandleFunc("GET /api/agreements/{id}/public", h.handlePublicAgreement)
	mux.HandleFunc("POST /api/fairdeal/match", h.handleWinWinMatch)
	mux.HandleFunc("POST /api/wasabi/match", h.handleWinWinMatch)
	mux.HandleFunc("POST /api/pact/match", h.handleWinWinMatch)
	mux.HandleFunc("POST /api/winwin/match", h.handleWinWinMatch)
	mux.HandleFunc("GET /api/buyer/summary", h.handleBuyerSummary)
}

func (h *PayPalHandler) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AgreementID == "" || req.ClearingToken == "" {
		writeJSONError(w, http.StatusBadRequest, "INVALID", "agreement_id and clearing_token are required")
		return
	}

	// BR-21: price comes only from SQLite; anything else the client sent is ignored.
	agr, err := h.negService.Redeem(r.Context(), req.AgreementID, req.ClearingToken)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if agr.PayPalOrderID != nil {
		writeJSON(w, 201, CreateOrderResponse{OrderID: *agr.PayPalOrderID, Mode: h.mode})
		return
	}
	orderID, err := h.createPayPalOrder(r, agr)
	if err != nil {
		// The agreement is already redeemed and cannot be reused: fail it so the buyer renegotiates.
		_ = h.store.MarkPaymentUncertain(r.Context(), agr.ID)
		slog.Error("paypal order creation failed", "agreement", agr.ID, "error", err)
		writeJSONError(w, http.StatusBadGateway, "UPSTREAM", "payment result uncertain; retry this agreement")
		return
	}

	if err := h.store.SetPayPalOrderID(r.Context(), agr.ID, orderID); err != nil {
		_ = h.store.MarkPaymentUncertain(r.Context(), agr.ID)
		writeServiceError(w, err)
		return
	}

	_ = h.store.ClearPaymentUncertain(r.Context(), agr.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(CreateOrderResponse{OrderID: orderID, Mode: h.mode})
}

func (h *PayPalHandler) createPayPalOrder(r *http.Request, agr *store.AgreementRecord) (string, error) {
	mainProd, err := h.store.GetProduct(r.Context(), agr.SKU)
	if err != nil {
		return "", err
	}
	var bundleProd *store.Product
	if agr.BundleSKU != nil {
		if bundleProd, err = h.store.GetProduct(r.Context(), *agr.BundleSKU); err != nil {
			return "", err
		}
	}
	orderReq, err := paypal.BuildOrderRequest(agr, mainProd, bundleProd, h.baseURL)
	if err != nil {
		return "", err
	}
	return h.paypalClient.CreateOrder(r.Context(), *orderReq)
}

func (h *PayPalHandler) handleCaptureOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("id")
	agr, err := h.store.GetAgreementByPayPalOrderID(r.Context(), orderID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// Idempotent: a captured agreement just returns its stored capture.
	if agr.Status == store.StatusCaptured && agr.CaptureID != nil {
		h.writeCapture(w, agr, *agr.CaptureID)
		return
	}
	if agr.Status != store.StatusOrderCreated {
		writeJSONError(w, http.StatusConflict, "ALREADY_USED", "this order can no longer be captured")
		return
	}

	if h.reconcileCompleted(r.Context(), agr) {
		h.writeCaptureFromStore(w, r, agr.ID)
		return
	}
	if err := h.verifyOrder(r, orderID, agr); err != nil {
		if current, e := h.store.GetAgreement(r.Context(), agr.ID); e == nil && current.Status == store.StatusCaptured && current.CaptureID != nil {
			h.writeCapture(w, current, *current.CaptureID)
			return
		}
		_ = h.store.MarkPaymentUncertain(r.Context(), agr.ID)
		slog.Warn("paypal order verification failed", "agreement", agr.ID, "error", err)
		writeJSONError(w, http.StatusConflict, "NOT_APPROVED", "the order was not approved or does not match the agreement")
		return
	}

	if err := h.store.BeginCapture(r.Context(), agr.ID); err != nil {
		writeServiceError(w, err)
		return
	}
	capture, err := h.paypalClient.CaptureOrder(r.Context(), orderID, agr.ID+"-capture")
	if err != nil || capture == nil || capture.Status != "COMPLETED" || capture.ID == "" || capture.Amount.CurrencyCode != "USD" || capture.Amount.Value != money.Cents(agr.PriceCents).Format() {
		_ = h.store.MarkPaymentUncertain(r.Context(), agr.ID)
		slog.Error("paypal capture failed", "agreement", agr.ID, "error", err)
		writeJSONError(w, http.StatusBadGateway, "UPSTREAM", "payment result uncertain; retry to reconcile before starting a new purchase")
		return
	}

	reason := "CAPTURE_SUCCESS"
	if err := h.store.TransitionAgreement(r.Context(), agr.ID, store.StatusOrderCreated, store.StatusCaptured, &reason, nil, &capture.ID); err != nil {
		// Lost a race with a concurrent identical capture: serve the stored result.
		if cur, gErr := h.store.GetAgreement(r.Context(), agr.ID); gErr == nil && cur.CaptureID != nil {
			h.writeCapture(w, cur, *cur.CaptureID)
			return
		}
		writeServiceError(w, err)
		return
	}
	h.writeCapture(w, agr, capture.ID)
}

// verifyOrder implements §4.5: APPROVED status, custom_id == agreement id and exact amount.
func (h *PayPalHandler) verifyOrder(r *http.Request, orderID string, agr *store.AgreementRecord) error {
	order, err := h.paypalClient.GetOrder(r.Context(), orderID)
	if err != nil {
		return err
	}
	if order == nil || order.Status != "APPROVED" {
		return paypal.ErrOrderNotApproved
	}
	if len(order.PurchaseUnits) == 0 {
		return paypal.ErrOrderAmountMismatch
	}
	pu := order.PurchaseUnits[0]
	if pu.CustomID != agr.ID || pu.Amount.CurrencyCode != "USD" || pu.Amount.Value != money.Cents(agr.PriceCents).Format() {
		return errors.Join(paypal.ErrOrderAmountMismatch)
	}
	return nil
}

func (h *PayPalHandler) writeCapture(w http.ResponseWriter, agr *store.AgreementRecord, captureID string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CaptureOrderResponse{
		Status:      "COMPLETED",
		CaptureID:   captureID,
		AgreementID: agr.ID,
		Amount:      money.Cents(agr.PriceCents).Format(),
	})
}

// handlePublicAgreement returns the sanitised receipt used by the confirmation page.
func (h *PayPalHandler) handlePublicAgreement(w http.ResponseWriter, r *http.Request) {
	dto, err := h.negService.PublicView(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto)
}
