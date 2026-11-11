package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	StatusOffered      = "OFFERED"
	StatusOrderCreated = "ORDER_CREATED"
	StatusCaptured     = "CAPTURED"
	StatusFailed       = "FAILED"
	StatusExpired      = "EXPIRED"
)

type AgreementRecord struct {
	Economics          *EconomicSnapshot `json:"-"`
	DeliveryCountry    string            `json:"delivery_country,omitempty"`
	ID                 string            `json:"id"`
	NegotiationID      string            `json:"negotiation_id"`
	SessionID          string            `json:"session_id"`
	SKU                string            `json:"sku"`
	BundleSKU          *string           `json:"bundle_sku"`
	PriceCents         int64             `json:"price_cents"`
	ShippingTier       string            `json:"shipping_tier"`
	PayLater           bool              `json:"pay_later"`
	BuyerBudgetCents   int64             `json:"-"`
	StandardTotalCents int64             `json:"standard_total_cents"`
	FeeCents           int64             `json:"fee_cents"`
	ContributionCents  int64             `json:"contribution_cents"`
	BuyerSurplusCents  int64             `json:"-"`
	SellerSurplusCents int64             `json:"seller_surplus_cents"`
	PolicyID           int64             `json:"policy_id"`
	Status             string            `json:"status"`
	StatusReason       *string           `json:"status_reason"`
	PayPalOrderID      *string           `json:"paypal_order_id"`
	CaptureID          *string           `json:"capture_id"`
	SolverLatencyUs    int64             `json:"solver_latency_us"`
	CreatedAt          int64             `json:"created_at"`
	ExpiresAt          int64             `json:"expires_at"`
	UpdatedAt          int64             `json:"updated_at"`
}

// IsValidTransition checks if moving from 'from' to 'to' is permitted.
func IsValidTransition(from, to string) bool {
	switch from {
	case StatusOffered:
		return to == StatusOrderCreated || to == StatusExpired
	case StatusOrderCreated:
		return to == StatusCaptured || to == StatusFailed || to == StatusExpired
	default:
		return false
	}
}

// SetPayPalOrderID attaches the PayPal order id to an ORDER_CREATED agreement exactly once.
func (s *Store) SetPayPalOrderID(ctx context.Context, id, orderID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE agreements SET paypal_order_id = ?, updated_at = ?
		WHERE id = ? AND status = 'ORDER_CREATED' AND paypal_order_id IS NULL
	`, orderID, NowUnix(), id)
	if err != nil {
		return fmt.Errorf("set paypal order id: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// CreateAgreement persists an initial OFFERED agreement record.
func (s *Store) CreateAgreement(ctx context.Context, a *AgreementRecord) error {
	payLaterInt := 0
	if a.PayLater {
		payLaterInt = 1
	}

	now := NowUnix()
	a.CreatedAt = now
	a.UpdatedAt = now
	a.Status = StatusOffered

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO agreements (
			id, negotiation_id, session_id, sku, bundle_sku, price_cents, shipping_tier,
			pay_later, buyer_budget_cents, standard_total_cents, fee_cents, contribution_cents,
			buyer_surplus_cents, seller_surplus_cents, policy_id, status, status_reason,
			paypal_order_id, capture_id, solver_latency_us, created_at, expires_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?
		)
	`,
		a.ID, a.NegotiationID, a.SessionID, a.SKU, a.BundleSKU, a.PriceCents, a.ShippingTier,
		payLaterInt, a.BuyerBudgetCents, a.StandardTotalCents, a.FeeCents, a.ContributionCents,
		a.BuyerSurplusCents, a.SellerSurplusCents, a.PolicyID, a.Status, a.StatusReason,
		a.PayPalOrderID, a.CaptureID, a.SolverLatencyUs, a.CreatedAt, a.ExpiresAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create agreement: %w", err)
	}
	if a.Economics != nil {
		revision, e := economicRevision(ctx, tx)
		if e != nil {
			return e
		}
		if revision != a.Economics.Revision {
			return ErrEconomicChange
		}
		data, e := json.Marshal(a.Economics)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO agreement_economics(agreement_id,snapshot_json)VALUES(?,?)", a.ID, string(data)); e != nil {
			return e
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO agreement_delivery(agreement_id,country)VALUES(?,?)", a.ID, a.DeliveryCountry); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO merchant_events(kind,agreement_id,created_at)VALUES('QUOTE_CREATED',?,?)", a.ID, NowUnix()); err != nil {
		return err
	}
	return tx.Commit()
}

