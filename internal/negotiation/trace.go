package negotiation

import (
	"context"

	"github.com/nashpay/nashpay/internal/clearing"
	"github.com/nashpay/nashpay/internal/money"
)

// FrontierDTO is a public Pareto curve with coordinates normalised to [0,1].
type FrontierDTO struct {
	Label    string       `json:"label"`
	Selected bool         `json:"selected"`
	Points   [][2]float64 `json:"points"`
}

// TraceDTO is the public SOLVER_TRACE payload. All values are dimensionless ratios in [0,1]
// so that neither prices nor merchant surplus in dollars ever reach the browser (BR-20).
type TraceDTO struct {
	Frontiers    []FrontierDTO `json:"frontiers"`
	NashPoint    [2]float64    `json:"nash_point"`
	Disagreement [2]float64    `json:"disagreement"`
}

// NormalizeTrace divides every [ub, us] pair by the frame maxima (spec §4.4).
func NormalizeTrace(frontiers []clearing.Frontier) *TraceDTO {
	maxUb, maxUs := 0.0, 0.0
	for _, f := range frontiers {
		for _, p := range f.Points {
			if p[0] > maxUb {
				maxUb = p[0]
			}
			if p[1] > maxUs {
				maxUs = p[1]
			}
		}
	}
	if maxUb <= 0 {
		maxUb = 1
	}
	if maxUs <= 0 {
		maxUs = 1
	}

	out := &TraceDTO{Frontiers: make([]FrontierDTO, 0, len(frontiers))}
	for _, f := range frontiers {
		pts := make([][2]float64, len(f.Points))
		for i, p := range f.Points {
			pts[i] = [2]float64{p[0] / maxUb, p[1] / maxUs}
		}
		out.Frontiers = append(out.Frontiers, FrontierDTO{Label: f.Label, Selected: f.Selected, Points: pts})
		if f.Selected {
			out.NashPoint = [2]float64{f.NashPoint[0] / maxUb, f.NashPoint[1] / maxUs}
		}
	}
	return out
}

// PublicAgreementDTO is the only view of an agreement exposed by GET /api/agreements/{id}/public.
type PublicAgreementDTO struct {
	AgreementID   string         `json:"agreement_id"`
	Status        string         `json:"status"`
	FinalPrice    string         `json:"final_price"`
	Currency      string         `json:"currency"`
	Concessions   ConcessionsDTO `json:"concessions"`
	PayPalOrderID string         `json:"paypal_order_id,omitempty"`
	CaptureID     string         `json:"capture_id,omitempty"`
}

// PublicView builds the sanitised receipt for an agreement (no cost, fee, margin or surplus).
func (s *Service) PublicView(ctx context.Context, agreementID string) (*PublicAgreementDTO, error) {
	agr, err := s.store.GetAgreement(ctx, agreementID)
	if err != nil {
		return nil, err
	}

	var bundle *clearing.Product
	if agr.BundleSKU != nil {
		if bp, err := s.store.GetProduct(ctx, *agr.BundleSKU); err == nil {
			bundle = &clearing.Product{SKU: bp.SKU, RetailValueCents: money.Cents(bp.RetailValueCents)}
		}
	}
	res := &clearing.Result{
		PriceCents: money.Cents(agr.PriceCents),
		Shipping:   agr.ShippingTier,
		PayLater:   agr.PayLater,
		Bundle:     bundle != nil,
	}

	dto := &PublicAgreementDTO{
		AgreementID: agr.ID,
		Status:      agr.Status,
		FinalPrice:  res.PriceCents.Format(),
		Currency:    "USD",
		Concessions: s.buildConcessions(ctx, res, bundle),
	}
	if agr.PayPalOrderID != nil {
		dto.PayPalOrderID = *agr.PayPalOrderID
	}
	if agr.CaptureID != nil {
		dto.CaptureID = *agr.CaptureID
	}
	return dto, nil
}
