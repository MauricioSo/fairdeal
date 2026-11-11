package economics

import (
	"github.com/nashpay/nashpay/internal/money"
	"testing"
)

func example() Candidate {
	return Candidate{Currency: "USD", Gross: 10000, Items: []Item{{SKU: "MAIN", Quantity: 1, UnitCost: 5000, ListPrice: 10000, CostKnown: true}}, Shipping: "EXPRESS", ShippingCost: 1500, CostsKnown: true, PaymentFeeBps: 300, PaymentFixed: 50, MinGross: 7000, MinMarginBps: 2500}
}
func has(e Evaluation, reason string) bool {
	for _, r := range e.Reasons {
		if r == reason {
			return true
		}
	}
	return false
}
func TestCostsAndAllLimits(t *testing.T) {
	c := example()
	e := Evaluate(c)
	if !e.Approved || e.Breakdown.Contribution != 3150 {
		t.Fatalf("unexpected base %+v", e)
	}
	c.Config = Config{HandlingCents: 200, PackagingCents: 100, PlatformFeeBps: 100, PlatformFixedCents: 50, MinContributionCents: 2800}
	e = Evaluate(c)
	if !has(e, "CONTRIBUTION_BELOW_MINIMUM") || e.Breakdown.Contribution != 2700 {
		t.Fatalf("missing costs %+v", e)
	}
	c.Config.MinContributionCents = 0
	c.Config.IncludedTaxBps = 1900
	e = Evaluate(c)
	if !has(e, "MARGIN_BELOW_MINIMUM") || e.Breakdown.NetRevenue != 8403 {
		t.Fatalf("tax treated as revenue %+v", e)
	}
}
func TestAccessoryCanDestroyMarginButHoldingReliefCannotRescueIt(t *testing.T) {
	c := example()
	c.Items = append(c.Items, Item{SKU: "OLD", Quantity: 1, UnitCost: 2000, ListPrice: 2500, CostKnown: true})
	if e := Evaluate(c); !has(e, "MARGIN_BELOW_MINIMUM") {
		t.Fatalf("deficit bundle allowed %+v", e)
	}
	c.Config.ProductRules = []ProductRule{{SKU: "OLD", Protected: true}}
	if e := Evaluate(c); !has(e, "PROTECTED_PRODUCT") {
		t.Fatalf("protected gift allowed %+v", e)
	}
}
func TestExactBoundaryAndMissingCosts(t *testing.T) {
	c := example()
	c.PaymentFeeBps = 0
	c.PaymentFixed = 0
	c.ShippingCost = 0
	c.Items[0].UnitCost = 7500
	if !Evaluate(c).Approved {
		t.Fatal("exact 25% rejected")
	}
	c.Items[0].UnitCost++
	if !has(Evaluate(c), "MARGIN_BELOW_MINIMUM") {
		t.Fatal("one-cent violation allowed")
	}
	c.Items[0].CostKnown = false
	if !has(Evaluate(c), "MISSING_COST") {
		t.Fatal("unknown cost assumed zero")
	}
	c.Items[0].CostKnown = true
	c.CostsKnown = false
	if !has(Evaluate(c), "MISSING_COST") {
		t.Fatal("unknown shipping allowed")
	}
}
func TestProductMarginCannotHideBehindOtherItem(t *testing.T) {
	c := example()
	c.PaymentFeeBps = 0
	c.PaymentFixed = 0
	c.ShippingCost = 0
	c.MinMarginBps = 1000
	c.Gross = 20000
	c.Items = []Item{{"MAIN", 1, 1000, 10000, true}, {"EXPENSIVE", 1, 9500, 10000, true}}
	c.Config.ProductRules = []ProductRule{{SKU: "EXPENSIVE", MinMarginBps: 2000}}
	if e := Evaluate(c); !has(e, "PRODUCT_MARGIN_BELOW_MINIMUM") {
		t.Fatalf("item floor bypassed %+v", e)
	}
}
func TestUntrustedNumericBounds(t *testing.T) {
	for _, mutate := range []func(*Candidate){func(c *Candidate) { c.Gross = MaxAmount + 1 }, func(c *Candidate) { c.PaymentFeeBps = -1 }, func(c *Candidate) { c.Items[0].Quantity = 100000000 }, func(c *Candidate) { c.Config.PlatformFeeBps = 10001 }, func(c *Candidate) { c.Items[0].UnitCost = money.Cents(-1) }} {
		c := example()
		mutate(&c)
		if Evaluate(c).Approved {
			t.Fatal("invalid numeric input allowed")
		}
	}
}
