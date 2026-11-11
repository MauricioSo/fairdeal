package store

import (
	"context"
)

// Oldest attempted pending records first, independent of recent shopping activity.
func (s *Store) PendingReconciliation(ctx context.Context) ([]AgreementRecord, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT a.id FROM agreements a JOIN payment_reconciliation p ON p.agreement_id=a.id WHERE a.status='ORDER_CREATED' ORDER BY a.updated_at,a.id LIMIT 10")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []AgreementRecord{}
	for _, id := range ids {
		a, e := s.GetAgreement(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, *a)
	}
	return out, nil
}
func (s *Store) NoteReconciliationAttempt(ctx context.Context, id string) {
	_, _ = s.db.ExecContext(ctx, "UPDATE agreements SET updated_at=? WHERE id=? AND status='ORDER_CREATED'", NowUnix(), id)
}
