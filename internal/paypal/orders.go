package paypal

import (
	"errors"
	"fmt"

	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/store"
)

// BuildOrderRequest constructs an atomic PayPal Orders v2 request from an agreement.
// Mathematical identity strictly enforced: value = item_total + shipping - discount
func BuildOrderRequest(agr *store.AgreementRecord, mainProd *store.Product, bundleProd *store.Product, baseURL string) (*OrderRequest, error) {
	if agr == nil || mainProd == nil {
		return nil, errors.New("agreement and main product must not be nil")
	}

	priceCents := money.Cents(agr.PriceCents)
	items := make([]OrderItemDTO, 0, 2)

	// Item 1: Main product
	items = append(items, OrderItemDTO{
		Name:     mainProd.Name,
		SKU:      mainProd.SKU,
		Quantity: "1",
		Category: "PHYSICAL_GOODS",
		UnitAmount: MoneyDTO{
			CurrencyCode: "USD",
			Value:        priceCents.Format(),
		},
	})

	itemTotalCents := priceCents
	var discountDTO *MoneyDTO

	// Item 2: Bundle gift (if included)
	if agr.BundleSKU != nil && bundleProd != nil {
		bundleValCents := money.Cents(bundleProd.RetailValueCents)
		items = append(items, OrderItemDTO{
			Name:     fmt.Sprintf("%s (NashPay Bundle Concession)", bundleProd.Name),
			SKU:      bundleProd.SKU,
			Quantity: "1",
			Category: "PHYSICAL_GOODS",
			UnitAmount: MoneyDTO{
				CurrencyCode: "USD",
				Value:        bundleValCents.Format(),
			},
		})
		itemTotalCents += bundleValCents
		discountDTO = &MoneyDTO{
			CurrencyCode: "USD",
			Value:        bundleValCents.Format(),
		}
	}

	// Validate rule: value = item_total + shipping - discount
	discountVal := money.Cents(0)
	if discountDTO != nil {
		d, _ := money.Parse(discountDTO.Value)
		discountVal = d
	}
	expectedValue := itemTotalCents - discountVal
	if expectedValue != priceCents {
		return nil, fmt.Errorf("paypal breakdown invariant failed: item_total (%d) - discount (%d) != final_price (%d)",
			itemTotalCents, discountVal, priceCents)
	}

	breakdown := &AmountBreakdownDTO{
		ItemTotal: MoneyDTO{CurrencyCode: "USD", Value: itemTotalCents.Format()},
		Shipping:  MoneyDTO{CurrencyCode: "USD", Value: "0.00"},
		Discount:  discountDTO,
	}

	req := &OrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []PurchaseUnitDTO{
			{
				ReferenceID: agr.ID,
				CustomID:    agr.ID,
				InvoiceID:   "NASH-" + agr.ID,
				Description: fmt.Sprintf("NashPay Cleared Contract: %s", mainProd.Name),
				Amount: AmountDTO{
					CurrencyCode: "USD",
					Value:        priceCents.Format(),
					Breakdown:    breakdown,
				},
				Items: items,
			},
		},
		PaymentSource: PaymentSourceDTO{
			PayPal: PayPalPaymentSourceDTO{
				ExperienceContext: ExperienceContextDTO{
					BrandName:          "NashPay Demo Store",
					ShippingPreference: "GET_FROM_FILE",
					UserAction:         "PAY_NOW",
					ReturnURL:          baseURL + "/checkout/confirmed",
					CancelURL:          baseURL + "/",
				},
			},
		},
	}

	return req, nil
}
