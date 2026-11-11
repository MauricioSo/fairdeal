package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	passwordauth "github.com/nashpay/nashpay/internal/auth"
	"github.com/nashpay/nashpay/internal/config"
	"github.com/nashpay/nashpay/internal/store"
)

var passwordOnce sync.Once
var passwordHash string

func testPasswordHash(t *testing.T) string {
	t.Helper()
	passwordOnce.Do(func() {
		var err error
		passwordHash, err = passwordauth.HashPassword("correct horse battery staple 123")
		if err != nil {
			t.Fatal(err)
		}
	})
	return passwordHash
}

func loginMerchant(t *testing.T, handler http.Handler, password string) (*http.Cookie, string) {
	t.Helper()
	pre := httptest.NewRecorder()
	handler.ServeHTTP(pre, httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/auth/csrf", nil))
	if pre.Code != 200 {
		t.Fatalf("pre-session: %d %s", pre.Code, pre.Body.String())
	}
	var state map[string]string
	json.Unmarshal(pre.Body.Bytes(), &state)
	body, _ := json.Marshal(map[string]string{"email": "merchant@fairdeal.demo", "password": password})
	req := httptest.NewRequest("POST", "https://fairdeal.test/api/merchant/login", bytes.NewReader(body))
	req.Header.Set("Origin", "https://fairdeal.test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", state["csrf_token"])
	for _, c := range pre.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &state)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "__Host-fairdeal_merchant" {
			return c, state["csrf_token"]
		}
	}
	t.Fatal("missing session cookie")
	return nil, ""
}

func merchantFixture(t *testing.T) (*store.Store, *MerchantAuth, *http.ServeMux) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "merchant.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	auth := NewMerchantAuth(st, "merchant@fairdeal.demo", testPasswordHash(t), "https://fairdeal.test")
	m := NewMerchantHandler(st, nil, false, auth)
	server := NewServer(st, &config.Config{}, nil, nil, nil, m)
	return st, auth, server.Mux
}

func TestMerchantLoginProtectsAllEntryPoints(t *testing.T) {
	_, _, mux := merchantFixture(t)
	for _, target := range []string{"/merchant", "/merchant.html", "/merchant.html/", "/%6derchant.html"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "https://fairdeal.test"+target, nil))
		if rec.Code != 303 || rec.Header().Get("Location") != "/merchant/login" || strings.Contains(rec.Body.String(), "Live Clearing") {
			t.Errorf("page bypass %s: %d", target, rec.Code)
		}
	}
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/merchant/policy"}, {"GET", "/api/merchant/metrics"}, {"GET", "/api/merchant/agreements"},
		{"POST", "/api/merchant/policy/compile"}, {"POST", "/api/merchant/policy/1/activate"}, {"POST", "/api/merchant/demo/reset"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(route.method, "https://fairdeal.test"+route.path, nil)
		req.Header.Set("X-Merchant-Token", "old-token")
		mux.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Errorf("API allowed legacy token: %s %d", route.path, rec.Code)
		}
	}
	cookie, csrf := loginMerchant(t, mux, "correct horse battery staple 123")
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge != 14400 {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	if cookie.Value == csrf || !validSecret(cookie.Value) {
		t.Fatal("invalid session separation")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://fairdeal.test/merchant.html", nil)
	req.AddCookie(cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("protected page: %d %+v", rec.Code, rec.Header())
	}
}

func TestMerchantSessionRevocationAndCSRF(t *testing.T) {
	st, _, mux := merchantFixture(t)
	cookie, csrf := loginMerchant(t, mux, "correct horse battery staple 123")
	var stored string
	if err := st.DB().QueryRow(`SELECT session_hash FROM merchant_sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == cookie.Value || stored != secretHash(cookie.Value) {
		t.Fatal("raw session secret stored in database")
	}
	for _, tc := range []struct {
		origin, csrf string
		want         int
	}{
		{"https://evil.example", csrf, 403}, {"https://fairdeal.test", "bad", 403}, {"", csrf, 403}, {"https://fairdeal.test", csrf, 204},
	} {
		req := httptest.NewRequest("POST", "https://fairdeal.test/api/merchant/logout", nil)
		req.AddCookie(cookie)
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("X-CSRF-Token", tc.csrf)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("CSRF/logout: got %d want %d", rec.Code, tc.want)
		}
	}
	req := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/metrics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal("logout session replay succeeded")
	}
}

func TestMerchantSessionExpiryAndCredentialRotation(t *testing.T) {
	for _, mode := range []string{"idle", "absolute", "credential-rotation"} {
		t.Run(mode, func(t *testing.T) {
			_, auth, mux := merchantFixture(t)
			now := time.Now()
			auth.clock = func() time.Time { return now }
			cookie, _ := loginMerchant(t, mux, "correct horse battery staple 123")
			if mode == "idle" {
				now = now.Add(1801 * time.Second)
			}
			if mode == "absolute" {
				for i := 0; i < 8; i++ {
					now = now.Add(1700 * time.Second)
					req := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/session", nil)
					req.AddCookie(cookie)
					mux.ServeHTTP(httptest.NewRecorder(), req)
				}
				now = now.Add(1001 * time.Second)
			}
			if mode == "credential-rotation" {
				auth.version = "changed-password-verifier"
			}
			req := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/session", nil)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != 401 {
				t.Fatalf("%s session remained active", mode)
			}
		})
	}
}

func TestMerchantLoginFailsClosedAndResistsCSRF(t *testing.T) {
	st, auth, mux := merchantFixture(t)
	for _, origin := range []string{"", "https://evil.example"} {
		req := httptest.NewRequest("POST", "https://fairdeal.test/api/merchant/login", strings.NewReader(`{"email":"merchant@fairdeal.demo","password":"correct horse battery staple 123"}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatal("login CSRF not rejected")
		}
	}
	auth.configured = false
	req := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatal("unconfigured auth did not fail closed")
	}
	for _, origin := range []string{"http://fairdeal.example", "https://fairdeal.test/path", "https://fairdeal.test?bad=1"} {
		if NewMerchantAuth(st, "merchant@fairdeal.demo", testPasswordHash(t), origin).configured {
			t.Fatalf("unsafe origin accepted: %s", origin)
		}
	}
}

