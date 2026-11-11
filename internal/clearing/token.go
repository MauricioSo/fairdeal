package clearing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrBadToken = errors.New("invalid or tampered clearing token")
	ErrExpired  = errors.New("clearing token has expired")
)

type AgreementTokenPayload struct {
	AgreementID   string
	PriceCents    int64
	Shipping      string
	PayLater      bool
	BundleSKU     *string
	ExpiresAtUnix int64
}

// Canonical formats the agreement payload for cryptographic signing.
func (a AgreementTokenPayload) Canonical() string {
	bundleStr := "-"
	if a.BundleSKU != nil && *a.BundleSKU != "" {
		bundleStr = *a.BundleSKU
	}
	return fmt.Sprintf("v1|%s|%d|%s|%t|%s|%d",
		a.AgreementID,
		a.PriceCents,
		a.Shipping,
		a.PayLater,
		bundleStr,
		a.ExpiresAtUnix,
	)
}

// Sign generates an HMAC-SHA256 hex signature over the canonical payload.
func Sign(payload AgreementTokenPayload, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload.Canonical()))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify validates that tokenHex matches the HMAC signature and is not expired.
func Verify(payload AgreementTokenPayload, tokenHex string, secret []byte, nowUnix int64) error {
	if nowUnix > payload.ExpiresAtUnix {
		return ErrExpired
	}

	gotBytes, err := hex.DecodeString(tokenHex)
	if err != nil {
		return ErrBadToken
	}

	wantHex := Sign(payload, secret)
	wantBytes, _ := hex.DecodeString(wantHex)

	if !hmac.Equal(wantBytes, gotBytes) {
		return ErrBadToken
	}

	return nil
}
