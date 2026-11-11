package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/config"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/paypal"
	"github.com/nashpay/nashpay/internal/store"
)

func TestPayPalOrderCreationAndCapture(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_api_pp.db")

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	secret := []byte("01234567890123456789012345678901")
	negSvc := negotiation.NewService(st, secret, 300)
	mockPP := paypal.NewMockClient()
	ppHandler := NewPayPalHandler(st, mockPP, negSvc, "mock", "http://localhost:8080")
	aiHandler := NewAIHandler(nil)
	merchHandler := NewMerchantHandler(st, nil, true, "secret-token")
	cfg := &config.Config{PayPalMode: "mock", DemoMode: true}
	srv := NewServer(st, cfg, nil, ppHandler, aiHandler, merchHandler)

	// 1. Create OFFERED agreement via Commit
	buyer := clearing.BuyerConstraints{
		Budget:              10500,
		ShippingFlexibility: 0.85,
		PrefersPayLater:     true,
		WillingToBundle:     true,
	}
	commitResp, err := negSvc.Commit(ctx, "0123456789abcdef0123456789abcdef", "neg_1", "CH-WINTER-PARKA-01", buyer, false)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Tampered token attack
	tamperedBody, _ := json.Marshal(CreateOrderRequest{
		AgreementID:   commitResp.AgreementID,
		ClearingToken: "bad_token_123",
	})
	reqTampered := httptest.NewRequest("POST", "/api/paypal/orders", bytes.NewReader(tamperedBody))
	recTampered := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recTampered, reqTampered)
	if recTampered.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for bad token, got %d", recTampered.Code)
	}

	// 3. Valid Create Order
	validBody, _ := json.Marshal(CreateOrderRequest{
		AgreementID:   commitResp.AgreementID,
		ClearingToken: commitResp.ClearingToken,
	})
	reqCreate := httptest.NewRequest("POST", "/api/paypal/orders", bytes.NewReader(validBody))
	recCreate := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createResp CreateOrderResponse
	_ = json.NewDecoder(recCreate.Body).Decode(&createResp)
	if createResp.OrderID == "" {
		t.Fatal("expected order_id in response")
	}

	// 4. Capture Order
	reqCapture := httptest.NewRequest("POST", "/api/paypal/orders/"+createResp.OrderID+"/capture", nil)
	recCapture := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recCapture, reqCapture)
	if recCapture.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on capture, got %d: %s", recCapture.Code, recCapture.Body.String())
	}

	var captureResp CaptureOrderResponse
	_ = json.NewDecoder(recCapture.Body).Decode(&captureResp)
	if captureResp.Status != "COMPLETED" || captureResp.Amount != "97.00" {
		t.Errorf("unexpected capture response: %+v", captureResp)
	}

	// 4b. Idempotent Capture Order (US-11 AC5)
	reqCap2 := httptest.NewRequest("POST", "/api/paypal/orders/"+createResp.OrderID+"/capture", nil)
	recCap2 := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recCap2, reqCap2)
	if recCap2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on idempotent capture, got %d", recCap2.Code)
	}

	// 5. Verify Public Agreement Endpoint & database state
	reqPub := httptest.NewRequest("GET", "/api/agreements/"+commitResp.AgreementID+"/public", nil)
	recPub := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recPub, reqPub)
	if recPub.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on public agreement, got %d", recPub.Code)
	}

	// 6. Test Merchant Auth & Endpoints
	reqUnauth := httptest.NewRequest("GET", "/api/merchant/agreements", nil)
	recUnauth := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without X-Merchant-Token, got %d", recUnauth.Code)
	}

	reqAuth := httptest.NewRequest("GET", "/api/merchant/agreements", nil)
	reqAuth.Header.Set("X-Merchant-Token", "secret-token")
	recAuth := httptest.NewRecorder()
	srv.Mux.ServeHTTP(recAuth, reqAuth)
	if recAuth.Code != http.StatusOK {
		t.Errorf("expected 200 with valid X-Merchant-Token, got %d", recAuth.Code)
	}
}
