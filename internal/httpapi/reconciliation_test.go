package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/nashpay/nashpay/internal/paypal"
	"github.com/nashpay/nashpay/internal/store"
	"net/http/httptest"
	"testing"
)

type lostCaptureResponse struct {
	paypal.Client
	capture *paypal.Capture
	calls   int
}

func (c *lostCaptureResponse) CaptureOrder(ctx context.Context, id, key string) (*paypal.Capture, error) {
	c.calls++
	capture, err := c.Client.CaptureOrder(ctx, id, key)
	if err != nil {
		return nil, err
	}
	c.capture = capture
	return nil, errors.New("simulated connection loss after provider capture")
}
func (c *lostCaptureResponse) GetOrder(ctx context.Context, id string) (*paypal.Order, error) {
	order, err := c.Client.GetOrder(ctx, id)
	if err == nil && order != nil && c.capture != nil {
		order.Status = "COMPLETED"
		order.PurchaseUnits[0].Payments.Captures = []paypal.Capture{*c.capture}
	}
	return order, err
}
func TestLostCaptureResponseReconcilesWithoutSecondCharge(t *testing.T) {
	market := marketplaceFixture(t)
	tenant := market.tenants[0]
	client := &lostCaptureResponse{Client: tenant.server.PPHandler.paypalClient}
	tenant.server.PPHandler.paypalClient = client
	rec := httptest.NewRecorder()
	tenant.server.PPHandler.handleWinWinMatch(rec, httptest.NewRequest("POST", "/api/fairdeal/match", bytes.NewBufferString(`{"budget":"130.00","shipping_flexibility":0.85}`)))
	var match WinWinMatchResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &match)
	if len(match.TopOffers) != 1 {
		t.Fatal(rec.Body.String())
	}
	offer := match.TopOffers[0]
	body, _ := json.Marshal(CreateOrderRequest{AgreementID: offer.AgreementID, ClearingToken: offer.ClearingToken})
	created := httptest.NewRecorder()
	tenant.server.Mux.ServeHTTP(created, httptest.NewRequest("POST", "/api/paypal/orders", bytes.NewReader(body)))
	var order CreateOrderResponse
	_ = json.Unmarshal(created.Body.Bytes(), &order)
	if order.OrderID == "" {
		t.Fatal(created.Body.String())
	}
	first := httptest.NewRecorder()
	tenant.server.Mux.ServeHTTP(first, httptest.NewRequest("POST", "/api/paypal/orders/"+order.OrderID+"/capture", nil))
	if first.Code != 502 {
		t.Fatalf("expected uncertain result: %d %s", first.Code, first.Body.String())
	}
	a, _ := tenant.server.Store.GetAgreement(context.Background(), offer.AgreementID)
	if a.Status != store.StatusOrderCreated || !tenant.server.Store.PaymentUncertain(context.Background(), a.ID) {
		t.Fatal("uncertain payment was released or falsely failed")
	}
	second := httptest.NewRecorder()
	tenant.server.Mux.ServeHTTP(second, httptest.NewRequest("POST", "/api/paypal/orders/"+order.OrderID+"/capture", nil))
	if second.Code != 200 || client.calls != 1 {
		t.Fatalf("reconciliation %d, captures %d: %s", second.Code, client.calls, second.Body.String())
	}
	a, _ = tenant.server.Store.GetAgreement(context.Background(), a.ID)
	if a.Status != store.StatusCaptured || a.CaptureID == nil || tenant.server.Store.PaymentUncertain(context.Background(), a.ID) {
		t.Fatal("reconciliation not persisted")
	}
}
