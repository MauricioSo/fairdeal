package store

import (
	"context"
)

// RecordEvent accepts server-controlled event kinds only; no prompts or budgets.
func (s *Store) RecordEvent(ctx context.Context, kind string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO merchant_events(kind,created_at)VALUES(?,?)", kind, NowUnix())
	return err
}