func TestMerchantLoginRateLimitPersists(t *testing.T) {
	st, auth, mux := merchantFixture(t)
	pre := httptest.NewRecorder()
	mux.ServeHTTP(pre, httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/auth/csrf", nil))
	var state map[string]string
	json.Unmarshal(pre.Body.Bytes(), &state)
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("POST", "https://fairdeal.test/api/merchant/login", strings.NewReader(`{"email":"merchant@fairdeal.demo","password":"wrong"}`))
		for _, c := range pre.Result().Cookies() {
			req.AddCookie(c)
		}
		req.Header.Set("Origin", "https://fairdeal.test")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", state["csrf_token"])
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		want := 401
		if i == 10 {
			want = 429
		}
		if rec.Code != want {
			t.Fatalf("attempt %d: %d want %d", i, rec.Code, want)
		}
	}
	other := NewMerchantAuth(st, auth.email, auth.passwordHash, auth.origin)
	if allowed, err := other.store.MerchantLoginAttempt(context.Background(), "merchant-login-ip-"+secretHash("192.0.2.1"), time.Now().Unix(), 10); err != nil || allowed {
		t.Fatal("new auth instance reset durable rate limit")
	}
}

func TestMerchantSessionSurvivesServiceRestart(t *testing.T) {
	st, auth, mux := merchantFixture(t)
	cookie, _ := loginMerchant(t, mux, "correct horse battery staple 123")
	replacement := NewMerchantAuth(st, auth.email, auth.passwordHash, auth.origin)
	sessionMux := http.NewServeMux()
	replacement.Register(sessionMux)
	req := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/session", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	sessionMux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal("valid persisted session lost on service restart")
	}
}

func TestMerchantLoginRotatesExistingSession(t *testing.T) {
	_, _, mux := merchantFixture(t)
	old, _ := loginMerchant(t, mux, "correct horse battery staple 123")
	pre := httptest.NewRecorder()
	mux.ServeHTTP(pre, httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/auth/csrf", nil))
	var state map[string]string
	json.Unmarshal(pre.Body.Bytes(), &state)
	req := httptest.NewRequest("POST", "https://fairdeal.test/api/merchant/login", strings.NewReader(`{"email":"merchant@fairdeal.demo","password":"correct horse battery staple 123"}`))
	req.AddCookie(old)
	for _, c := range pre.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set("Origin", "https://fairdeal.test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", state["csrf_token"])
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal("second login failed")
	}
	probe := httptest.NewRequest("GET", "https://fairdeal.test/api/merchant/session", nil)
	probe.AddCookie(old)
	result := httptest.NewRecorder()
	mux.ServeHTTP(result, probe)
	if result.Code != 401 {
		t.Fatal("old browser session survived login rotation")
	}
}
