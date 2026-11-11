package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/nashpay/nashpay/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMerchantEconomicSimulationAndManualPolicy(t *testing.T) {
	st, _, mux := merchantFixture(t)
	unauth := httptest.NewRecorder()
	mux.ServeHTTP(unauth, httptest.NewRequest("POST", "https://fairdeal.test/api/merchant/economics/simulate", strings.NewReader(`{}`)))
	if unauth.Code != 401 {
		t.Fatalf("unauthenticated simulation %d", unauth.Code)
	}
	cookie, csrf := loginMerchant(t, mux, "correct horse battery staple 123")
	call := func(path string, body any, token string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "https://fairdeal.test"+path, bytes.NewReader(data))
		req.AddCookie(cookie)
		req.Header.Set("Origin", "https://fairdeal.test")
		req.Header.Set("X-CSRF-Token", token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	active, _ := st.GetActivePolicy(context.Background())
	cfg := active.Policy
	cfg.Economics.MinContributionCents = 999999
	result := call("/api/merchant/economics/simulate", map[string]any{"sku": "CH-WINTER-PARKA-01", "price_cents": 10000, "policy": cfg, "budget": 12345}, csrf)
	if result.Code != 200 || !strings.Contains(result.Body.String(), "CONTRIBUTION_BELOW_MINIMUM") || strings.Contains(result.Body.String(), "budget") {
		t.Fatalf("simulation %d %s", result.Code, result.Body.String())
	}
	var count int
	st.DB().QueryRow("SELECT count(*) FROM agreements").Scan(&count)
	if count != 0 {
		t.Fatal("simulation created binding agreement")
	}
	cfg.Economics.MinContributionCents = 1000
	if result = call("/api/merchant/policy/manual", cfg, ""); result.Code != 403 {
		t.Fatal("CSRF bypass")
	}
	result = call("/api/merchant/policy/manual", cfg, csrf)
	if result.Code != 201 {
		t.Fatalf("draft %d %s", result.Code, result.Body.String())
	}
	var response struct {
		Draft store.MerchantPolicy `json:"draft"`
	}
	json.Unmarshal(result.Body.Bytes(), &response)
	stillActive, _ := st.GetActivePolicy(context.Background())
	if stillActive.ID != active.ID {
		t.Fatal("draft activated silently")
	}
	if response.Draft.Policy.Economics.MinContributionCents != 1000 {
		t.Fatal("rules not persisted")
	}
	cfg.Economics.PlatformFeeBps = -1
	if result = call("/api/merchant/policy/manual", cfg, csrf); result.Code != 400 {
		t.Fatal("negative fee accepted")
	}
	cfg = active.Policy
	cfg.MinMarginPct = .01
	if result = call("/api/merchant/policy/manual", cfg, csrf); result.Code != 400 {
		t.Fatal("hard floor bypass")
	}
}
