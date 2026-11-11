package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/nashpay/nashpay/internal/config"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/paypal"
	"github.com/nashpay/nashpay/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func marketplaceFixture(t *testing.T) *Marketplace {
	t.Helper()
	cfg := &config.Config{DBPath: filepath.Join(t.TempDir(), "alpine.db"), HMACSecret: []byte("01234567890123456789012345678901"), MerchantEmail: "merchant@fairdeal.demo", MerchantPasswordHash: testPasswordHash(t), PayPalMode: "mock"}
	st, e := store.Open(context.Background(), cfg.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	pp := paypal.NewMockClient()
	svc := negotiation.NewService(st, cfg.HMACSecret, 300)
	handler := NewPayPalHandler(st, pp, svc, "mock", "https://fairdeal.test")
	auth := NewMerchantAuth(st, cfg.MerchantEmail, cfg.MerchantPasswordHash, "https://fairdeal.test")
	ai := NewAIHandler(nil)
	server := NewServer(st, cfg, nil, handler, ai, NewMerchantHandler(st, nil, false, auth))
	credentials := map[string]MerchantCredential{}
	for _, d := range DemoMerchants {
		if d.ID != "alpine" {
			credentials[d.ID] = MerchantCredential{d.ID + "@fairdeal.demo", cfg.MerchantPasswordHash}
		}
	}
	data, _ := json.Marshal(credentials)
	market, e := NewMarketplace(server, cfg, pp, ai, "https://fairdeal.test", string(data))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(market.Close)
	return market
}
func TestFiveMerchantOffersAreSeparateAndCaptureExactlyOnce(t *testing.T) {
	market := marketplaceFixture(t)
	rec := httptest.NewRecorder()
	market.ServeHTTP(rec, httptest.NewRequest("POST", "https://fairdeal.test/api/fairdeal/match", strings.NewReader(`{"budget":"200.00","shipping_flexibility":0.85,"willing_to_bundle":true}`)))
	if rec.Code != 200 {
		t.Fatalf("match %d %s", rec.Code, rec.Body.String())
	}
	var response WinWinMatchResponse
	if e := json.Unmarshal(rec.Body.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	if len(response.AllOffers) != 5 {
		t.Fatalf("expected five actual offers, got %d", len(response.AllOffers))
	}
	for _, offer := range response.AllOffers {
		body, _ := json.Marshal(CreateOrderRequest{AgreementID: offer.AgreementID, ClearingToken: offer.ClearingToken})
		orderResponse := httptest.NewRecorder()
		market.ServeHTTP(orderResponse, httptest.NewRequest("POST", "https://fairdeal.test/api/paypal/orders", bytes.NewReader(body)))
		if orderResponse.Code != 201 {
			t.Fatalf("create %s", orderResponse.Body.String())
		}
		var order CreateOrderResponse
		_ = json.Unmarshal(orderResponse.Body.Bytes(), &order)
		for i := 0; i < 2; i++ {
			captured := httptest.NewRecorder()
			market.ServeHTTP(captured, httptest.NewRequest("POST", "https://fairdeal.test/api/paypal/orders/"+order.OrderID+"/capture", nil))
			if captured.Code != 200 {
				t.Fatalf("capture %d %s", captured.Code, captured.Body.String())
			}
		}
	}
	for _, tenant := range market.tenants {
		metrics, e := tenant.server.Store.GetMetrics(context.Background())
		if e != nil || metrics.Captured != 1 {
			t.Fatalf("tenant captures: %+v %v", metrics, e)
		}
	}
}
func TestSellerProjectionDoesNotExposeBuyerPrivateData(t *testing.T) {
	market := marketplaceFixture(t)
	match := httptest.NewRecorder()
	market.ServeHTTP(match, httptest.NewRequest("POST", "https://fairdeal.test/api/fairdeal/match", strings.NewReader(`{"budget":"173.21","shipping_flexibility":0.85}`)))
	if match.Code != 200 {
		t.Fatal(match.Body.String())
	}
	cookie, _ := loginMerchant(t, market, "correct horse battery staple 123")
	req := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/agreements", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	market.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	for _, private := range []string{"buyer_budget", "buyer_surplus", "seller_surplus", "session_id", "negotiation_id", "173.21", "17321"} {
		if strings.Contains(rec.Body.String(), private) {
			t.Fatalf("private field exposed: %s", private)
		}
	}
	var agreements []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &agreements)
	if len(agreements) != 1 {
		t.Fatalf("cross-tenant agreements: %s", rec.Body.String())
	}
	// Present an Alpine cookie directly to Nordic: it cannot authorize any operation.
	request := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/agreements", nil)
	request.AddCookie(cookie)
	cross := httptest.NewRecorder()
	market.tenants[1].server.Mux.ServeHTTP(cross, request)
	if cross.Code != http.StatusUnauthorized {
		t.Fatalf("cross-tenant session accepted: %d", cross.Code)
	}
}

