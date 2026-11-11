package clearing

import (
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/nashpay/nashpay/internal/money"
)

func defaultSeedInput() Input {
	parka := Product{
		SKU:              "CH-WINTER-PARKA-01",
		ListPriceCents:   12000,
		CostCents:        5000,
		RetailValueCents: 12000,
		HoldingCostCents: 0,
	}
	scarf := Product{
		SKU:              "SKU-SCARF-09",
		ListPriceCents:   2500,
		CostCents:        900,
		RetailValueCents: 2500,
		HoldingCostCents: 500,
	}
	market := Market{
		ExpressShipCostCents:        1500,
		GroundShipCostCents:         400,
		StandardShippingChargeCents: 1500,
		WaitCostMaxCents:            1500,
		PayLaterBuyerValueCents:     300,
		BundleValuePct:              0.48,
		FeeBps:                      349,
		FeeFixedCents:               49,
		PayLaterMinCents:            3000,
		PayLaterMaxCents:            150000,
	}
	policy := Policy{
		MinMarginPct:      0.20,
		MaxDiscountPct:    0.30,
		BuyerPower:        0.35,
		AllowSlowShipping: true,
		PayLaterEnabled:   true,
	}
	return Input{
		Product: parka,
		Bundle:  &scarf,
		Market:  market,
		Policy:  policy,
	}
}

func TestGoldenCases(t *testing.T) {
	tests := []struct {
		name         string
		budget       money.Cents
		flex         float64
		prefBNPL     bool
		willBundle   bool
		wantErr      error
		wantShip     string
		wantBNPL     bool
		wantBundle   bool
		wantRawPrice money.Cents
		wantFinal    money.Cents
		wantFee      money.Cents
		wantContrib  money.Cents
	}{
		{
			name:         "G1 demo",
			budget:       10500,
			flex:         0.85,
			prefBNPL:     true,
			willBundle:   true,
			wantShip:     ShippingConsolidatedGround,
			wantBNPL:     true,
			wantBundle:   true,
			wantRawPrice: 9785,
			wantFinal:    9700,
			wantFee:      388,
			wantContrib:  3012,
		},
		{
			name:         "G2 flex0",
			budget:       10500,
			flex:         0.00,
			prefBNPL:     true,
			willBundle:   true,
			wantShip:     ShippingExpress,
			wantBNPL:     true,
			wantBundle:   true,
			wantRawPrice: 10329,
			wantFinal:    10300,
			wantFee:      408,
			wantContrib:  2492,
		},
		{
			name:         "G3 budget85",
			budget:       8500,
			flex:         0.85,
			prefBNPL:     true,
			willBundle:   true,
			wantShip:     ShippingConsolidatedGround,
			wantBNPL:     true,
			wantBundle:   true,
			wantRawPrice: 8467,
			wantFinal:    8400,
		},
		{
			name:       "G4 budget60",
			budget:     6000,
			flex:       0.85,
			prefBNPL:   true,
			willBundle: true,
			wantErr:    ErrNoZOPA,
		},
		{
			name:         "G5 nobundle nobnpl",
			budget:       10500,
			flex:         0.85,
			prefBNPL:     false,
			willBundle:   false,
			wantShip:     ShippingConsolidatedGround,
			wantBNPL:     false,
			wantBundle:   false,
			wantRawPrice: 8667,
			wantFinal:    8600,
			wantFee:      349,
			wantContrib:  2851,
		},
		{
			name:         "G6 budget200",
			budget:       20000,
			flex:         0.85,
			prefBNPL:     true,
			willBundle:   true,
			wantShip:     ShippingConsolidatedGround,
			wantBNPL:     true,
			wantBundle:   true,
			wantRawPrice: 11733,
			wantFinal:    11700,
			wantFee:      457,
			wantContrib:  4943,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := defaultSeedInput()
			in.Buyer = BuyerConstraints{
				Budget:              tc.budget,
				ShippingFlexibility: tc.flex,
				PrefersPayLater:     tc.prefBNPL,
				WillingToBundle:     tc.willBundle,
			}

			res, err := Solve(in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if res.Shipping != tc.wantShip {
				t.Errorf("shipping = %s, want %s", res.Shipping, tc.wantShip)
			}
			if res.PayLater != tc.wantBNPL {
				t.Errorf("payLater = %t, want %t", res.PayLater, tc.wantBNPL)
			}
			if res.Bundle != tc.wantBundle {
				t.Errorf("bundle = %t, want %t", res.Bundle, tc.wantBundle)
			}
			if tc.wantRawPrice > 0 && res.RawPriceCents != tc.wantRawPrice {
				t.Errorf("rawPrice = %d, want %d", res.RawPriceCents, tc.wantRawPrice)
			}
			if res.PriceCents != tc.wantFinal {
				t.Errorf("finalPrice = %d, want %d", res.PriceCents, tc.wantFinal)
			}
			if tc.wantFee > 0 && res.FeeCents != tc.wantFee {
				t.Errorf("fee = %d, want %d", res.FeeCents, tc.wantFee)
			}
			if tc.wantContrib > 0 && res.ContributionCents != tc.wantContrib {
				t.Errorf("contrib = %d, want %d", res.ContributionCents, tc.wantContrib)
			}
		})
	}
}

