package negotiation

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/store"
)

func TestServiceCommitAndRedeem(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_service.db")

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	secret := []byte("12345678901234567890123456789012")
	svc := NewService(st, secret, 300)

	// G1 constraints
	buyer := clearing.BuyerConstraints{
		Budget:              10500,
		ShippingFlexibility: 0.85,
		PrefersPayLater:     true,
		WillingToBundle:     true,
	}

	// 0. Preview (Feasible + NO_ZOPA)
	prevOk, err := svc.Preview(ctx, "CH-WINTER-PARKA-01", buyer, false)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if !prevOk.Feasible || prevOk.EstimatedPrice != "97.00" {
		t.Errorf("unexpected preview result: %+v", prevOk)
	}

	lowBuyer := buyer
	lowBuyer.Budget = 7500
	prevLow, err := svc.Preview(ctx, "CH-WINTER-PARKA-01", lowBuyer, false)
	if err != nil {
		t.Fatalf("preview low failed: %v", err)
	}
	if prevLow.Feasible || prevLow.Reason != "NO_ZOPA" {
		t.Errorf("expected NO_ZOPA preview, got %+v", prevLow)
	}

	// 1. First Commit (to be superseded by next commit)
	firstRes, err := svc.Commit(ctx, "sess_abc", "neg_1", "CH-WINTER-PARKA-01", buyer, false)
	if err != nil {
		t.Fatalf("first commit failed: %v", err)
	}

	// 2. Second Commit supersedes the first OFFERED agreement
	res, err := svc.Commit(ctx, "sess_abc", "neg_1", "CH-WINTER-PARKA-01", buyer, false)
	if err != nil {
		t.Fatalf("second commit failed: %v", err)
	}
	firstRec, err := st.GetAgreement(ctx, firstRes.AgreementID)
	if err != nil {
		t.Fatal(err)
	}
	if firstRec.Status != store.StatusExpired || firstRec.StatusReason == nil || *firstRec.StatusReason != "SUPERSEDED" {
		t.Errorf("expected first agreement SUPERSEDED, got %s (%v)", firstRec.Status, firstRec.StatusReason)
	}

	resp := res.NashAgreementResponseDTO
	if resp.Agreement.FinalPrice != "97.00" {
		t.Errorf("expected final price 97.00, got %s", resp.Agreement.FinalPrice)
	}
	if resp.Agreement.Concessions.PaymentMethod != "PAY_LATER" {
		t.Errorf("expected PAY_LATER, got %s", resp.Agreement.Concessions.PaymentMethod)
	}
	if resp.Agreement.Concessions.Installments == nil || resp.Agreement.Concessions.Installments.Amount != "24.25" {
		t.Errorf("expected 4 installments of 24.25, got %+v", resp.Agreement.Concessions.Installments)
	}
	if resp.ClearingToken == "" {
		t.Fatal("expected clearing token, got empty")
	}

	// 3. AC4: Confidentiality check on DTO and Normalized Trace
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	jsonStr := string(jsonBytes)
	forbiddenKeys := []string{"cost", "fee", "contribution", "surplus", "floor", "margin", "buyer_power"}
	for _, fk := range forbiddenKeys {
		if strings.Contains(jsonStr, "\""+fk) {
			t.Errorf("confidentiality breach: public DTO contains %q: %s", fk, jsonStr)
		}
	}

	if res.Trace == nil || len(res.Trace.Frontiers) == 0 {
		t.Fatal("expected normalized trace on CommitResult")
	}
	for _, f := range res.Trace.Frontiers {
		for _, pt := range f.Points {
			if pt[0] < 0 || pt[0] > 1.0 || pt[1] < 0 || pt[1] > 1.0 {
				t.Errorf("unnormalized frontier point leaked: %+v", pt)
			}
		}
	}

	// 4. PublicView check
	pub, err := svc.PublicView(ctx, resp.AgreementID)
	if err != nil {
		t.Fatalf("PublicView failed: %v", err)
	}
	if pub.FinalPrice != "97.00" || pub.Status != store.StatusOffered {
		t.Errorf("unexpected PublicView: %+v", pub)
	}

	// 5. Redeem with invalid token -> fails
	_, err = svc.Redeem(ctx, resp.AgreementID, "invalid_token_1234")
	if !errors.Is(err, clearing.ErrBadToken) {
		t.Fatalf("expected ErrBadToken, got %v", err)
	}

	// 6. Redeem with valid token -> success (transitions to ORDER_CREATED)
	rec, err := svc.Redeem(ctx, resp.AgreementID, resp.ClearingToken)
	if err != nil {
		t.Fatalf("redeem failed: %v", err)
	}
	if rec.Status != store.StatusOrderCreated {
		t.Errorf("expected status ORDER_CREATED, got %s", rec.Status)
	}

	// 7. Commit while ORDER_CREATED is live -> fails with ErrLiveAgreement
	_, err = svc.Commit(ctx, "sess_abc", "neg_1", "CH-WINTER-PARKA-01", buyer, false)
	if !errors.Is(err, ErrLiveAgreement) {
		t.Fatalf("expected ErrLiveAgreement while ORDER_CREATED is active, got %v", err)
	}

	// 8. Re-redeem -> should fail with ErrAlreadyUsed
	_, err = svc.Redeem(ctx, resp.AgreementID, resp.ClearingToken)
	if !errors.Is(err, store.ErrAlreadyUsed) {
		t.Fatalf("expected ErrAlreadyUsed on second redeem, got %v", err)
	}

	// 9. ExpireAgreement ownership check on a new session
	res2, err := svc.Commit(ctx, "sess_xyz", "neg_2", "CH-WINTER-PARKA-01", buyer, true)
	if err != nil {
		t.Fatalf("forbidPayLater commit failed: %v", err)
	}
	if res2.Agreement.Concessions.PaymentMethod != "PAYPAL" {
		t.Errorf("expected PAYPAL when forbidPayLater=true, got %s", res2.Agreement.Concessions.PaymentMethod)
	}
	if err := svc.ExpireAgreement(ctx, "wrong_sess", res2.AgreementID, "PAY_LATER_INELIGIBLE"); err == nil {
		t.Error("expected error when expiring agreement from wrong session")
	}
	if err := svc.ExpireAgreement(ctx, "sess_xyz", res2.AgreementID, "PAY_LATER_INELIGIBLE"); err != nil {
		t.Errorf("expected valid ExpireAgreement to succeed, got %v", err)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10)
	sess := "sess_rate_test"

	// Preview allows up to 60 per 10m window
	for i := 0; i < 60; i++ {
		if !rl.AllowPreview(sess) {
			t.Fatalf("expected preview %d to be allowed", i)
		}
	}
	if rl.AllowPreview(sess) {
		t.Error("expected 61st preview in window to be rate-limited")
	}

	// AllowCommit does not consume until MarkCommit is called
	if !rl.AllowCommit(sess) {
		t.Error("expected AllowCommit to succeed before MarkCommit")
	}
	if !rl.AllowCommit(sess) {
		t.Error("expected second AllowCommit to succeed when MarkCommit was not called")
	}
	rl.MarkCommit(sess)
	if rl.AllowCommit(sess) {
		t.Error("expected AllowCommit to block after MarkCommit")
	}
}
