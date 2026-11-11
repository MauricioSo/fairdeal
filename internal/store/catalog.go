package store

import (
	"context"
	"errors"
)

func (s *Store) SaveCatalogProduct(ctx context.Context, p Product) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_reservations r JOIN agreements a ON a.id=r.agreement_id WHERE r.state='HELD' AND (a.sku=? OR a.bundle_sku=?)", p.SKU, p.SKU).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return errors.New("active reservation")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO products(sku,name,list_price_cents,cost_cents,retail_value_cents,holding_cost_cents,is_deadstock,stock)VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(sku)DO UPDATE SET name=excluded.name,list_price_cents=excluded.list_price_cents,cost_cents=excluded.cost_cents,retail_value_cents=excluded.retail_value_cents,holding_cost_cents=excluded.holding_cost_cents,is_deadstock=excluded.is_deadstock,stock=excluded.stock`, p.SKU, p.Name, p.ListPriceCents, p.CostCents, p.RetailValueCents, p.HoldingCostCents, p.IsDeadstock, p.Stock)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE agreements SET status='EXPIRED',status_reason='CATALOG_CHANGED' WHERE status='OFFERED' AND (sku=? OR bundle_sku=?)", p.SKU, p.SKU)
	if err != nil {
		return err
	}
	return tx.Commit()
}
