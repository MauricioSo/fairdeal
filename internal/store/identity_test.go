package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityMigrationPreservesCapturesAndCreatesRecoverableBackup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "historic.db")
	st, e := Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	policy, _ := st.GetActivePolicy(ctx)
	a := &AgreementRecord{ID: "historic", SKU: "CH-WINTER-PARKA-01", SessionID: "old", PolicyID: policy.ID, PriceCents: 9500, BuyerBudgetCents: 10500, BuyerSurplusCents: 99, ExpiresAt: NowUnix() + 300}
	if e = st.CreateAgreement(ctx, a); e != nil {
		t.Fatal(e)
	}
	if e = st.TransitionAgreement(ctx, a.ID, StatusOffered, StatusOrderCreated, nil, nil, nil); e != nil {
		t.Fatal(e)
	}
	capture := "old-capture"
	if e = st.TransitionAgreement(ctx, a.ID, StatusOrderCreated, StatusCaptured, nil, nil, &capture); e != nil {
		t.Fatal(e)
	}
	if e = st.BindMerchant(ctx, "alpine"); e != nil {
		t.Fatal(e)
	}
	current, e := st.GetAgreement(ctx, a.ID)
	if e != nil || current.PriceCents != 9500 || current.CaptureID == nil || *current.CaptureID != capture || current.Status != StatusCaptured || current.BuyerBudgetCents != 0 || current.BuyerSurplusCents != 0 {
		t.Fatalf("migration corrupted historic capture: %+v %v", current, e)
	}
	backupPath := path + ".pre-marketplace.backup.db"
	if _, e = os.Stat(backupPath); e != nil {
		t.Fatal("backup missing", e)
	}
	backup, e := Open(ctx, backupPath)
	if e != nil {
		t.Fatal(e)
	}
	defer backup.Close()
	before, e := backup.GetAgreement(ctx, a.ID)
	if e != nil || before.BuyerBudgetCents != 10500 || before.PriceCents != 9500 {
		t.Fatal("backup did not preserve original state")
	}
	if st.BindMerchant(ctx, "nordic") == nil {
		t.Fatal("database was rebound to a different merchant")
	}
}
