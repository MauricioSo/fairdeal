package httpapi

import (
	"context"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/store"
	"net/http"
	"sync"
)

// Query the provider after an uncertain capture; never infer success from a timeout.
func (h *PayPalHandler) reconcileCompleted(ctx context.Context, a *store.AgreementRecord) bool {
	if a.PayPalOrderID == nil {
		return false
	}
	order, err := h.paypalClient.GetOrder(ctx, *a.PayPalOrderID)
	if err != nil || order == nil || order.Status != "COMPLETED" || len(order.PurchaseUnits) != 1 {
		return false
	}
	unit := order.PurchaseUnits[0]
	if unit.CustomID != a.ID || unit.Amount.CurrencyCode != "USD" || unit.Amount.Value != money.Cents(a.PriceCents).Format() || unit.Payments == nil || len(unit.Payments.Captures) != 1 {
		return false
	}
	capture := unit.Payments.Captures[0]
	if capture.ID == "" || capture.Status != "COMPLETED" || capture.Amount.CurrencyCode != "USD" || capture.Amount.Value != unit.Amount.Value {
		return false
	}
	reason := "PROVIDER_RECONCILED"
	return h.store.TransitionAgreement(ctx, a.ID, store.StatusOrderCreated, store.StatusCaptured, &reason, nil, &capture.ID) == nil
}
func (h *PayPalHandler) writeCaptureFromStore(w http.ResponseWriter, r *http.Request, id string) {
	a, err := h.store.GetAgreement(r.Context(), id)
	if err != nil || a.CaptureID == nil {
		writeJSONError(w, 503, "RECONCILIATION_REQUIRED", "retry later")
		return
	}
	h.writeCapture(w, a, *a.CaptureID)
}

// Reconcile only observes provider state; it does not initiate captures.
func (m *Marketplace) Reconcile(ctx context.Context) {
	var work sync.WaitGroup
	for _, tenant := range m.tenants {
		work.Add(1)
		go func(t marketTenant) {
			defer work.Done()
			items, err := t.server.Store.PendingReconciliation(ctx)
			if err != nil {
				return
			}
			for i := range items {
				a := &items[i]
				t.server.Store.NoteReconciliationAttempt(ctx, a.ID)
				if a.Status == store.StatusOrderCreated && t.server.Store.PaymentUncertain(ctx, a.ID) {
					if a.PayPalOrderID == nil {
						request, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost", nil)
						if orderID, e := t.server.PPHandler.createPayPalOrder(request, a); e == nil {
							if e = t.server.Store.SetPayPalOrderID(ctx, a.ID, orderID); e == nil {
								_ = t.server.Store.ClearPaymentUncertain(ctx, a.ID)
							}
						}
						continue
					}
					t.server.PPHandler.reconcileCompleted(ctx, a)
				}
			}
			_ = t.server.Store.ExpireReservations(ctx, store.NowUnix())
			_, _ = t.server.Store.ExpireStale(ctx, store.NowUnix())
		}(tenant)
	}
	work.Wait()
}
