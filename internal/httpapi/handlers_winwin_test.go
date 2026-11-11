package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/paypal"
	"github.com/nashpay/nashpay/internal/store"
)

func TestMatchOfferMatchesStoredAgreementAndPayment(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "match.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := negotiation.NewService(st, []byte("01234567890123456789012345678901"), 300)
	pp := paypal.NewMockClient()
	handler := NewPayPalHandler(st, pp, svc, "mock", "http://localhost:8080")
	mux := http.NewServeMux()
	handler.Register(mux)
	for _, tc := range []struct {
		budget           string
		bundle, paylater bool
	}{
		{"105.00", true, true}, {"105.00", false, false}, {"85.00", true, false},
	} {
		body, _ := json.Marshal(WinWinMatchRequest{Budget: tc.budget, ShippingFlexibility: 0.85, WillingToBundle: tc.bundle, PrefersPayLater: tc.paylater})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/fairdeal/match", bytes.NewReader(body)))
		if rec.Code != 200 {
			t.Fatalf("match: %d %s", rec.Code, rec.Body.String())
		}
		var response WinWinMatchResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.TopOffers) != 1 || len(response.AllOffers) != 1 {
			t.Fatal("expected one actual merchant offer")
		}
		off := response.TopOffers[0]
		a, err := st.GetAgreement(ctx, off.AgreementID)
		if err != nil {
			t.Fatal(err)
		}
		price, _ := money.Parse(off.FinalPrice)
		budget, _ := money.Parse(tc.budget)
		if int64(price) != a.PriceCents || price > budget || off.Concessions.ShippingTier != a.ShippingTier || (off.Concessions.Bundle != nil) != (a.BundleSKU != nil) || (off.Concessions.PaymentMethod == "PAY_LATER") != a.PayLater {
			t.Fatalf("offer disagrees with contract: %+v %+v", off, a)
		}
		if !tc.bundle && a.BundleSKU != nil || !tc.paylater && a.PayLater {
			t.Fatal("ignored buyer refusal")
		}
		cost := int64(5000)
		if a.BundleSKU != nil {
			cost += 900
		}
		ship := int64(400)
		if a.ShippingTier == "EXPRESS" {
			ship = 1500
		}
		if a.ContributionCents != a.PriceCents-cost-ship-a.FeeCents || a.ContributionCents*10000 < a.PriceCents*2000 {
			t.Fatalf("invalid economics: %+v", a)
		}
		if response.NegotiationLog.AgreedPrice != off.FinalPrice {
			t.Fatal("trace price differs")
		}
		payload, _ := json.Marshal(CreateOrderRequest{AgreementID: off.AgreementID, ClearingToken: off.ClearingToken})
		created := httptest.NewRecorder()
		mux.ServeHTTP(created, httptest.NewRequest("POST", "/api/paypal/orders", bytes.NewReader(payload)))
		if created.Code != 201 {
			t.Fatalf("create: %d %s", created.Code, created.Body.String())
		}
		var order CreateOrderResponse
		json.Unmarshal(created.Body.Bytes(), &order)
		captured := httptest.NewRecorder()
		mux.ServeHTTP(captured, httptest.NewRequest("POST", "/api/paypal/orders/"+order.OrderID+"/capture", nil))
		var capture CaptureOrderResponse
		json.Unmarshal(captured.Body.Bytes(), &capture)
		if captured.Code != 200 || capture.Amount != off.FinalPrice {
			t.Fatalf("displayed %s, captured %+v", off.FinalPrice, capture)
		}
	}
}

func TestMatchRejectsInvalidAndInfeasibleBudgets(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "limits.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := negotiation.NewService(st, []byte("01234567890123456789012345678901"), 300)
	h := NewPayPalHandler(st, paypal.NewMockClient(), svc, "mock", "")
	mux := http.NewServeMux()
	h.Register(mux)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"budget":"60.00","shipping_flexibility":0.85}`, 422},
		{`{"budget":"invalid"}`, 400}, {`{"budget":"-1"}`, 400},
		{`{"budget":"105.00","shipping_flexibility":1.1}`, 400},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/fairdeal/match", bytes.NewBufferString(tc.body)))
		if rec.Code != tc.code {
			t.Errorf("%s: got %d want %d", tc.body, rec.Code, tc.code)
		}
	}
	list, err := st.GetRecentAgreements(ctx, 100)
	if err != nil || len(list) != 0 {
		t.Fatalf("invalid mandates created agreements: %+v %v", list, err)
	}
}