func TestProperties(t *testing.T) {
	in := defaultSeedInput()
	in.Buyer = BuyerConstraints{
		Budget:              10500,
		ShippingFlexibility: 0.85,
		PrefersPayLater:     true,
		WillingToBundle:     true,
	}

	// Determinism test: 100 runs give exact same result
	var first *Result
	for i := 0; i < 100; i++ {
		r, err := Solve(in)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = r
		} else {
			if r.PriceCents != first.PriceCents || r.Shipping != first.Shipping || r.PayLater != first.PayLater || r.Bundle != first.Bundle {
				t.Fatalf("non-deterministic result on iteration %d", i)
			}
		}
	}

	// ForbidPayLater property
	inForbid := in
	inForbid.ForbidPayLater = true
	rForbid, err := Solve(inForbid)
	if err != nil {
		t.Fatal(err)
	}
	if rForbid.PayLater {
		t.Errorf("expected PayLater=false when ForbidPayLater=true, got true")
	}

	// Surpluses positive
	if first.BuyerSurplusCents <= 0 {
		t.Errorf("buyer surplus not positive: %d", first.BuyerSurplusCents)
	}
	if first.SellerSurplusCents <= 0 {
		t.Errorf("seller surplus not positive: %d", first.SellerSurplusCents)
	}
}

// TestInvariantsAcrossBudgets sweeps budgets/flex and checks properties (a), (b), (c), (f)
// from the backlog: price bounds, margin floor, positive surpluses and Pareto-optimality
// of the chosen price within its own option frontier.
func TestInvariantsAcrossBudgets(t *testing.T) {
	for budget := money.Cents(7000); budget <= 14000; budget += 250 {
		for _, flex := range []float64{0, 0.5, 1} {
			in := defaultSeedInput()
			in.Buyer = BuyerConstraints{Budget: budget, ShippingFlexibility: flex, PrefersPayLater: true, WillingToBundle: true}
			res, err := Solve(in)
			if errors.Is(err, ErrNoZOPA) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			lower := money.Cents(8400)
			upper := in.Product.ListPriceCents
			if budget < upper {
				upper = budget
			}
			if res.PriceCents < lower || res.PriceCents > upper {
				t.Errorf("(a) budget=%d flex=%.1f price=%d outside [%d,%d]", budget, flex, res.PriceCents, lower, upper)
			}
			if !meetsMargin(res.ContributionCents, res.PriceCents, 2000) {
				t.Errorf("(b) budget=%d flex=%.1f margin floor violated: contrib=%d price=%d", budget, flex, res.ContributionCents, res.PriceCents)
			}
			if res.BuyerSurplusCents <= 0 || res.SellerSurplusCents <= 0 {
				t.Errorf("(c) budget=%d flex=%.1f non-positive surplus b=%d s=%d", budget, flex, res.BuyerSurplusCents, res.SellerSurplusCents)
			}
			// (f) no feasible point of the selected frontier strictly dominates the raw optimum
			for _, f := range res.Trace {
				if !f.Selected {
					continue
				}
				for _, pt := range f.Points {
					rawUb := float64(res.BuyerSurplusCents) / 100.0
					rawUs := float64(res.SellerSurplusCents) / 100.0
					if pt[0] > rawUb+1.0 && pt[1] > rawUs+1.0 {
						t.Errorf("(f) point %v dominates chosen (%.2f,%.2f) by more than rounding slack", pt, rawUb, rawUs)
					}
				}
			}
		}
	}
}

func TestPurePackageImports(t *testing.T) {
	// AC5: verify clearing package does not import database/sql, net/http, or os
	files := []string{"market.go", "solver.go"}
	fset := token.NewFileSet()
	forbidden := map[string]bool{
		"database/sql": true,
		"net/http":     true,
		"os":           true,
	}

	for _, file := range files {
		node, err := parser.ParseFile(fset, filepath.Join(".", file), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", file, err)
		}
		for _, imp := range node.Imports {
			path := imp.Path.Value
			// strip quotes
			clean := path[1 : len(path)-1]
			if forbidden[clean] {
				t.Errorf("%s imports forbidden package %q", file, clean)
			}
		}
	}
}

func BenchmarkSolve(b *testing.B) {
	in := defaultSeedInput()
	in.Buyer = BuyerConstraints{
		Budget:              10500,
		ShippingFlexibility: 0.85,
		PrefersPayLater:     true,
		WillingToBundle:     true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Solve(in)
	}
}
