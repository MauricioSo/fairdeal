package kernel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNeverClaimsVerificationWithoutPriceEvidence(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`{"id":"browser-created"}`)) }))
	defer server.Close()
	for _, key := range []string{"", "configured-key"} {
		c := NewClient(server.URL, key)
		v, err := c.VerifyMarketPrice(context.Background(), "SKU", "97.00")
		if err != nil || v.Verified || v.LiveMSRP != "" || v.BrowserSessionID != "" || v.LatencyMs != 0 {
			t.Fatalf("fabricated evidence: %+v, %v", v, err)
		}
	}
	if calls != 0 {
		t.Fatalf("spent browser credits without a price source: %d calls", calls)
	}
}
