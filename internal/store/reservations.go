package store

import (
	"context"
	"errors"
)

var ErrOutOfStock = errors.New("product is no longer available")

// transitionWithInventory atomically reserves/consumes/releases both products.
// Stock is available stock: deducted on reserve, restored only on confirmed failure.
func (s *Store) transitionWithInventory(ctx context.Context, id, from, to string, reason, orderID, captureID *string, economicCheck ...bool) error {
	if !IsValidTransition(from, to) {
		return ErrInvalidTransition
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current, sku string
	var bundle *string
	if err = tx.QueryRowContext(ctx, "SELECT status,sku,bundle_sku FROM agreements WHERE id=?", id).Scan(&current, &sku, &bundle); err != nil {
		return ErrNotFound
	}
	if current != from {
		return ErrAlreadyUsed
	}
	if from == StatusOrderCreated && to == StatusExpired {
		var pending int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_reconciliation WHERE agreement_id=?", id).Scan(&pending); err != nil {
			return err
		}
		if pending > 0 {
			return ErrInvalidTransition
		}
	}
	if from == StatusOffered && to == StatusOrderCreated {
		if len(economicCheck) > 0 && economicCheck[0] {
			if err = checkEconomicSnapshot(ctx, tx, id); err != nil {
				return err
			}
		}
		products := []string{sku}
		if bundle != nil {
			products = append(products, *bundle)
		}
		for _, p := range products {
			res, e := tx.ExecContext(ctx, "UPDATE products SET stock=stock-1 WHERE sku=? AND stock>0", p)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n != 1 {
				return ErrOutOfStock
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO inventory_reservations(agreement_id,state)VALUES(?,'HELD')", id); err != nil {
			return err
		}
	}
	if from == StatusOrderCreated && (to == StatusCaptured || to == StatusFailed || to == StatusExpired) {
		state := "CONSUMED"
		if to != StatusCaptured {
			state = "RELEASED"
		}
		result, e := tx.ExecContext(ctx, "UPDATE inventory_reservations SET state=? WHERE agreement_id=? AND state='HELD'", state, id)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n == 1 && state == "RELEASED" {
			if _, err = tx.ExecContext(ctx, "UPDATE products SET stock=stock+1 WHERE sku=?", sku); err != nil {
				return err
			}
			if bundle != nil {
				if _, err = tx.ExecContext(ctx, "UPDATE products SET stock=stock+1 WHERE sku=?", *bundle); err != nil {
					return err
				}
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE agreements SET status=?,status_reason=COALESCE(?,status_reason),paypal_order_id=COALESCE(?,paypal_order_id),capture_id=COALESCE(?,capture_id),updated_at=? WHERE id=? AND status=?", to, reason, orderID, captureID, NowUnix(), id, from); err != nil {
		return err
	}
	if to == StatusCaptured || to == StatusFailed {
		if _, err = tx.ExecContext(ctx, "DELETE FROM payment_reconciliation WHERE agreement_id=?", id); err != nil {
			return err
		}
	}
	event := to
	if to == StatusOrderCreated {
		event = "INVENTORY_RESERVED"
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO merchant_events(kind,agreement_id,created_at)VALUES(?,?,?)", event, id, NowUnix()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) MarkPaymentUncertain(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO payment_reconciliation(agreement_id,created_at) SELECT id,? FROM agreements WHERE id=? AND status='ORDER_CREATED' ON CONFLICT(agreement_id)DO NOTHING", NowUnix(), id)
	return err
}
func (s *Store) PaymentUncertain(ctx context.Context, id string) bool {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_reconciliation WHERE agreement_id=?", id).Scan(&n)
	return err == nil && n > 0
}

// BeginCapture makes the in-flight marker durable before contacting the provider.
func (s *Store) BeginCapture(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE agreements SET updated_at=? WHERE id=? AND status='ORDER_CREATED'", NowUnix(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrAlreadyUsed
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO payment_reconciliation(agreement_id,created_at)VALUES(?,?) ON CONFLICT(agreement_id)DO NOTHING", id, NowUnix()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ClearPaymentUncertain(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM payment_reconciliation WHERE agreement_id=?", id)
	return err
}
func (s *Store) ExpireReservations(ctx context.Context, now int64) error {
	rows, err := s.db.QueryContext(ctx, "SELECT a.id FROM agreements a JOIN inventory_reservations r ON r.agreement_id=a.id WHERE a.status='ORDER_CREATED' AND r.state='HELD' AND a.created_at<? AND a.id NOT IN (SELECT agreement_id FROM payment_reconciliation)", now-300)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	reason := "RESERVATION_EXPIRED"
	for _, id := range ids {
		err = s.TransitionAgreement(ctx, id, StatusOrderCreated, StatusExpired, &reason, nil, nil)
		if err != nil && !errors.Is(err, ErrAlreadyUsed) && !errors.Is(err, ErrInvalidTransition) {
			return err
		}
	}
	return nil
}
