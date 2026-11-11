package paypal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSandboxOAuthAndOrderLifecycle(t *testing.T) {
	ctx := context.Background()
	var tokenCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/oauth2/token":
			atomic.AddInt32(&tokenCalls, 1)
			_, _ = w.Write([]byte(`{
				"access_token": "ACCESS-TOKEN-TEST-XYZ",
				"token_type": "Bearer",
				"expires_in": 3600
			}`))
		case "/v2/checkout/orders":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "SANDBOX-ORDER-123"}`))
		case "/v2/checkout/orders/SANDBOX-ORDER-123":
			_, _ = w.Write([]byte(`{
				"id": "SANDBOX-ORDER-123",
				"status": "APPROVED",
				"purchase_units": [{
					"custom_id": "agr_123",
					"amount": {"currency_code": "USD", "value": "94.00"}
				}]
			}`))
		case "/v2/checkout/orders/SANDBOX-ORDER-123/capture":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{
				"id": "SANDBOX-ORDER-123",
				"status": "COMPLETED",
				"purchase_units": [{
					"payments": {
						"captures": [{
							"id": "CAP-NESTED-999",
							"status": "COMPLETED"
						}]
					}
				}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewSandboxClient("test_client_id", "test_secret")
	client.baseURL = server.URL

	// 1. OAuth Token & Cache
	token1, err := client.getAccessToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	token2, err := client.getAccessToken(ctx)
	if err != nil || token2 != token1 {
		t.Fatalf("expected cached token, got %v / %s", err, token2)
	}
	if atomic.LoadInt32(&tokenCalls) != 1 {
		t.Errorf("expected 1 OAuth call, got %d", atomic.LoadInt32(&tokenCalls))
	}

	// 2. CreateOrder
	orderID, err := client.CreateOrder(ctx, OrderRequest{Intent: "CAPTURE"})
	if err != nil || orderID != "SANDBOX-ORDER-123" {
		t.Fatalf("CreateOrder failed: %v (id=%s)", err, orderID)
	}

	// 3. GetOrder
	details, err := client.GetOrder(ctx, orderID)
	if err != nil || details.Status != "APPROVED" || details.PurchaseUnits[0].CustomID != "agr_123" {
		t.Fatalf("GetOrder failed: %v (%+v)", err, details)
	}

	// 4. CaptureOrder (extracts nested capture ID CAP-NESTED-999)
	capRes, err := client.CaptureOrder(ctx, orderID, "req-id-1")
	if err != nil {
		t.Fatalf("CaptureOrder failed: %v", err)
	}
	if capRes.ID != "CAP-NESTED-999" || capRes.Status != "COMPLETED" {
		t.Errorf("expected nested capture ID CAP-NESTED-999, got %+v", capRes)
	}
}
