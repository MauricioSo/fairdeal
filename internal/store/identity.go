package store

import (
	"context"
	"errors"
	"os"
	"strings"
)

func (s *Store) MerchantID() string { return s.merchantID }
func (s *Store) BindMerchant(ctx context.Context, id string) error {
	var configured int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM settings WHERE key='merchant-identity'").Scan(&configured); err != nil {
		return err
	}
	if configured == 0 && s.dbPath != "" && s.dbPath != ":memory:" && !strings.HasPrefix(s.dbPath, "file:") {
		backup := s.dbPath + ".pre-marketplace.backup.db"
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", backup); err != nil {
				return err
			}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO settings(key,value_json)VALUES('merchant-identity',?)", id)
	if err != nil {
		return err
	}
	var stored string
	if err = tx.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='merchant-identity'").Scan(&stored); err != nil {
		return err
	}
	if stored != id {
		return errors.New("merchant database identity mismatch")
	}
	n, _ := result.RowsAffected()
	if n == 1 {
		if _, err = tx.ExecContext(ctx, "UPDATE agreements SET buyer_budget_cents=0,buyer_surplus_cents=0"); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE agreements SET status='EXPIRED',status_reason='TOKEN_VERSION_UPGRADE' WHERE status='OFFERED'"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations(version,applied_at)VALUES(2,?)", NowUnix()); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		s.merchantID = id
	}
	return err
}
