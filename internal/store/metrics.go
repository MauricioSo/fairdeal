package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

type MetricsResult struct {
	Offers                   int64   `json:"offers"`
	Captured                 int64   `json:"captured"`
	RescuedCarts             int64   `json:"rescued_carts"`
	RecoveredGMV             string  `json:"recovered_gmv"` // e.g. "94.00"
	PayLaterShare            float64 `json:"pay_later_share"`
	AvgContributionMarginPct float64 `json:"avg_contribution_margin_pct"`
	DeadstockUnitsCleared    int64   `json:"deadstock_units_cleared"`
	SolverLatencyP50Us       int64   `json:"solver_latency_p50_us"`
	SolverLatencyP99Us       int64   `json:"solver_latency_p99_us"`
	Conversion               float64 `json:"conversion"`
}

// GetMetrics calculates aggregate business metrics across agreements.
func (s *Store) GetMetrics(ctx context.Context) (*MetricsResult, error) {
	var m MetricsResult

	// 1. Total offers
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agreements").Scan(&m.Offers); err != nil {
		return nil, err
	}

	// 2. Captured count
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agreements WHERE status = 'CAPTURED'").Scan(&m.Captured); err != nil {
		return nil, err
	}

	if m.Offers > 0 {
		m.Conversion = float64(m.Captured) / float64(m.Offers)
	}

	// 3. Rescued carts & recovered GMV
	var rescuedGMVCents sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), SUM(price_cents)
		FROM agreements
		WHERE status = 'CAPTURED' AND price_cents < standard_total_cents
	`).Scan(&m.RescuedCarts, &rescuedGMVCents)
	if err != nil {
		return nil, err
	}

	totalRescuedCents := int64(0)
	if rescuedGMVCents.Valid {
		totalRescuedCents = rescuedGMVCents.Int64
	}
	dollars := totalRescuedCents / 100
	cents := totalRescuedCents % 100
	m.RecoveredGMV = fmt.Sprintf("%d.%02d", dollars, cents)

	// 4. Pay in 4 share & deadstock cleared
	if m.Captured > 0 {
		var payLaterCount int64
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agreements WHERE status = 'CAPTURED' AND pay_later = 1").Scan(&payLaterCount); err != nil {
			return nil, err
		}
		m.PayLaterShare = float64(payLaterCount) / float64(m.Captured)

		// Deadstock cleared
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agreements WHERE status = 'CAPTURED' AND bundle_sku IS NOT NULL").Scan(&m.DeadstockUnitsCleared); err != nil {
			return nil, err
		}

		// New agreements use net revenue after included tax. Legacy captures keep their original gross basis.
		var avgMargin sql.NullFloat64
		if err := s.db.QueryRowContext(ctx, "SELECT AVG(CAST(a.contribution_cents AS REAL) / COALESCE(NULLIF(json_extract(e.snapshot_json,'$.evaluation.breakdown.net_revenue_cents'),0),a.price_cents)) FROM agreements a LEFT JOIN agreement_economics e ON e.agreement_id=a.id WHERE a.status = 'CAPTURED' AND a.price_cents > 0").Scan(&avgMargin); err != nil {
			return nil, err
		}
		if avgMargin.Valid {
			m.AvgContributionMarginPct = avgMargin.Float64
		}
	}

	// 5. Latency percentiles (P50 and P99)
	rows, err := s.db.QueryContext(ctx, "SELECT solver_latency_us FROM agreements WHERE solver_latency_us > 0")
	if err == nil {
		defer rows.Close()
		var latencies []int64
		for rows.Next() {
			var lat int64
			if err := rows.Scan(&lat); err == nil {
				latencies = append(latencies, lat)
			}
		}
		if len(latencies) > 0 {
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			p50Idx := len(latencies) * 50 / 100
			p99Idx := len(latencies) * 99 / 100
			if p99Idx >= len(latencies) {
				p99Idx = len(latencies) - 1
			}
			m.SolverLatencyP50Us = latencies[p50Idx]
			m.SolverLatencyP99Us = latencies[p99Idx]
		}
	}

	return &m, nil
}

// GetRecentAgreements returns up to limit recent agreements for the merchant audit log.
func (s *Store) GetRecentAgreements(ctx context.Context, limit int) ([]AgreementRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, negotiation_id, session_id, sku, bundle_sku, price_cents, shipping_tier,
		       pay_later, buyer_budget_cents, standard_total_cents, fee_cents, contribution_cents,
		       buyer_surplus_cents, seller_surplus_cents, policy_id, status, status_reason,
		       paypal_order_id, capture_id, solver_latency_us, created_at, expires_at, updated_at
		FROM agreements
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AgreementRecord
	for rows.Next() {
		var a AgreementRecord
		var payLaterInt int
		if err := rows.Scan(
			&a.ID, &a.NegotiationID, &a.SessionID, &a.SKU, &a.BundleSKU, &a.PriceCents, &a.ShippingTier,
			&payLaterInt, &a.BuyerBudgetCents, &a.StandardTotalCents, &a.FeeCents, &a.ContributionCents,
			&a.BuyerSurplusCents, &a.SellerSurplusCents, &a.PolicyID, &a.Status, &a.StatusReason,
			&a.PayPalOrderID, &a.CaptureID, &a.SolverLatencyUs, &a.CreatedAt, &a.ExpiresAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.PayLater = payLaterInt == 1
		list = append(list, a)
	}

	return list, rows.Err()
}

// ResetDemoData clears agreements and non-seed policies for a clean presentation run.
func (s *Store) ResetDemoData(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, "UPDATE products SET stock=stock+(SELECT COUNT(*) FROM inventory_reservations r JOIN agreements a ON a.id=r.agreement_id WHERE r.state IN ('HELD','CONSUMED') AND (a.sku=products.sku OR a.bundle_sku=products.sku))"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM payment_reconciliation"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM inventory_reservations"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM agreement_delivery"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM merchant_events"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM agreement_economics"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM agreements"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM merchant_policies WHERE source != 'seed'"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE merchant_policies SET status = 'ACTIVE' WHERE source = 'seed'"); err != nil {
		return err
	}

	return tx.Commit()
}
