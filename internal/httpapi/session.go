package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

const SessionCookieName = "nashpay_sid"

// RandomToken returns prefix + hex(n random bytes) from crypto/rand.
func RandomToken(prefix string, n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func GenerateSessionID() string { return RandomToken("", 16) }

// ValidSessionID accepts only the format we issue: 32 lowercase hex characters.
func ValidSessionID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// GetOrCreateSessionID retrieves the session cookie or sets a new one.
func GetOrCreateSessionID(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(SessionCookieName); err == nil && ValidSessionID(cookie.Value) {
		return cookie.Value
	}

	sid := GenerateSessionID()
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30, // 30 days
	})
	return sid
}

// GetSessionID extracts the session ID from the cookie without writing a response.
func GetSessionID(r *http.Request) string {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}
