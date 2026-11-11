// Package economics is the sole authority for offer profitability. It has no
// database, network, buyer budget or LLM dependencies. All amounts are USD cents.
package economics

import (
	"fmt"
	"github.com/nashpay/nashpay/internal/money"
)

const MaxAmount money.Cents = 100000000

// Config is merchant-authorized, versioned with the commercial policy. Shipping
// and payment overrides are conservative upper bounds, not live carrier quotes.
type Config struct {
	MinContributionCents money.Cents   `json:"min_contribution_cents"`
	HandlingCents        money.Cents   `json:"handling_cents"`
	PackagingCents       money.Cents   `json:"packaging_cents"`
	PlatformFeeBps       int64         `json:"platform_fee_bps"`
	PlatformFixedCents   money.Cents   `json:"platform_fixed_cents"`
	IncludedTaxBps       int64         `json:"included_tax_bps"`
	ExpressCostCents     *money.Cents  `json:"express_cost_cents,omitempty"`
	GroundCostCents      *money.Cents  `json:"ground_cost_cents,omitempty"`
	PaymentFeeBps        *int64        `json:"payment_fee_bps,omitempty"`
	PaymentFixedCents    *money.Cents  `json:"payment_fixed_cents,omitempty"`
	ProductRules         []ProductRule `json:"product_rules,omitempty"`
}

// A protected product cannot be discounted or included as a free accessory.
// A per-item margin is evaluated on a proportional allocation of net revenue
// and shared costs; its cost is never hidden by a more profitable item.
type ProductRule struct {
	SKU          string `json:"sku"`
	Protected    bool   `json:"protected"`
	MinMarginBps int64  `json:"min_margin_bps"`
}

func (c Config) Validate() error {
	for _, v := range []money.Cents{c.MinContributionCents, c.HandlingCents, c.PackagingCents, c.PlatformFixedCents} {
		if v < 0 || v > MaxAmount {
			return fmt.Errorf("cost or contribution outside allowed range")
		}
	}
	for _, v := range []*money.Cents{c.ExpressCostCents, c.GroundCostCents, c.PaymentFixedCents} {
		if v != nil && (*v < 0 || *v > MaxAmount) {
			return fmt.Errorf("invalid cost bound")
		}
	}
	if c.PlatformFeeBps < 0 || c.PlatformFeeBps > 10000 || c.IncludedTaxBps < 0 || c.IncludedTaxBps > 10000 || (c.PaymentFeeBps != nil && (*c.PaymentFeeBps < 0 || *c.PaymentFeeBps > 10000)) {
		return fmt.Errorf("invalid rate in basis points")
	}
	if len(c.ProductRules) > 30 {
		return fmt.Errorf("at most 30 product rules")
	}
	seen := map[string]bool{}
	for _, r := range c.ProductRules {
		if r.SKU == "" || len(r.SKU) > 80 || seen[r.SKU] || r.MinMarginBps < 0 || r.MinMarginBps > 10000 {
			return fmt.Errorf("invalid or duplicate product rule")
		}
		seen[r.SKU] = true
	}
	return nil
}

type Item struct {
	SKU       string      `json:"sku"`
	Quantity  int64       `json:"quantity"`
	UnitCost  money.Cents `json:"unit_cost_cents"`
	ListPrice money.Cents `json:"list_price_cents"`
	CostKnown bool        `json:"cost_known"`
}
type Candidate struct {
	Currency      string      `json:"currency"`
	Gross         money.Cents `json:"gross_cents"` // complete amount paid, including shipping and included tax
	Items         []Item      `json:"items"`
	Shipping      string      `json:"shipping"`
	ShippingCost  money.Cents `json:"shipping_cost_cents"`
	CostsKnown    bool        `json:"costs_known"`
	PaymentFeeBps int64       `json:"payment_fee_bps"`
	PaymentFixed  money.Cents `json:"payment_fixed_cents"`
	MinGross      money.Cents `json:"min_gross_cents"`
	MinMarginBps  int64       `json:"min_margin_bps"`
	Config        Config      `json:"config"`
}
type Breakdown struct {
	Gross        money.Cents `json:"gross_cents"`
	IncludedTax  money.Cents `json:"included_tax_cents"`
	NetRevenue   money.Cents `json:"net_revenue_cents"`
	Goods        money.Cents `json:"goods_cents"`
	Shipping     money.Cents `json:"shipping_cents"`
	Handling     money.Cents `json:"handling_cents"`
	Packaging    money.Cents `json:"packaging_cents"`
	PaymentFee   money.Cents `json:"payment_fee_cents"`
	PlatformFee  money.Cents `json:"platform_fee_cents"`
	TotalCost    money.Cents `json:"total_cost_cents"`
	Contribution money.Cents `json:"contribution_cents"`
	MarginBps    int64       `json:"margin_bps"` // display only; checks use cross multiplication
}
type Evaluation struct {
	Approved  bool      `json:"approved"`
	Reasons   []string  `json:"reasons"`
	Breakdown Breakdown `json:"breakdown"`
}

