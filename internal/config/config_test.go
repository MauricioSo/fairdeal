package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	// Clear relevant env vars
	keys := []string{
		"PORT", "DB_PATH", "PAYPAL_MODE", "PAYPAL_CLIENT_ID", "PAYPAL_CLIENT_SECRET",
		"NASHPAY_HMAC_SECRET", "LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL",
	}
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.PayPalMode != "mock" {
		t.Errorf("PayPalMode = %q, want 'mock'", cfg.PayPalMode)
	}
	if cfg.AIMode != "heuristic" {
		t.Errorf("AIMode = %q, want 'heuristic'", cfg.AIMode)
	}
	if len(cfg.HMACSecret) != 32 {
		t.Errorf("HMACSecret length = %d, want 32", len(cfg.HMACSecret))
	}
}

func TestConfigFromDotEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	content := `
# Test env
PORT=9090
PAYPAL_MODE=mock
LLM_BASE_URL=https://api.openai.com/v1
LLM_API_KEY=test-key
LLM_MODEL=gpt-4o-mini
`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.AIMode != "llm" {
		t.Errorf("AIMode = %q, want 'llm'", cfg.AIMode)
	}
}