// GetAgreement retrieves an agreement by ID.
func (s *Store) GetAgreement(ctx context.Context, id string) (*AgreementRecord, error) {
	var a AgreementRecord
	var payLaterInt int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, negotiation_id, session_id, sku, bundle_sku, price_cents, shipping_tier,
		       pay_later, buyer_budget_cents, standard_total_cents, fee_cents, contribution_cents,
		       buyer_surplus_cents, seller_surplus_cents, policy_id, status, status_reason,
		       paypal_order_id, capture_id, solver_latency_us, created_at, expires_at, updated_at
		FROM agreements WHERE id = ?
	`, id).Scan(
		&a.ID, &a.NegotiationID, &a.SessionID, &a.SKU, &a.BundleSKU, &a.PriceCents, &a.ShippingTier,
		&payLaterInt, &a.BuyerBudgetCents, &a.StandardTotalCents, &a.FeeCents, &a.ContributionCents,
		&a.BuyerSurplusCents, &a.SellerSurplusCents, &a.PolicyID, &a.Status, &a.StatusReason,
		&a.PayPalOrderID, &a.CaptureID, &a.SolverLatencyUs, &a.CreatedAt, &a.ExpiresAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.PayLater = payLaterInt == 1
	_ = s.db.QueryRowContext(ctx, "SELECT country FROM agreement_delivery WHERE agreement_id=?", id).Scan(&a.DeliveryCountry)
	return &a, nil
}

// GetAgreementByPayPalOrderID retrieves an agreement by its PayPal order ID.
func (s *Store) GetAgreementByPayPalOrderID(ctx context.Context, orderID string) (*AgreementRecord, error) {
	var a AgreementRecord
	var payLaterInt int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, negotiation_id, session_id, sku, bundle_sku, price_cents, shipping_tier,
		       pay_later, buyer_budget_cents, standard_total_cents, fee_cents, contribution_cents,
		       buyer_surplus_cents, seller_surplus_cents, policy_id, status, status_reason,
		       paypal_order_id, capture_id, solver_latency_us, created_at, expires_at, updated_at
		FROM agreements WHERE paypal_order_id = ?
	`, orderID).Scan(
		&a.ID, &a.NegotiationID, &a.SessionID, &a.SKU, &a.BundleSKU, &a.PriceCents, &a.ShippingTier,
		&payLaterInt, &a.BuyerBudgetCents, &a.StandardTotalCents, &a.FeeCents, &a.ContributionCents,
		&a.BuyerSurplusCents, &a.SellerSurplusCents, &a.PolicyID, &a.Status, &a.StatusReason,
		&a.PayPalOrderID, &a.CaptureID, &a.SolverLatencyUs, &a.CreatedAt, &a.ExpiresAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.PayLater = payLaterInt == 1
	return &a, nil
}

// TransitionAgreement atomically updates agreement status from fromStatus to toStatus.
func (s *Store) TransitionAgreement(ctx context.Context, id string, fromStatus, toStatus string, reason *string, payPalOrderID, captureID *string) error {
	return s.transitionWithInventory(ctx, id, fromStatus, toStatus, reason, payPalOrderID, captureID)
}

// ExpireStale marks as EXPIRED:
// - Any OFFERED agreement past its expires_at
// - Any ORDER_CREATED agreement older than 3600 seconds
func (s *Store) ExpireStale(ctx context.Context, nowUnix int64) (int64, error) {
	reasonTTL := "TTL_EXPIRED"
	resOffered, err := s.db.ExecContext(ctx, `
		UPDATE agreements
		SET status = 'EXPIRED', status_reason = ?, updated_at = ?
		WHERE status = 'OFFERED' AND expires_at < ?
	`, reasonTTL, nowUnix, nowUnix)
	if err != nil {
		return 0, fmt.Errorf("expire offered: %w", err)
	}

	reasonTimeout := "ORDER_CREATED_TIMEOUT"
	timeoutCutoff := nowUnix - 3600
	resCreated, err := s.db.ExecContext(ctx, `
		UPDATE agreements
		SET status = 'EXPIRED', status_reason = ?, updated_at = ?
		WHERE status = 'ORDER_CREATED' AND created_at < ? AND id NOT IN (SELECT agreement_id FROM inventory_reservations WHERE state='HELD') AND id NOT IN (SELECT agreement_id FROM payment_reconciliation)
	`, reasonTimeout, nowUnix, timeoutCutoff)
	if err != nil {
		return 0, fmt.Errorf("expire created: %w", err)
	}

	r1, _ := resOffered.RowsAffected()
	r2, _ := resCreated.RowsAffected()
	return r1 + r2, nil
}

// GetLiveAgreementBySessionAndSKU returns the single "live" agreement of a (session, sku):
// an OFFERED one that has not expired, or an ORDER_CREATED one younger than 3600 s.
func (s *Store) GetLiveAgreementBySessionAndSKU(ctx context.Context, sessionID, sku string, nowUnix int64) (*AgreementRecord, error) {
	var a AgreementRecord
	var payLaterInt int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, negotiation_id, session_id, sku, bundle_sku, price_cents, shipping_tier,
		       pay_later, buyer_budget_cents, standard_total_cents, fee_cents, contribution_cents,
		       buyer_surplus_cents, seller_surplus_cents, policy_id, status, status_reason,
		       paypal_order_id, capture_id, solver_latency_us, created_at, expires_at, updated_at
		FROM agreements
		WHERE session_id = ? AND sku = ?
		  AND ((status = 'OFFERED' AND expires_at > ?) OR (status = 'ORDER_CREATED' AND (created_at > ? OR id IN (SELECT agreement_id FROM inventory_reservations WHERE state='HELD')) ))
		ORDER BY created_at DESC LIMIT 1
	`, sessionID, sku, nowUnix, nowUnix-3600).Scan(
		&a.ID, &a.NegotiationID, &a.SessionID, &a.SKU, &a.BundleSKU, &a.PriceCents, &a.ShippingTier,
		&payLaterInt, &a.BuyerBudgetCents, &a.StandardTotalCents, &a.FeeCents, &a.ContributionCents,
		&a.BuyerSurplusCents, &a.SellerSurplusCents, &a.PolicyID, &a.Status, &a.StatusReason,
		&a.PayPalOrderID, &a.CaptureID, &a.SolverLatencyUs, &a.CreatedAt, &a.ExpiresAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.PayLater = payLaterInt == 1
	return &a, nil
}
