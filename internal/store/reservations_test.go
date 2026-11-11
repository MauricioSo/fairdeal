package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLastUnitReservationIsAtomicAndFailureRestoresOnce(t *testing.T) {
	ctx := context.Background()
	st, e := Open(ctx, filepath.Join(t.TempDir(), "stock.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	_, e = st.db.Exec("UPDATE products SET stock=1 WHERE sku='CH-WINTER-PARKA-01'")
	if e != nil {
		t.Fatal(e)
	}
	policy, e := st.GetActivePolicy(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"a", "b"} {
		if e = st.CreateAgreement(ctx, &AgreementRecord{ID: id, SessionID: id, SKU: "CH-WINTER-PARKA-01", PolicyID: policy.ID, PriceCents: 10000, ExpiresAt: NowUnix() + 300}); e != nil {
			t.Fatal(e)
		}
	}
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if st.TransitionAgreement(ctx, id, StatusOffered, StatusOrderCreated, nil, nil, nil) == nil {
				results <- id
			}
		}(id)
	}
	wg.Wait()
	close(results)
	count := 0
	winner := ""
	for id := range results {
		winner = id
		count++
	}
	if count != 1 {
		t.Fatalf("reserved last unit %d times", count)
	}
	_ = st.MarkPaymentUncertain(ctx, winner)
	if e = st.ExpireReservations(ctx, NowUnix()+7200); e != nil {
		t.Fatal(e)
	}
	_, _ = st.ExpireStale(ctx, NowUnix()+7200)
	a, e := st.GetAgreement(ctx, winner)
	if e != nil || a.Status != StatusOrderCreated {
		t.Fatal("uncertain payment reservation was expired")
	}
	if e = st.TransitionAgreement(ctx, winner, StatusOrderCreated, StatusFailed, nil, nil, nil); e != nil {
		t.Fatal(e)
	}
	_ = st.TransitionAgreement(ctx, winner, StatusOrderCreated, StatusFailed, nil, nil, nil)
	product, e := st.GetProduct(ctx, "CH-WINTER-PARKA-01")
	if e != nil || product.Stock != 1 {
		t.Fatalf("stock restored incorrectly: %+v", product)
	}
	data, _ := json.Marshal(AgreementRecord{BuyerBudgetCents: 17321, BuyerSurplusCents: 55})
	if strings.Contains(string(data), "17321") || strings.Contains(string(data), "buyer_surplus") {
		t.Fatal("private fields serializable")
	}
}

func TestAbandonedCheckoutReleasesStockAndCannotCapture(t *testing.T) {
	ctx := context.Background()
	st, e := Open(ctx, filepath.Join(t.TempDir(), "abandoned.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	policy, _ := st.GetActivePolicy(ctx)
	a := &AgreementRecord{ID: "abandoned", SKU: "CH-WINTER-PARKA-01", SessionID: "test", PolicyID: policy.ID, PriceCents: 10000, ExpiresAt: NowUnix() + 300}
	if e = st.CreateAgreement(ctx, a); e != nil {
		t.Fatal(e)
	}
	if e = st.TransitionAgreement(ctx, a.ID, StatusOffered, StatusOrderCreated, nil, nil, nil); e != nil {
		t.Fatal(e)
	}
	if e = st.ExpireReservations(ctx, NowUnix()+301); e != nil {
		t.Fatal(e)
	}
	product, e := st.GetProduct(ctx, a.SKU)
	if e != nil || product.Stock != 40 {
		t.Fatalf("abandoned stock not restored: %+v", product)
	}
	if st.BeginCapture(ctx, a.ID) == nil {
		t.Fatal("expired checkout allowed a capture")
	}
}
