package clearing

import (
	"fmt"
	"math"

	"github.com/nashpay/nashpay/internal/money"
)

// CalculateFee returns PayPal transaction fee in integer cents using rule BR-12:
// fee = (p * fee_bps + 5000) / 10000 + fee_fixed_cents
func CalculateFee(p money.Cents, bps int64, fixed money.Cents) money.Cents {
	v := (int64(p)*bps + 5000) / 10000
	return money.Cents(v) + fixed
}

// pctToBps converts a fractional percentage (0.20) into basis points (2000)
// so every margin / discount comparison is done in exact integer arithmetic.
func pctToBps(pct float64) int64 { return int64(math.Round(pct * 10000)) }

// meetsMargin reports contrib/price >= marginBps/10000 using integers only.
func meetsMargin(contrib, price money.Cents, marginBps int64) bool {
	return int64(contrib)*10000 >= marginBps*int64(price)
}

type option struct {
	shipping string
	bnpl     bool
	bundle   bool
}

type candidate struct {
	opt      option
	rawPrice money.Cents
	fee      money.Cents
	contrib  money.Cents
	obj      float64
}

type optionPoints struct {
	opt    option
	points [][2]float64
}

type solver struct {
	in           Input
	dB           money.Cents
	lower        money.Cents
	upper        money.Cents
	marginBps    int64
	best         *candidate
	alternatives []*candidate
	frontiers    []optionPoints
	standardTo   money.Cents
}

// Solve executes the discrete combinatorial Nash Bargaining search across Pareto frontier candidates.
func Solve(in Input) (*Result, error) {
	s := &solver{in: in}
	s.standardTo = in.Product.ListPriceCents + in.Market.StandardShippingChargeCents
	if in.ReferenceValue == 0 && in.Buyer.Budget > s.standardTo {
		s.dB = in.Buyer.Budget - s.standardTo
	}
	s.marginBps = pctToBps(in.Policy.MinMarginPct)

	// lower = ceil(list * (1 - maxDiscount)) computed with integers.
	keepBps := 10000 - pctToBps(in.Policy.MaxDiscountPct)
	s.lower = money.Cents((int64(in.Product.ListPriceCents)*keepBps + 9999) / 10000)
	s.upper = in.Product.ListPriceCents
	if in.Buyer.Budget < s.upper {
		s.upper = in.Buyer.Budget
	}
	if s.lower > s.upper {
		return nil, ErrNoZOPA
	}

	for _, opt := range s.options() {
		s.searchOption(opt)
	}
	if s.best == nil {
		return nil, ErrNoZOPA
	}
	if in.ReferenceValue > 0 {
		s.reconsiderRounded()
	}
	return s.finalize(), nil
}

// options enumerates strategy combinations in the contractual order:
// shipping (express, ground) x pay later (false, true) x bundle (false, true).
func (s *solver) options() []option {
	ships := []string{ShippingExpress}
	if s.in.Policy.AllowSlowShipping && s.in.Buyer.ShippingFlexibility > 0 {
		ships = append(ships, ShippingConsolidatedGround)
	}
	bnpls := []bool{false}
	if s.in.Policy.PayLaterEnabled && s.in.Buyer.PrefersPayLater && !s.in.ForbidPayLater {
		bnpls = append(bnpls, true)
	}
	bundles := []bool{false}
	if s.in.Bundle != nil && s.in.Buyer.WillingToBundle {
		bundles = append(bundles, true)
	}
	var out []option
	for _, sh := range ships {
		for _, b := range bnpls {
			for _, bu := range bundles {
				out = append(out, option{sh, b, bu})
			}
		}
	}
	return out
}

func (s *solver) shipCost(ship string) money.Cents {
	if ship == ShippingConsolidatedGround {
		return s.in.Market.GroundShipCostCents
	}
	return s.in.Market.ExpressShipCostCents
}

// buyerValue is V_b(x): budget minus waiting cost plus perceived-value bonuses.
func (s *solver) buyerValue(o option) money.Cents {
	m := s.in.Market
	v := s.in.Buyer.Budget
	if s.in.ReferenceValue > 0 {
		v = s.in.ReferenceValue
	}
	if o.shipping == ShippingConsolidatedGround {
		wait := math.Round((1.0 - s.in.Buyer.ShippingFlexibility) * float64(m.WaitCostMaxCents))
		v -= money.Cents(wait)
	}
	if o.bnpl && s.in.Buyer.PrefersPayLater {
		v += m.PayLaterBuyerValueCents
	}
	if o.bundle && s.in.Bundle != nil {
		v += money.Cents(math.Round(m.BundleValuePct * float64(s.in.Bundle.RetailValueCents)))
	}
	return v
}

func (s *solver) bundleHolding(o option) money.Cents {
	if o.bundle && s.in.Bundle != nil {
		return s.in.Bundle.HoldingCostCents
	}
	return 0
}

func (s *solver) bundleCost(o option) money.Cents {
	if o.bundle && s.in.Bundle != nil {
		return s.in.Bundle.CostCents
	}
	return 0
}

func (s *solver) payLaterOK(o option, p money.Cents) bool {
	return !o.bnpl || (p >= s.in.Market.PayLaterMinCents && p <= s.in.Market.PayLaterMaxCents)
}

