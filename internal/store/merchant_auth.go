package store

import (
	"context"
	"database/sql"
	"errors"
)

type MerchantSession struct {
	CSRF                 string
	CreatedAt, ExpiresAt int64
}

func (s *Store) CreateMerchantSession(ctx context.Context, hash, csrf, version string, now int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM merchant_sessions WHERE expires_at<=? OR idle_expires_at<=? OR credential_version!=?`, now, now, version); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO merchant_sessions(session_hash,csrf_token,credential_version,created_at,expires_at,idle_expires_at) VALUES(?,?,?,?,?,?)`, hash, csrf, version, now, now+4*3600, now+1800)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MerchantSession(ctx context.Context, hash, version string, now int64) (*MerchantSession, error) {
	var session MerchantSession
	err := s.db.QueryRowContext(ctx, `UPDATE merchant_sessions SET idle_expires_at=MIN(expires_at,?) WHERE session_hash=? AND credential_version=? AND expires_at>? AND idle_expires_at>? RETURNING csrf_token,created_at,expires_at`, now+1800, hash, version, now, now).Scan(&session.CSRF, &session.CreatedAt, &session.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &session, err
}

func (s *Store) DeleteMerchantSession(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM merchant_sessions WHERE session_hash=?`, hash)
	return err
}

// Durable limits survive a restart. Only a hash of the peer address is retained.
func (s *Store) MerchantLoginAttempt(ctx context.Context, bucket string, now int64, limit int) (bool, error) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM merchant_login_attempts WHERE window_start<=?`, now-900); err != nil {
		return false, err
	}
	var attempts int
	err := s.db.QueryRowContext(ctx, `INSERT INTO merchant_login_attempts(bucket,window_start,attempts) VALUES(?,?,1) ON CONFLICT(bucket) DO UPDATE SET attempts=CASE WHEN window_start<=? THEN 1 ELSE attempts+1 END, window_start=CASE WHEN window_start<=? THEN excluded.window_start ELSE window_start END RETURNING attempts`, bucket, now, now-900, now-900).Scan(&attempts)
	if err != nil {
		return false, err
	}
	return attempts <= limit, nil
}
