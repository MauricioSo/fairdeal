package negotiation

import (
	"context"
	"errors"
	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/store"
	"path/filepath"
	"testing"
)

func TestEconomicSnapshotAndChangedSettingsBlockCheckout(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "economics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := NewService(st, []byte("economic-test-signing-key"), 300)
	quote, err := svc.Commit(ctx, "buyer", "neg", "CH-WINTER-PARKA-01", clearing.BuyerConstraints{Budget: 12000, ShippingFlexibility: 1, WillingToBundle: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := st.GetEconomics(ctx, quote.AgreementID)
	if err != nil || !snap.Evaluation.Approved {
		t.Fatalf("missing snapshot %v", err)
	}
	if _, err = st.DB().ExecContext(ctx, "UPDATE settings SET value_json=json_set(value_json,'$.ground_ship_cost_cents',9000) WHERE key='market'"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Redeem(ctx, quote.AgreementID, quote.ClearingToken); !errors.Is(err, store.ErrEconomicChange) {
		t.Fatalf("stale costs accepted: %v", err)
	}
	p, _ := st.GetProduct(ctx, "CH-WINTER-PARKA-01")
	if p.Stock != 40 {
		t.Fatal("rejected checkout reserved inventory")
	}
}
func TestPolicyChangeExpiresOffersButPreservesFrozenCheckout(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "policy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := NewService(st, []byte("economic-test-signing-key"), 300)
	buyer := clearing.BuyerConstraints{Budget: 12000, ShippingFlexibility: 1}
	reserved, err := svc.Commit(ctx, "buyer1", "neg1", "CH-WINTER-PARKA-01", buyer, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Redeem(ctx, reserved.AgreementID, reserved.ClearingToken); err != nil {
		t.Fatal(err)
	}
	offered, err := svc.Commit(ctx, "buyer2", "neg2", "CH-WINTER-PARKA-01", buyer, false)
	if err != nil {
		t.Fatal(err)
	}
	active, _ := st.GetActivePolicy(ctx)
	cfg := active.Policy
	cfg.Economics.MinContributionCents = 5000
	draft, err := st.CreatePolicyDraft(ctx, "manual", "manual", cfg, []string{}, active.HardFloorMarginPct)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.ActivatePolicy(ctx, draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Redeem(ctx, offered.AgreementID, offered.ClearingToken); err == nil {
		t.Fatal("old offer redeemed")
	}
	reason := "TEST_CAPTURE"
	capture := "cap_frozen"
	if err = st.TransitionAgreement(ctx, reserved.AgreementID, store.StatusOrderCreated, store.StatusCaptured, &reason, nil, &capture); err != nil {
		t.Fatal(err)
	}
	snap, err := st.GetEconomics(ctx, reserved.AgreementID)
	if err != nil || snap.PolicyID != active.ID || !snap.Evaluation.Approved {
		t.Fatal("snapshot changed retroactively")
	}
}