func TestPublicQuotationsDoNotDependOnPrivateBudget(t *testing.T) {
	market := marketplaceFixture(t)
	prices := map[string]string{}
	for _, budget := range []string{"173.21", "198.76"} {
		rec := httptest.NewRecorder()
		market.ServeHTTP(rec, httptest.NewRequest("POST", "/api/fairdeal/match", strings.NewReader(`{"budget":"`+budget+`","shipping_flexibility":0.85,"willing_to_bundle":true}`)))
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
		var result WinWinMatchResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		if len(result.AllOffers) != 5 {
			t.Fatal("missing merchant")
		}
		for _, offer := range result.AllOffers {
			if previous, ok := prices[offer.MerchantName]; ok && previous != offer.FinalPrice {
				t.Fatal("budget changed public quotation")
			}
			prices[offer.MerchantName] = offer.FinalPrice
		}
	}
	for _, tenant := range market.tenants {
		agreements, e := tenant.server.Store.GetRecentAgreements(context.Background(), 100)
		if e != nil {
			t.Fatal(e)
		}
		for _, a := range agreements {
			if a.BuyerBudgetCents != 0 || a.BuyerSurplusCents != 0 {
				t.Fatal("private budget or derived utility persisted in merchant database")
			}
		}
	}
}

func TestNordicLoginSelectsOnlyNordicPolicy(t *testing.T) {
	market := marketplaceFixture(t)
	pre := httptest.NewRecorder()
	market.ServeHTTP(pre, httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/auth/csrf", nil))
	var token map[string]string
	_ = json.Unmarshal(pre.Body.Bytes(), &token)
	request := httptest.NewRequest("POST", "https://fairdeal.test/api/merchant/login", strings.NewReader(`{"email":"nordic@fairdeal.demo","password":"correct horse battery staple 123"}`))
	request.Header.Set("Origin", "https://fairdeal.test")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", token["csrf_token"])
	for _, cookie := range pre.Result().Cookies() {
		request.AddCookie(cookie)
	}
	logged := httptest.NewRecorder()
	market.ServeHTTP(logged, request)
	if logged.Code != 200 {
		t.Fatal(logged.Body.String())
	}
	policyRequest := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/policy", nil)
	for _, cookie := range logged.Result().Cookies() {
		policyRequest.AddCookie(cookie)
	}
	policy := httptest.NewRecorder()
	market.ServeHTTP(policy, policyRequest)
	if policy.Code != 200 || !strings.Contains(policy.Body.String(), `"min_margin_pct":0.22`) {
		t.Fatalf("wrong tenant policy: %s", policy.Body.String())
	}
}

func TestPublicConcessionTiersAllowAffordablePackagesWithoutExposingBudget(t *testing.T) {
	market := marketplaceFixture(t)
	for _, budget := range []string{"105.00", "90.00"} {
		rec := httptest.NewRecorder()
		market.ServeHTTP(rec, httptest.NewRequest("POST", "/api/fairdeal/match", strings.NewReader(`{"budget":"`+budget+`","shipping_flexibility":0.85,"willing_to_bundle":true}`)))
		if rec.Code != 200 {
			t.Fatalf("affordable scenario %s: %s", budget, rec.Body.String())
		}
		var result WinWinMatchResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		if len(result.AllOffers) == 0 {
			t.Fatal("missing affordable offers")
		}
		for _, offer := range result.AllOffers {
			if numericPrice(offer.FinalPrice) > numericPrice(budget) {
				t.Fatal("offer exceeds private maximum")
			}
		}
	}
}
