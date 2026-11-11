package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                  int
	DBPath                string
	PayPalMode            string // "mock" or "sandbox"
	PayPalClientID        string
	PayPalClientSecret    string
	PayPalWebhookID       string
	HMACSecret            []byte
	LLMBaseURL            string
	LLMAPIKey             string
	LLMModel              string
	CommitCooldownSeconds int
	DemoMode              bool
	MerchantEmail         string
	MerchantPasswordHash  string
	AIMode                string // "llm" or "heuristic"
	KernelAPIURL          string
	KernelAPIKey          string
}

// Load loads the configuration from environment variables (after loading .env).
func Load(dotEnvPath string) (*Config, error) {
	if dotEnvPath != "" {
		_ = LoadDotEnv(dotEnvPath, false)
	}

	cfg := &Config{
		Port:                  8080,
		DBPath:                "./nashpay.db",
		PayPalMode:            "mock",
		CommitCooldownSeconds: 30,
		DemoMode:              true,
		AIMode:                "heuristic",
	}

	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p < 65536 {
			cfg.Port = p
		}
	}

	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		cfg.DBPath = dbPath
	}

	if ppMode := strings.ToLower(os.Getenv("PAYPAL_MODE")); ppMode != "" {
		if ppMode == "sandbox" || ppMode == "mock" {
			cfg.PayPalMode = ppMode
		}
	}

	cfg.PayPalClientID = os.Getenv("PAYPAL_CLIENT_ID")
	cfg.PayPalClientSecret = os.Getenv("PAYPAL_CLIENT_SECRET")
	cfg.PayPalWebhookID = os.Getenv("PAYPAL_WEBHOOK_ID")

	// HMAC Secret
	hmacHex := os.Getenv("NASHPAY_HMAC_SECRET")
	if hmacHex != "" {
		sec, err := hex.DecodeString(hmacHex)
		if err == nil && len(sec) >= 32 {
			cfg.HMACSecret = sec
		} else {
			// If not valid hex or < 32 bytes
			if len([]byte(hmacHex)) >= 32 {
				cfg.HMACSecret = []byte(hmacHex)
			}
		}
	}

	if len(cfg.HMACSecret) < 32 {
		if cfg.PayPalMode == "sandbox" {
			return nil, errors.New("NASHPAY_HMAC_SECRET must be at least 32 bytes in sandbox mode")
		}
		// In mock mode, generate random 32 bytes and log warning
		sec := make([]byte, 32)
		_, _ = rand.Read(sec)
		cfg.HMACSecret = sec
		slog.Warn("NASHPAY_HMAC_SECRET not set or < 32 bytes; generated ephemeral random 32-byte secret for mock mode")
	}

	cfg.LLMBaseURL = strings.TrimRight(os.Getenv("LLM_BASE_URL"), "/")
	cfg.LLMAPIKey = os.Getenv("LLM_API_KEY")
	cfg.LLMModel = os.Getenv("LLM_MODEL")

	if cfg.LLMBaseURL != "" && cfg.LLMAPIKey != "" && cfg.LLMModel != "" {
		cfg.AIMode = "llm"
	} else {
		cfg.AIMode = "heuristic"
	}

	if cooldownStr := os.Getenv("COMMIT_COOLDOWN_SECONDS"); cooldownStr != "" {
		if c, err := strconv.Atoi(cooldownStr); err == nil && c >= 0 {
			cfg.CommitCooldownSeconds = c
		}
	}

	if demoStr := os.Getenv("DEMO_MODE"); demoStr != "" {
		cfg.DemoMode = demoStr == "1" || strings.ToLower(demoStr) == "true"
	}

	cfg.MerchantEmail = os.Getenv("MERCHANT_EMAIL")
	cfg.MerchantPasswordHash = os.Getenv("MERCHANT_PASSWORD_HASH")

	cfg.KernelAPIURL = os.Getenv("KERNEL_API_URL")
	if cfg.KernelAPIURL == "" {
		cfg.KernelAPIURL = "https://api.onkernel.com"
	}
	cfg.KernelAPIKey = os.Getenv("KERNEL_API_KEY")

	return cfg, nil
}

// LogSummary logs non-sensitive startup info.
func (c *Config) LogSummary() {
	slog.Info("Fairdeal configuration loaded",
		"port", c.Port,
		"db_path", c.DBPath,
		"paypal_mode", c.PayPalMode,
		"ai_mode", c.AIMode,
		"demo_mode", c.DemoMode,
		"kernel_cloud", c.KernelAPIKey != "",
		"cooldown_s", c.CommitCooldownSeconds,
	)
}