func Fee(gross money.Cents, bps int64, fixed money.Cents) money.Cents {
	return money.Cents((int64(gross)*bps+5000)/10000) + fixed
}
func Evaluate(c Candidate) Evaluation {
	e := Evaluation{Reasons: []string{}}
	reject := func(reason string) { e.Reasons = append(e.Reasons, reason) }
	if c.Config.Validate() != nil || c.Currency != "USD" || c.Gross <= 0 || c.Gross > MaxAmount || c.MinGross < 0 || c.MinGross > MaxAmount || c.MinMarginBps < 0 || c.MinMarginBps > 10000 || c.PaymentFeeBps < 0 || c.PaymentFeeBps > 10000 || c.PaymentFixed < 0 || c.PaymentFixed > MaxAmount || c.ShippingCost < 0 || c.ShippingCost > MaxAmount || len(c.Items) == 0 || len(c.Items) > 20 {
		reject("INVALID_ECONOMIC_INPUT")
		return e
	}
	if !c.CostsKnown {
		reject("MISSING_COST")
		return e
	}
	var list money.Cents
	seen := map[string]bool{}
	for _, item := range c.Items {
		if !item.CostKnown {
			reject("MISSING_COST")
			return e
		}
		if item.SKU == "" || seen[item.SKU] || item.Quantity < 1 || item.Quantity > 100 || item.UnitCost < 0 || item.UnitCost > MaxAmount || item.ListPrice <= 0 || item.ListPrice > MaxAmount {
			reject("INVALID_ITEM")
			return e
		}
		seen[item.SKU] = true
		e.Breakdown.Goods += item.UnitCost * money.Cents(item.Quantity)
		list += item.ListPrice * money.Cents(item.Quantity)
	}
	if list > MaxAmount || e.Breakdown.Goods > MaxAmount {
		reject("INVALID_ITEM_TOTAL")
		return e
	}
	b := &e.Breakdown
	b.Gross = c.Gross
	// Included tax rounds upward, net revenue downward: conservative for margin.
	b.NetRevenue = money.Cents(int64(c.Gross) * 10000 / (10000 + c.Config.IncludedTaxBps))
	b.IncludedTax = c.Gross - b.NetRevenue
	b.Shipping = c.ShippingCost
	b.Handling = c.Config.HandlingCents
	b.Packaging = c.Config.PackagingCents
	b.PaymentFee = Fee(c.Gross, c.PaymentFeeBps, c.PaymentFixed)
	// Platform percentage charged on net revenue; payment fee on the full charge.
	b.PlatformFee = Fee(b.NetRevenue, c.Config.PlatformFeeBps, c.Config.PlatformFixedCents)
	b.TotalCost = b.Goods + b.Shipping + b.Handling + b.Packaging + b.PaymentFee + b.PlatformFee
	b.Contribution = b.NetRevenue - b.TotalCost
	if b.NetRevenue <= 0 {
		reject("NO_NET_REVENUE")
		return e
	}
	b.MarginBps = int64(b.Contribution) * 10000 / int64(b.NetRevenue)
	if c.Gross < c.MinGross {
		reject("DISCOUNT_LIMIT")
	}
	if int64(b.Contribution)*10000 < c.MinMarginBps*int64(b.NetRevenue) {
		reject("MARGIN_BELOW_MINIMUM")
	}
	if b.Contribution < c.Config.MinContributionCents {
		reject("CONTRIBUTION_BELOW_MINIMUM")
	}
	// Allocate by public list value. Round each item's shared costs upward and
	// revenue downward so rounding cannot manufacture item profitability.
	for _, item := range c.Items {
		for _, r := range c.Config.ProductRules {
			if r.SKU == item.SKU {
				weight := item.ListPrice * money.Cents(item.Quantity)
				if r.Protected && c.Gross < list {
					reject("PROTECTED_PRODUCT")
				}
				if r.MinMarginBps > 0 {
					revenue := money.Cents(int64(b.NetRevenue) * int64(weight) / int64(list))
					shared := b.TotalCost - b.Goods
					allocated := money.Cents((int64(shared)*int64(weight) + int64(list) - 1) / int64(list))
					contribution := revenue - item.UnitCost*money.Cents(item.Quantity) - allocated
					if revenue <= 0 || int64(contribution)*10000 < r.MinMarginBps*int64(revenue) {
						reject("PRODUCT_MARGIN_BELOW_MINIMUM")
					}
				}
			}
		}
	}
	e.Approved = len(e.Reasons) == 0
	return e
}
