package paypal

import (
	"context"
	"errors"
)

var (
	ErrOrderNotFound       = errors.New("paypal order not found")
	ErrOrderNotApproved    = errors.New("paypal order has not been approved by buyer")
	ErrOrderAmountMismatch = errors.New("paypal order amount does not match agreement")
	ErrCaptureFailed       = errors.New("paypal order capture failed")
)

type MoneyDTO struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type AmountBreakdownDTO struct {
	ItemTotal MoneyDTO  `json:"item_total"`
	Shipping  MoneyDTO  `json:"shipping"`
	Discount  *MoneyDTO `json:"discount,omitempty"`
}

type AmountDTO struct {
	CurrencyCode string              `json:"currency_code"`
	Value        string              `json:"value"`
	Breakdown    *AmountBreakdownDTO `json:"breakdown,omitempty"`
}

type OrderItemDTO struct {
	Name       string   `json:"name"`
	SKU        string   `json:"sku"`
	Quantity   string   `json:"quantity"`
	Category   string   `json:"category"`
	UnitAmount MoneyDTO `json:"unit_amount"`
}

type ExperienceContextDTO struct {
	BrandName          string `json:"brand_name"`
	ShippingPreference string `json:"shipping_preference"`
	UserAction         string `json:"user_action"`
	ReturnURL          string `json:"return_url,omitempty"`
	CancelURL          string `json:"cancel_url,omitempty"`
}

type PayPalPaymentSourceDTO struct {
	ExperienceContext ExperienceContextDTO `json:"experience_context"`
}

type PaymentSourceDTO struct {
	PayPal PayPalPaymentSourceDTO `json:"paypal"`
}

type PurchaseUnitDTO struct {
	Payments struct {
		Captures []Capture `json:"captures"`
	} `json:"payments,omitempty"`
	ReferenceID string         `json:"reference_id"`
	CustomID    string         `json:"custom_id"`
	InvoiceID   string         `json:"invoice_id"`
	Description string         `json:"description,omitempty"`
	Amount      AmountDTO      `json:"amount"`
	Items       []OrderItemDTO `json:"items"`
}

type OrderRequest struct {
	Intent        string            `json:"intent"`
	PurchaseUnits []PurchaseUnitDTO `json:"purchase_units"`
	PaymentSource PaymentSourceDTO  `json:"payment_source"`
}

type Order struct {
	ID            string            `json:"id"`
	Status        string            `json:"status"` // CREATED, SAVED, APPROVED, VOIDED, COMPLETED
	PurchaseUnits []PurchaseUnitDTO `json:"purchase_units"`
}

type Capture struct {
	ID     string   `json:"id"`
	Status string   `json:"status"` // COMPLETED, DECLINED, FAILED
	Amount MoneyDTO `json:"amount"`
}

type Client interface {
	CreateOrder(ctx context.Context, req OrderRequest) (orderID string, err error)
	GetOrder(ctx context.Context, id string) (*Order, error)
	CaptureOrder(ctx context.Context, id string, requestID string) (*Capture, error)
}
