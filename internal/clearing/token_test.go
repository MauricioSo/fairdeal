package clearing

import (
	"errors"
	"testing"
)

func TestTokenSigningAndVerification(t *testing.T) {
	secret := []byte("01234567890123456789012345678901") // 32 bytes
	bundle := "SKU-SCARF-09"
	now := int64(1000)

	payload := AgreementTokenPayload{
		AgreementID:   "agr_test_123",
		PriceCents:    9400,
		Shipping:      ShippingConsolidatedGround,
		PayLater:      true,
		BundleSKU:     &bundle,
		ExpiresAtUnix: now + 300,
	}

	token := Sign(payload, secret)
	if token == "" {
		t.Fatal("token should not be empty")
	}

	// 1. Valid token
	if err := Verify(payload, token, secret, now); err != nil {
		t.Fatalf("expected valid token, got: %v", err)
	}

	// 2. Tampered token (change 1 char)
	tamperedToken := token[:len(token)-1] + "f"
	if token[len(token)-1] == 'f' {
		tamperedToken = token[:len(token)-1] + "e"
	}
	if err := Verify(payload, tamperedToken, secret, now); !errors.Is(err, ErrBadToken) {
		t.Fatalf("expected ErrBadToken for tampered token, got %v", err)
	}

	// 3. Tampered payload (altered price, e.g. lower price attack)
	attackPayload := payload
	attackPayload.PriceCents = 5000
	if err := Verify(attackPayload, token, secret, now); !errors.Is(err, ErrBadToken) {
		t.Fatalf("expected ErrBadToken for tampered price, got %v", err)
	}

	// 4. Expired token
	futureTime := now + 400
	if err := Verify(payload, token, secret, futureTime); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}
