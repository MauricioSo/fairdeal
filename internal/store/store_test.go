package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreInitAndSeed(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_seed.db")

	// 1. Initial open and seed
	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	defer s.Close()

	// Verify products
	p1, err := s.GetProduct(ctx, "CH-WINTER-PARKA-01")
	if err != nil {
		t.Fatalf("failed to get parka: %v", err)
	}
	if p1.ListPriceCents != 12000 || p1.CostCents != 5000 {
		t.Errorf("parka unexpected values: %+v", p1)
	}

	p2, err := s.GetProduct(ctx, "SKU-SCARF-09")
	if err != nil {
		t.Fatalf("failed to get scarf: %v", err)
	}
	if !p2.IsDeadstock || p2.HoldingCostCents != 500 {
		t.Errorf("scarf unexpected values: %+v", p2)
	}

	// Verify market settings
	m, err := s.GetMarketSettings(ctx)
	if err != nil {
		t.Fatalf("failed to get market settings: %v", err)
	}
	if m.FeeBps != 349 || m.FeeFixedCents != 49 {
		t.Errorf("unexpected market settings: %+v", m)
	}

	// Verify active policy
	pol, err := s.GetActivePolicy(ctx)
	if err != nil {
		t.Fatalf("failed to get active policy: %v", err)
	}
	if pol.Status != "ACTIVE" || pol.Policy.MinMarginPct != 0.20 {
		t.Errorf("unexpected policy: %+v", pol)
	}

	// AC2: Reopen should be idempotent and not duplicate data
	_ = s.Close()
	s2, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer s2.Close()

	var prodCount int
	if err := s2.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM products").Scan(&prodCount); err != nil {
		t.Fatal(err)
	}
	if prodCount != 2 {
		t.Errorf("expected 2 products after reopen, got %d", prodCount)
	}
}

func TestStoreAgreementTransitionsAndConcurrency(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_agreements.db")

	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	pol, err := s.GetActivePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}

	agrID := "agr_test_01"
	agr := &AgreementRecord{
		ID:                 agrID,
		NegotiationID:      "neg_test_01",
		SessionID:          "sess_01",
		SKU:                "CH-WINTER-PARKA-01",
		PriceCents:         9400,
		ShippingTier:       "CONSOLIDATED_GROUND",
		PayLater:           true,
		BuyerBudgetCents:   10500,
		StandardTotalCents: 13500,
		FeeCents:           377,
		ContributionCents:  3623,
		BuyerSurplusCents:  2450,
		SellerSurplusCents: 4123,
		PolicyID:           pol.ID,
		SolverLatencyUs:    400,
		ExpiresAt:          NowUnix() + 300,
	}

	if err := s.CreateAgreement(ctx, agr); err != nil {
		t.Fatalf("failed to create agreement: %v", err)
	}

	// Invalid transition test: OFFERED -> CAPTURED directly is forbidden
	err = s.TransitionAgreement(ctx, agrID, StatusOffered, StatusCaptured, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid transition OFFERED -> CAPTURED")
	}

	// AC3: Concurrent transition OFFERED -> ORDER_CREATED
	var wg sync.WaitGroup
	winners := 0
	var mu sync.Mutex
	orderID := "ORDER-123"

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.TransitionAgreement(ctx, agrID, StatusOffered, StatusOrderCreated, nil, &orderID, nil)
			if err == nil {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if winners != 1 {
		t.Fatalf("expected exactly 1 winner in concurrent redeem, got %d", winners)
	}

	// Verify status is ORDER_CREATED
	saved, err := s.GetAgreement(ctx, agrID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != StatusOrderCreated {
		t.Errorf("status = %q, want ORDER_CREATED", saved.Status)
	}

	// Second transition ORDER_CREATED -> CAPTURED
	captureID := "CAP-456"
	if err := s.TransitionAgreement(ctx, agrID, StatusOrderCreated, StatusCaptured, nil, nil, &captureID); err != nil {
		t.Fatalf("failed transition to CAPTURED: %v", err)
	}
	saved, _ = s.GetAgreement(ctx, agrID)
	if saved.Status != StatusCaptured || saved.CaptureID == nil || *saved.CaptureID != captureID {
		t.Errorf("unexpected agreement state after capture: %+v", saved)
	}
}

func TestStoreExpiration(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_expire.db")

	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	pol, _ := s.GetActivePolicy(ctx)
	now := NowUnix()

	// Expired OFFERED agreement
	agrExpired := &AgreementRecord{
		ID:                 "agr_expired",
		NegotiationID:      "neg_1",
		SessionID:          "sess_1",
		SKU:                "CH-WINTER-PARKA-01",
		PriceCents:         9400,
		ShippingTier:       "CONSOLIDATED_GROUND",
		BuyerBudgetCents:   10500,
		StandardTotalCents: 13500,
		PolicyID:           pol.ID,
		ExpiresAt:          now - 10, // expired 10s ago
	}
	_ = s.CreateAgreement(ctx, agrExpired)

	// Valid OFFERED agreement
	agrValid := &AgreementRecord{
		ID:                 "agr_valid",
		NegotiationID:      "neg_2",
		SessionID:          "sess_2",
		SKU:                "CH-WINTER-PARKA-01",
		PriceCents:         9400,
		ShippingTier:       "CONSOLIDATED_GROUND",
		BuyerBudgetCents:   10500,
		StandardTotalCents: 13500,
		PolicyID:           pol.ID,
		ExpiresAt:          now + 300,
	}
	_ = s.CreateAgreement(ctx, agrValid)

	// Run expiration
	expiredCount, err := s.ExpireStale(ctx, now)
	if err != nil {
		t.Fatalf("ExpireStale failed: %v", err)
	}
	if expiredCount != 1 {
		t.Errorf("expected 1 expired, got %d", expiredCount)
	}

	a1, _ := s.GetAgreement(ctx, "agr_expired")
	if a1.Status != StatusExpired {
		t.Errorf("expected agr_expired status to be EXPIRED, got %s", a1.Status)
	}
	a2, _ := s.GetAgreement(ctx, "agr_valid")
	if a2.Status != StatusOffered {
		t.Errorf("expected agr_valid status to be OFFERED, got %s", a2.Status)
	}
}

func TestStoreSingleActivePolicy(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_policy.db")

	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Initial active policy is seed
	p1, err := s.GetActivePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create draft
	cfg := PolicyConfig{
		MinMarginPct:      0.25,
		MaxDiscountPct:    0.20,
		BuyerPower:        0.30,
		AllowSlowShipping: true,
		PayLaterEnabled:   true,
		Rationale:         "Second policy",
	}
	draft, err := s.CreatePolicyDraft(ctx, "test prompt", "llm", cfg, []string{"adjusted"}, 0.15)
	if err != nil {
		t.Fatalf("create draft failed: %v", err)
	}

	// Activate draft
	active, err := s.ActivatePolicy(ctx, draft.ID)
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	if active.ID != draft.ID || active.Status != "ACTIVE" {
		t.Errorf("unexpected active policy: %+v", active)
	}

	// Ensure old policy is ARCHIVED
	old, _ := s.db.QueryContext(ctx, "SELECT status FROM merchant_policies WHERE id = ?", p1.ID)
	var oldStatus string
	old.Next()
	_ = old.Scan(&oldStatus)
	old.Close()
	if oldStatus != "ARCHIVED" {
		t.Errorf("expected old policy to be ARCHIVED, got %s", oldStatus)
	}

	// Ensure exactly 1 active policy exists
	var activeCount int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM merchant_policies WHERE status = 'ACTIVE'").Scan(&activeCount)
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active policy, got %d", activeCount)
	}
}
