package paypal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nashpay/nashpay/internal/store"
)

func TestBuildOrderRequestG1Bundle(t *testing.T) {
	bundleSKU := "SKU-SCARF-09"
	agr := &store.AgreementRecord{
		ID:         "agr_g1_test",
		PriceCents: 9400,
		BundleSKU:  &bundleSKU,
	}

	parka := &store.Product{
		SKU:  "CH-WINTER-PARKA-01",
		Name: "Winter Parka Performance 01",
	}

	scarf := &store.Product{
		SKU:              "SKU-SCARF-09",
		Name:             "Cashmere Scarf",
		RetailValueCents: 2500,
	}

	req, err := BuildOrderRequest(agr, parka, scarf, "http://localhost:8080")
	if err != nil {
		t.Fatalf("BuildOrderRequest failed: %v", err)
	}

	pu := req.PurchaseUnits[0]
	if pu.Amount.Value != "94.00" {
		t.Errorf("amount value = %s, want 94.00", pu.Amount.Value)
	}
	if pu.Amount.Breakdown.ItemTotal.Value != "119.00" {
		t.Errorf("item_total = %s, want 119.00", pu.Amount.Breakdown.ItemTotal.Value)
	}
	if pu.Amount.Breakdown.Discount == nil || pu.Amount.Breakdown.Discount.Value != "25.00" {
		t.Errorf("discount = %+v, want 25.00", pu.Amount.Breakdown.Discount)
	}
	if len(pu.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(pu.Items))
	}

	// Verify absence of forbidden payment_method_preference
	jsonBytes, _ := json.Marshal(req)
	if strings.Contains(string(jsonBytes), "payment_method_preference") {
		t.Error("OrderRequest must NOT contain payment_method_preference (PayPal API rejection risk)")
	}
}

func TestBuildOrderRequestG5Single(t *testing.T) {
	agr := &store.AgreementRecord{
		ID:         "agr_g5_test",
		PriceCents: 8600,
		BundleSKU:  nil,
	}

	parka := &store.Product{
		SKU:  "CH-WINTER-PARKA-01",
		Name: "Winter Parka Performance 01",
	}

	req, err := BuildOrderRequest(agr, parka, nil, "http://localhost:8080")
	if err != nil {
		t.Fatalf("BuildOrderRequest failed: %v", err)
	}

	pu := req.PurchaseUnits[0]
	if pu.Amount.Value != "86.00" {
		t.Errorf("amount value = %s, want 86.00", pu.Amount.Value)
	}
	if pu.Amount.Breakdown.ItemTotal.Value != "86.00" {
		t.Errorf("item_total = %s, want 86.00", pu.Amount.Breakdown.ItemTotal.Value)
	}
	if pu.Amount.Breakdown.Discount != nil {
		t.Errorf("expected nil discount for single item, got %+v", pu.Amount.Breakdown.Discount)
	}
	if len(pu.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(pu.Items))
	}
}

func TestMockClientIdempotency(t *testing.T) {
	ctx := context.Background()
	mock := NewMockClient()

	req := OrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []PurchaseUnitDTO{
			{
				CustomID: "agr_mock_1",
				Amount: AmountDTO{
					CurrencyCode: "USD",
					Value:        "94.00",
				},
			},
		},
	}

	orderID, err := mock.CreateOrder(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	details, err := mock.GetOrder(ctx, orderID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Status != "APPROVED" || details.PurchaseUnits[0].CustomID != "agr_mock_1" {
		t.Errorf("unexpected mock GetOrder details: %+v", details)
	}

	// First capture
	cap1, err := mock.CaptureOrder(ctx, orderID, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if cap1.Status != "COMPLETED" {
		t.Errorf("status = %s, want COMPLETED", cap1.Status)
	}

	// Second capture (idempotent)
	cap2, err := mock.CaptureOrder(ctx, orderID, "req-2")
	if err != nil {
		t.Fatal(err)
	}
	if cap2.ID != cap1.ID {
		t.Errorf("expected idempotent capture ID %s, got %s", cap1.ID, cap2.ID)
	}
}
