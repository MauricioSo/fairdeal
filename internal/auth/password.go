// Package auth contains the merchant password verifier. Passwords are never stored.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

const Iterations = 600000

func HashPassword(password string) (string, error) {
	if len(password) < 16 || len(password) > 1024 {
		return "", errors.New("password must contain 16 to 1024 bytes")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, Iterations, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2-sha256-600000$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(key), nil
}

func ValidHash(encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "pbkdf2-sha256-600000" {
		return false
	}
	salt, e1 := hex.DecodeString(parts[1])
	key, e2 := hex.DecodeString(parts[2])
	return e1 == nil && e2 == nil && len(salt) == 16 && len(key) == 32
}

func VerifyPassword(password, encoded string) bool {
	if !ValidHash(encoded) || len(password) > 1024 {
		return false
	}
	parts := strings.Split(encoded, "$")
	salt, _ := hex.DecodeString(parts[1])
	expected, _ := hex.DecodeString(parts[2])
	key, err := pbkdf2.Key(sha256.New, password, salt, Iterations, 32)
	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1
}
