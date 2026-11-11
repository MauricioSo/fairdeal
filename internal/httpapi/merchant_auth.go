package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	passwordauth "github.com/nashpay/nashpay/internal/auth"
	"github.com/nashpay/nashpay/internal/store"
)

type MerchantAuth struct {
	MerchantName                         string
	store                                *store.Store
	email, passwordHash, version, origin string
	secure, configured                   bool
	clock                                func() time.Time
	work                                 chan struct{}
}

func NewMerchantAuth(st *store.Store, email, passwordHash, origin string) *MerchantAuth {
	email = strings.ToLower(strings.TrimSpace(email))
	origin = strings.TrimRight(origin, "/")
	address, emailErr := mail.ParseAddress(email)
	u, urlErr := url.Parse(origin)
	configured := emailErr == nil && address.Address == email && passwordauth.ValidHash(passwordHash) && urlErr == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")))
	version := sha256.Sum256([]byte(email + "\x00" + passwordHash))
	return &MerchantAuth{store: st, email: email, passwordHash: passwordHash, origin: origin, version: hex.EncodeToString(version[:]), secure: strings.HasPrefix(origin, "https://"), configured: configured, clock: time.Now, work: make(chan struct{}, 2)}
}

func (a *MerchantAuth) cookieName() string {
	if a.secure {
		return "__Host-fairdeal_merchant"
	}
	return "fairdeal_merchant"
}
func (a *MerchantAuth) preCookieName() string {
	if a.secure {
		return "__Host-fairdeal_login"
	}
	return "fairdeal_login"
}
func authHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "DENY")
}
func (a *MerchantAuth) setCookie(w http.ResponseWriter, name, value string, age int) {
	cookie := &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode, MaxAge: age}
	if age < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, cookie)
}
func newSecret() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func secretHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func validSecret(token string) bool {
	v, err := hex.DecodeString(token)
	return err == nil && len(v) == 32
}
func sameSecret(a, b string) bool {
	return validSecret(a) && validSecret(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (a *MerchantAuth) sameOrigin(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json" && r.Header.Get("Origin") == a.origin && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}
func (a *MerchantAuth) unavailable(w http.ResponseWriter) bool {
	if a == nil || !a.configured {
		writeJSONError(w, http.StatusServiceUnavailable, "LOGIN_UNAVAILABLE", "merchant login is not configured")
		return true
	}
	return false
}
func (a *MerchantAuth) session(r *http.Request) (*store.MerchantSession, error) {
	if a == nil || !a.configured {
		return nil, store.ErrNotFound
	}
	c, err := r.Cookie(a.cookieName())
	if err != nil || !validSecret(c.Value) {
		return nil, store.ErrNotFound
	}
	return a.store.MerchantSession(r.Context(), secretHash(c.Value), a.version, a.clock().Unix())
}

func (a *MerchantAuth) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/merchant/auth/csrf", a.loginCSRF)
	mux.HandleFunc("POST /api/merchant/login", a.login)
	mux.HandleFunc("GET /api/merchant/session", a.sessionInfo)
	mux.HandleFunc("POST /api/merchant/logout", a.Guard(a.logout))
}
func (a *MerchantAuth) loginCSRF(w http.ResponseWriter, r *http.Request) {
	authHeaders(w)
	if a.unavailable(w) {
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeJSONError(w, 403, "FORBIDDEN", "same-origin request required")
		return
	}
	token, err := newSecret()
	if err != nil {
		writeJSONError(w, 503, "UNAVAILABLE", "please retry")
		return
	}
	a.setCookie(w, a.preCookieName(), token, 300)
	writeJSON(w, 200, map[string]string{"csrf_token": token})
}
func (a *MerchantAuth) login(w http.ResponseWriter, r *http.Request) {
	authHeaders(w)
	if a.unavailable(w) {
		return
	}
	pre, err := r.Cookie(a.preCookieName())
	if !a.sameOrigin(r) || err != nil || !sameSecret(pre.Value, r.Header.Get("X-CSRF-Token")) {
		writeJSONError(w, 403, "FORBIDDEN", "reload the login page and try again")
		return
	}
	var credentials struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&credentials) != nil || len(credentials.Email) > 254 || len(credentials.Password) > 1024 {
		writeJSONError(w, 400, "INVALID", "invalid login request")
		return
	}
	now := a.clock().Unix()
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	// Render terminates TLS. The global limit still applies if client IP headers are spoofed.
	if a.secure {
		if forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]; net.ParseIP(strings.TrimSpace(forwarded)) != nil {
			peer = strings.TrimSpace(forwarded)
		}
	}
	for _, bucket := range []struct {
		name  string
		limit int
	}{{"merchant-login-global", 30}, {"merchant-login-ip-" + secretHash(peer), 10}} {
		allowed, e := a.store.MerchantLoginAttempt(r.Context(), bucket.name, now, bucket.limit)
		if e != nil {
			writeJSONError(w, 503, "UNAVAILABLE", "please retry later")
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", "900")
			writeJSONError(w, 429, "RATE_LIMITED", "too many login attempts; try again later")
			return
		}
	}
	select {
	case a.work <- struct{}{}:
		defer func() { <-a.work }()
	default:
		w.Header().Set("Retry-After", "5")
		writeJSONError(w, 429, "RATE_LIMITED", "please retry shortly")
		return
	}
	validPassword := passwordauth.VerifyPassword(credentials.Password, a.passwordHash)
	suppliedEmail := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(credentials.Email))))
	expectedEmail := sha256.Sum256([]byte(a.email))
	if subtle.ConstantTimeCompare(suppliedEmail[:], expectedEmail[:]) != 1 || !validPassword {
		writeJSONError(w, 401, "INVALID_CREDENTIALS", "incorrect email or password")
		return
	}
	token, e1 := newSecret()
	csrf, e2 := newSecret()
	if e1 != nil || e2 != nil {
		writeJSONError(w, 503, "UNAVAILABLE", "please retry")
		return
	}
	if err := a.store.CreateMerchantSession(r.Context(), secretHash(token), csrf, a.version, now); err != nil {
		writeJSONError(w, 503, "UNAVAILABLE", "please retry")
		return
	}
	if old, e := r.Cookie(a.cookieName()); e == nil {
		_ = a.store.DeleteMerchantSession(r.Context(), secretHash(old.Value))
	}
	a.setCookie(w, a.cookieName(), token, 4*3600)
	a.setCookie(w, a.preCookieName(), "", -1)
	writeJSON(w, 200, map[string]string{"email": a.email, "csrf_token": csrf})
}
func (a *MerchantAuth) sessionInfo(w http.ResponseWriter, r *http.Request) {
	authHeaders(w)
	session, err := a.session(r)
	if err != nil {
		writeJSONError(w, 401, "LOGIN_REQUIRED", "sign in to continue")
		return
	}
	writeJSON(w, 200, map[string]any{"email": a.email, "csrf_token": session.CSRF, "expires_at": session.ExpiresAt, "merchant_name": a.MerchantName})
}
func (a *MerchantAuth) Guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if a.unavailable(w) {
			return
		}
		session, err := a.session(r)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, 401, "LOGIN_REQUIRED", "sign in to continue")
			} else {
				writeJSONError(w, 503, "UNAVAILABLE", "please retry")
			}
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && (!a.sameOrigin(r) || !sameSecret(session.CSRF, r.Header.Get("X-CSRF-Token"))) {
			writeJSONError(w, 403, "FORBIDDEN", "invalid request verification")
			return
		}
		next(w, r)
	}
}
func (a *MerchantAuth) Page(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
		if _, err := a.session(r); err != nil {
			http.Redirect(w, r, "/merchant/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
func (a *MerchantAuth) logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(a.cookieName())
	if err := a.store.DeleteMerchantSession(r.Context(), secretHash(cookie.Value)); err != nil {
		writeJSONError(w, 503, "UNAVAILABLE", "please retry")
		return
	}
	a.setCookie(w, a.cookieName(), "", -1)
	w.WriteHeader(http.StatusNoContent)
}