// searchOption scans every cent in [lower, upper] for one strategy option,
// recording its Pareto points and updating the best Nash-product candidate.
func (s *solver) searchOption(o option) {
	vB := s.buyerValue(o)
	shipCost := s.shipCost(o.shipping)
	alpha := s.in.Policy.BuyerPower
	var pts [][2]float64
	var optionBest *candidate

	for p := s.lower; p <= s.upper; p++ {
		if !s.payLaterOK(o, p) {
			continue
		}
		fee := CalculateFee(p, s.in.Market.FeeBps, s.in.Market.FeeFixedCents)
		contrib := p - s.in.Product.CostCents - s.bundleCost(o) - shipCost - fee
		if !meetsMargin(contrib, p, s.marginBps) {
			continue
		}
		uS := contrib + s.bundleHolding(o)
		uB := vB - p - s.dB
		if uB <= 0 || uS <= 0 {
			continue
		}
		pts = append(pts, [2]float64{float64(uB) / 100.0, float64(uS) / 100.0})
		obj := alpha*math.Log(float64(uB)) + (1.0-alpha)*math.Log(float64(uS))
		current := &candidate{opt: o, rawPrice: p, fee: fee, contrib: contrib, obj: obj}
		if optionBest == nil || obj > optionBest.obj+1e-12 {
			optionBest = current
		}
		if s.best == nil || obj > s.best.obj+1e-12 {
			s.best = &candidate{opt: o, rawPrice: p, fee: fee, contrib: contrib, obj: obj}
		}
	}
	if optionBest != nil {
		s.alternatives = append(s.alternatives, optionBest)
	}
	if len(pts) > 0 {
		s.frontiers = append(s.frontiers, optionPoints{opt: o, points: pts})
	}
}

// finalize applies the whole-dollar rounding policy and recomputes surpluses.
func (s *solver) finalize() *Result {
	b := s.best
	shipCost := s.shipCost(b.opt.shipping)
	price := (b.rawPrice / 100) * 100
	fee := CalculateFee(price, s.in.Market.FeeBps, s.in.Market.FeeFixedCents)
	contrib := price - s.in.Product.CostCents - s.bundleCost(b.opt) - shipCost - fee

	if price < s.lower || !meetsMargin(contrib, price, s.marginBps) || !s.payLaterOK(b.opt, price) {
		price, fee, contrib = b.rawPrice, b.fee, b.contrib
	}

	uB := s.buyerValue(b.opt) - price - s.dB
	uS := contrib + s.bundleHolding(b.opt)

	return &Result{
		PriceCents:         price,
		RawPriceCents:      b.rawPrice,
		Shipping:           b.opt.shipping,
		PayLater:           b.opt.bnpl,
		Bundle:             b.opt.bundle,
		FeeCents:           fee,
		ContributionCents:  contrib,
		BuyerSurplusCents:  uB,
		SellerSurplusCents: uS,
		StandardTotalCents: s.standardTo,
		Trace:              s.buildTrace(uB, uS),
	}
}

func (s *solver) buildTrace(uB, uS money.Cents) []Frontier {
	nash := [2]float64{float64(uB) / 100.0, float64(uS) / 100.0}
	out := make([]Frontier, 0, len(s.frontiers))
	for _, f := range s.frontiers {
		out = append(out, Frontier{
			Label:     formatOptionLabel(f.opt.shipping, f.opt.bnpl, f.opt.bundle),
			Selected:  f.opt == s.best.opt,
			Points:    downsamplePoints(f.points, 25),
			NashPoint: nash,
		})
	}
	return out
}

func downsamplePoints(pts [][2]float64, maxPts int) [][2]float64 {
	if len(pts) <= maxPts {
		return pts
	}
	res := make([][2]float64, 0, maxPts)
	step := float64(len(pts)-1) / float64(maxPts-1)
	for i := 0; i < maxPts; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx >= len(pts) {
			idx = len(pts) - 1
		}
		res = append(res, pts[idx])
	}
	return res
}

func formatOptionLabel(ship string, bnpl, bundle bool) string {
	shipLabel := "Ground"
	if ship == ShippingExpress {
		shipLabel = "Express"
	}
	bnplLabel := "Standard Pay"
	if bnpl {
		bnplLabel = "Pay in 4"
	}
	bundleLabel := "Single"
	if bundle {
		bundleLabel = "Bundle"
	}
	return fmt.Sprintf("%s · %s · %s", shipLabel, bnplLabel, bundleLabel)
}

// Compare the rounded, revalidated economic result of every strategy.
func (s *solver) reconsiderRounded() {
	bestObjective := math.Inf(-1)
	for _, c := range s.alternatives {
		p := (c.rawPrice / 100) * 100
		fee := CalculateFee(p, s.in.Market.FeeBps, s.in.Market.FeeFixedCents)
		contribution := p - s.in.Product.CostCents - s.bundleCost(c.opt) - s.shipCost(c.opt.shipping) - fee
		if p < s.lower || !meetsMargin(contribution, p, s.marginBps) || !s.payLaterOK(c.opt, p) {
			p = c.rawPrice
			contribution = c.contrib
		}
		buyer := s.buyerValue(c.opt) - p - s.dB
		seller := contribution + s.bundleHolding(c.opt)
		if buyer <= 0 || seller <= 0 {
			continue
		}
		objective := s.in.Policy.BuyerPower*math.Log(float64(buyer)) + (1-s.in.Policy.BuyerPower)*math.Log(float64(seller))
		if objective > bestObjective+1e-12 {
			bestObjective = objective
			s.best = c
		}
	}
}
