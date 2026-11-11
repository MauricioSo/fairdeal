package clearing

import (
	"github.com/nashpay/nashpay/internal/economics"
	"github.com/nashpay/nashpay/internal/money"
)

// OfferCandidate adapts trusted catalog/policy data. Budget and utility never
// enter the economic authority, including when called from merchant simulation.
func OfferCandidate(in Input, price money.Cents, shipping string, bundle bool) economics.Candidate {
	cfg := in.Policy.Economics
	ship := in.Market.ExpressShipCostCents
	if shipping == ShippingConsolidatedGround {
		ship = in.Market.GroundShipCostCents
		if cfg.GroundCostCents != nil {
			ship = *cfg.GroundCostCents
		}
	} else if cfg.ExpressCostCents != nil {
		ship = *cfg.ExpressCostCents
	}
	bps, fixed := in.Market.FeeBps, in.Market.FeeFixedCents
	if cfg.PaymentFeeBps != nil {
		bps = *cfg.PaymentFeeBps
	}
	if cfg.PaymentFixedCents != nil {
		fixed = *cfg.PaymentFixedCents
	}
	items := []economics.Item{{SKU: in.Product.SKU, Quantity: 1, UnitCost: in.Product.CostCents, ListPrice: in.Product.ListPriceCents, CostKnown: in.Product.CostCents > 0}}
	if bundle && in.Bundle != nil {
		items = append(items, economics.Item{SKU: in.Bundle.SKU, Quantity: 1, UnitCost: in.Bundle.CostCents, ListPrice: in.Bundle.ListPriceCents, CostKnown: in.Bundle.CostCents > 0})
	}
	keep := 10000 - pctToBps(in.Policy.MaxDiscountPct)
	return economics.Candidate{Currency: "USD", Gross: price, Items: items, Shipping: shipping, ShippingCost: ship, CostsKnown: shipping == ShippingExpress || shipping == ShippingConsolidatedGround, PaymentFeeBps: bps, PaymentFixed: fixed, MinGross: money.Cents((int64(in.Product.ListPriceCents)*keep + 9999) / 10000), MinMarginBps: pctToBps(in.Policy.MinMarginPct), Config: cfg}
}

func EvaluateOffer(in Input, price money.Cents, shipping string, bundle bool) economics.Evaluation {
	e := economics.Evaluate(OfferCandidate(in, price, shipping, bundle))
	if shipping == ShippingConsolidatedGround && !in.Policy.AllowSlowShipping {
		e.Reasons = append(e.Reasons, "SHIPPING_NOT_ALLOWED")
	}
	if bundle && in.Bundle == nil {
		e.Reasons = append(e.Reasons, "BUNDLE_NOT_ALLOWED")
	}
	e.Approved = len(e.Reasons) == 0
	return e
}
