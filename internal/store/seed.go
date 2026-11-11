package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type Product struct {
	SKU              string `json:"sku"`
	Name             string `json:"name"`
	ListPriceCents   int64  `json:"list_price_cents"`
	CostCents        int64  `json:"cost_cents"`
	RetailValueCents int64  `json:"retail_value_cents"`
	HoldingCostCents int64  `json:"holding_cost_cents"`
	IsDeadstock      bool   `json:"is_deadstock"`
	Stock            int    `json:"stock"`
}

type MarketSettings struct {
	ExpressShipCostCents        int64   `json:"express_ship_cost_cents"`
	GroundShipCostCents         int64   `json:"ground_ship_cost_cents"`
	StandardShippingChargeCents int64   `json:"standard_shipping_charge_cents"`
	WaitCostMaxCents            int64   `json:"wait_cost_max_cents"`
	PayLaterBuyerValueCents     int64   `json:"pay_later_buyer_value_cents"`
	BundleValuePct              float64 `json:"bundle_value_pct"`
	FeeBps                      int64   `json:"fee_bps"`
	FeeFixedCents               int64   `json:"fee_fixed_cents"`
	PayLaterMinCents            int64   `json:"pay_later_min_cents"`
	PayLaterMaxCents            int64   `json:"pay_later_max_cents"`
	AgreementTTLSeconds         int64   `json:"agreement_ttl_seconds"`
}

type PolicyConfig struct {
	MinMarginPct      float64 `json:"min_margin_pct"`
	MaxDiscountPct    float64 `json:"max_discount_pct"`
	BuyerPower        float64 `json:"buyer_power"`
	AllowSlowShipping bool    `json:"allow_slow_shipping"`
	PayLaterEnabled   bool    `json:"pay_later_enabled"`
	BundleSKU         *string `json:"bundle_sku"`
	Rationale         string  `json:"rationale"`
}

func (s *Store) seedIfEmpty(ctx context.Context) error {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM products").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // Already seeded
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Seed products
	products := []Product{
		{
			SKU:              "CH-WINTER-PARKA-01",
			Name:             "Winter Parka Performance 01",
			ListPriceCents:   12000,
			CostCents:        5000,
			RetailValueCents: 12000,
			HoldingCostCents: 0,
			IsDeadstock:      false,
			Stock:            40,
		},
		{
			SKU:              "SKU-SCARF-09",
			Name:             "Cashmere Scarf",
			ListPriceCents:   2500,
			CostCents:        900,
			RetailValueCents: 2500,
			HoldingCostCents: 500,
			IsDeadstock:      true,
			Stock:            500,
		},
	}

	prodStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO products (sku, name, list_price_cents, cost_cents, retail_value_cents, holding_cost_cents, is_deadstock, stock)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer prodStmt.Close()

	for _, p := range products {
		deadstockInt := 0
		if p.IsDeadstock {
			deadstockInt = 1
		}
		if _, err := prodStmt.ExecContext(ctx, p.SKU, p.Name, p.ListPriceCents, p.CostCents, p.RetailValueCents, p.HoldingCostCents, deadstockInt, p.Stock); err != nil {
			return fmt.Errorf("insert product %s: %w", p.SKU, err)
		}
	}

	// 2. Seed market settings
	market := MarketSettings{
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
		AgreementTTLSeconds:         300,
	}
	marketJSON, err := json.Marshal(market)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO settings (key, value_json) VALUES (?, ?)", "market", string(marketJSON)); err != nil {
		return fmt.Errorf("insert market settings: %w", err)
	}

	// 3. Seed active merchant policy
	bundleSKU := "SKU-SCARF-09"
	policy := PolicyConfig{
		MinMarginPct:      0.20,
		MaxDiscountPct:    0.30,
		BuyerPower:        0.35,
		AllowSlowShipping: true,
		PayLaterEnabled:   true,
		BundleSKU:         &bundleSKU,
		Rationale:         "Seed policy",
	}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return err
	}

	now := NowUnix()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO merchant_policies (created_at, status, source, source_prompt, policy_json, adjustments_json, hard_floor_margin_pct)
		VALUES (?, 'ACTIVE', 'seed', 'Initial default policy', ?, '[]', 0.15)
	`, now, string(policyJSON))
	if err != nil {
		return fmt.Errorf("insert seed policy: %w", err)
	}

	return tx.Commit()
}

// GetProduct retrieves a product by SKU.
func (s *Store) GetProduct(ctx context.Context, sku string) (*Product, error) {
	var p Product
	var isDeadstock int
	err := s.db.QueryRowContext(ctx, `
		SELECT sku, name, list_price_cents, cost_cents, retail_value_cents, holding_cost_cents, is_deadstock, stock
		FROM products WHERE sku = ?
	`, sku).Scan(&p.SKU, &p.Name, &p.ListPriceCents, &p.CostCents, &p.RetailValueCents, &p.HoldingCostCents, &isDeadstock, &p.Stock)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.IsDeadstock = isDeadstock == 1
	return &p, nil
}

// GetMarketSettings retrieves market parameters from settings table.
func (s *Store) GetMarketSettings(ctx context.Context) (*MarketSettings, error) {
	var val string
	err := s.db.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key = 'market'").Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var m MarketSettings
	if err := json.Unmarshal([]byte(val), &m); err != nil {
		return nil, fmt.Errorf("unmarshal market settings: %w", err)
	}
	return &m, nil
}
