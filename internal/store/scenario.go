package store

import (
	"context"
	"encoding/json"
)

// ConfigureDemoMerchant applies a scenario exactly once, never resetting sales or stock.
func (s *Store) ConfigureDemoMerchant(ctx context.Context, id string, list, cost, stock int64, accessory string, acost, avalue, astock, express, ground int64, margin, discount float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM settings WHERE key='scenario-v1'").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if _, err = tx.ExecContext(ctx, "UPDATE products SET list_price_cents=?,retail_value_cents=?,cost_cents=?,stock=? WHERE sku='CH-WINTER-PARKA-01'", list, list, cost, stock); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE products SET name=?,list_price_cents=?,retail_value_cents=?,cost_cents=?,stock=? WHERE sku='SKU-SCARF-09'", accessory, avalue, avalue, acost, astock); err != nil {
		return err
	}
	market := MarketSettings{}
	var raw string
	if err = tx.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='market'").Scan(&raw); err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(raw), &market); err != nil {
		return err
	}
	market.ExpressShipCostCents = express
	market.GroundShipCostCents = ground
	market.StandardShippingChargeCents = express
	data, _ := json.Marshal(market)
	if _, err = tx.ExecContext(ctx, "UPDATE settings SET value_json=? WHERE key='market'", string(data)); err != nil {
		return err
	}
	sku := "SKU-SCARF-09"
	p := PolicyConfig{MinMarginPct: margin, MaxDiscountPct: discount, BuyerPower: .35, AllowSlowShipping: true, PayLaterEnabled: true, BundleSKU: &sku, Rationale: "Fictional merchant scenario"}
	if accessory == "" {
		p.BundleSKU = nil
	}
	data, _ = json.Marshal(p)
	if _, err = tx.ExecContext(ctx, "UPDATE merchant_policies SET policy_json=? WHERE status='ACTIVE'", string(data)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO settings(key,value_json)VALUES('scenario-v1',?)", `"`+id+`"`); err != nil {
		return err
	}
	return tx.Commit()
}
