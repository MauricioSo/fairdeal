package clearing

import (
	"errors"

	"github.com/nashpay/nashpay/internal/money"
)

var (
	ErrNoZOPA = errors.New("no zone of possible agreement")
)

const (
	ShippingExpress            = "EXPRESS"
	ShippingConsolidatedGround = "CONSOLIDATED_GROUND"
)

type Product struct {
	SKU              string
	ListPriceCents   money.Cents
	CostCents        money.Cents
	RetailValueCents money.Cents
	HoldingCostCents money.Cents
}

type Market struct {
	ExpressShipCostCents        money.Cents
	GroundShipCostCents         money.Cents
	StandardShippingChargeCents money.Cents
	WaitCostMaxCents            money.Cents
	PayLaterBuyerValueCents     money.Cents
	BundleValuePct              float64
	FeeBps                      int64
	FeeFixedCents               money.Cents
	PayLaterMinCents            money.Cents
	PayLaterMaxCents            money.Cents
}

type Policy struct {
	MinMarginPct      float64
	MaxDiscountPct    float64
	BuyerPower        float64
	AllowSlowShipping bool
	PayLaterEnabled   bool
}

type BuyerConstraints struct {
	DeliveryCountry     string
	Budget              money.Cents
	ShippingFlexibility float64
	PrefersPayLater     bool
	WillingToBundle     bool
}

type Point struct {
	Ub float64 `json:"ub"`
	Us float64 `json:"us"`
}

type Frontier struct {
	Label     string       `json:"label"`
	Selected  bool         `json:"selected"`
	Points    [][2]float64 `json:"points"` // [ub, us] in dollars or utilities
	NashPoint [2]float64   `json:"nash_point"`
}

type Input struct {
	ReferenceValue money.Cents
	Product        Product
	Bundle         *Product
	Market         Market
	Policy         Policy
	Buyer          BuyerConstraints
	ForbidPayLater bool
}

type Result struct {
	PriceCents         money.Cents
	RawPriceCents      money.Cents
	Shipping           string
	PayLater           bool
	Bundle             bool
	FeeCents           money.Cents
	ContributionCents  money.Cents
	BuyerSurplusCents  money.Cents
	SellerSurplusCents money.Cents
	StandardTotalCents money.Cents
	Trace              []Frontier
}
