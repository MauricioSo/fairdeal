package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nashpay/nashpay/internal/ai"
	"github.com/nashpay/nashpay/internal/config"
	"github.com/nashpay/nashpay/internal/httpapi"
	"github.com/nashpay/nashpay/internal/kernel"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/paypal"
	"github.com/nashpay/nashpay/internal/store"
	"github.com/nashpay/nashpay/internal/ws"
)

func main() {
	// Structured JSON logging to stdout
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load(".env")
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	cfg.LogSummary()

	// Initialize SQLite Store
	ctx, cancelInit := context.WithTimeout(context.Background(), 10*time.Second)
	st, err := store.Open(ctx, cfg.DBPath)
	cancelInit()
	if err != nil {
		slog.Error("Failed to open SQLite database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Periodic cleanup of stale agreements (every 30 seconds)
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := store.NowUnix()
				if count, err := st.ExpireStale(cleanupCtx, now); err == nil && count > 0 {
					slog.Debug("Cleaned up expired agreements", "count", count)
				}
			case <-cleanupCtx.Done():
				return
			}
		}
	}()

	// Negotiation Service & Rate Limiter
	ttl := int64(300)
	negSvc := negotiation.NewService(st, cfg.HMACSecret, ttl)
	rateLimiter := negotiation.NewRateLimiter(cfg.CommitCooldownSeconds)
	wsHandler := ws.NewHandler(negSvc, rateLimiter, st)

	// PayPal Client (Mock or Sandbox)
	var ppClient paypal.Client
	switch {
	case cfg.PayPalMode == "sandbox" && cfg.PayPalClientID != "" && cfg.PayPalClientSecret != "":
		ppClient = paypal.NewSandboxClient(cfg.PayPalClientID, cfg.PayPalClientSecret)
		slog.Info("PayPal Sandbox client active")
	case cfg.PayPalMode == "sandbox":
		slog.Error("PAYPAL_MODE=sandbox requires PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET")
		os.Exit(1)
	default:
		ppClient = paypal.NewMockClient()
		slog.Info("PayPal Mock client active (simulated mode)")
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("RENDER_EXTERNAL_URL")
	}
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	ppHandler := httpapi.NewPayPalHandler(st, ppClient, negSvc, cfg.PayPalMode, baseURL)

	// Kernel Cloud Browser Agent Client (api.onkernel.com)
	kernelClient := kernel.NewClient(cfg.KernelAPIURL, cfg.KernelAPIKey)
	ppHandler.SetKernelClient(kernelClient)
	if kernelClient.IsConfigured() {
		slog.Info("Kernel key configured; market verification not implemented", "url", cfg.KernelAPIURL)
	} else {
		slog.Info("Kernel market verification unavailable", "hint", "set KERNEL_API_KEY for api.onkernel.com")
	}

	// AI Client
	var llmClient *ai.LLMClient
	if cfg.AIMode == "llm" {
		llmClient = ai.NewLLMClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
		ppHandler.SetLLMClient(llmClient)
		slog.Info("LLM client active", "model", cfg.LLMModel)
	} else {
		slog.Info("AI heuristic rules engine active (offline fallback)")
	}
	aiHandler := httpapi.NewAIHandler(llmClient)

	// Merchant Handler
	mchtHandler := httpapi.NewMerchantHandler(st, llmClient, cfg.DemoMode, cfg.MerchantToken)

	// HTTP API Server
	apiServer := httpapi.NewServer(st, cfg, wsHandler, ppHandler, aiHandler, mchtHandler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           apiServer.Mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown channel
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Starting Fairdeal HTTP server", "addr", addr, "url", fmt.Sprintf("http://localhost:%d", cfg.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	slog.Info("Shutting down Fairdeal server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Graceful shutdown failed, forcing close", "error", err)
	} else {
		slog.Info("Server stopped cleanly")
	}
}
