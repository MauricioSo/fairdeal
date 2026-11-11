package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nashpay/nashpay/internal/economics"
	"os"
	"strings"
)

var ErrEconomicChange = errors.New("economic conditions changed; request a new offer")

// Back up an existing pre-engine database before additive schema changes.
func backupBeforeEconomics(ctx context.Context, db *sql.DB, path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	var old int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('products','agreement_economics')").Scan(&old); err != nil {
		return err
	}
	if old != 1 {
		return nil
	}
	backup := path + ".pre-economics.backup.db"
	if _, err := os.Stat(backup); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	_, err := db.ExecContext(ctx, "VACUUM INTO ?", backup)
	return err
}

func (s *Store) migrateEconomics(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var applied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=3").Scan(&applied); err != nil {
		return err
	}
	if applied == 0 {
		if _, err = tx.ExecContext(ctx, "UPDATE agreements SET status='EXPIRED',status_reason='ECONOMIC_ENGINE_UPGRADE' WHERE status='OFFERED' AND id NOT IN (SELECT agreement_id FROM agreement_economics)"); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,applied_at)VALUES(3,?)", NowUnix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type EconomicSnapshot struct {
	Version    int                  `json:"version"`
	PolicyID   int64                `json:"policy_id"`
	Revision   string               `json:"revision"`
	Candidate  economics.Candidate  `json:"candidate"`
	Evaluation economics.Evaluation `json:"evaluation"`
}

type economicReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Hash all authoritative economic inputs, excluding available stock (validated
// separately by the atomic reservation). The same function works inside a tx.
func economicRevision(ctx context.Context, q economicReader) (string, error) {
	var id int64
	var policy, market string
	if err := q.QueryRowContext(ctx, "SELECT id,policy_json FROM merchant_policies WHERE status='ACTIVE' ORDER BY id DESC LIMIT 1").Scan(&id, &policy); err != nil {
		return "", err
	}
	if err := q.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='market'").Scan(&market); err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%d\n%s\n%s\n", id, policy, market)
	rows, err := q.QueryContext(ctx, "SELECT sku,list_price_cents,cost_cents,retail_value_cents,holding_cost_cents,is_deadstock FROM products ORDER BY sku")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var sku string
		var list, cost, value, holding, dead int64
		if err = rows.Scan(&sku, &list, &cost, &value, &holding, &dead); err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s:%d:%d:%d:%d:%d\n", sku, list, cost, value, holding, dead)
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *Store) EconomicRevision(ctx context.Context) (string, error) {
	return economicRevision(ctx, s.db)
}
func (s *Store) GetEconomics(ctx context.Context, id string) (*EconomicSnapshot, error) {
	var data string
	if err := s.db.QueryRowContext(ctx, "SELECT snapshot_json FROM agreement_economics WHERE agreement_id=?", id).Scan(&data); err != nil {
		return nil, err
	}
	var snapshot EconomicSnapshot
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}
func (s *Store) ReserveEconomically(ctx context.Context, id string) error {
	reason := "CHECKOUT_INITIATED"
	return s.transitionWithInventory(ctx, id, StatusOffered, StatusOrderCreated, &reason, nil, nil, true)
}

func checkEconomicSnapshot(ctx context.Context, tx *sql.Tx, id string) error {
	var data string
	if err := tx.QueryRowContext(ctx, "SELECT snapshot_json FROM agreement_economics WHERE agreement_id=?", id).Scan(&data); err != nil {
		return ErrEconomicChange
	}
	var snapshot EconomicSnapshot
	if json.Unmarshal([]byte(data), &snapshot) != nil || snapshot.Version != 1 {
		return ErrEconomicChange
	}
	revision, err := economicRevision(ctx, tx)
	if err != nil {
		return err
	}
	if revision != snapshot.Revision || !economics.Evaluate(snapshot.Candidate).Approved {
		return ErrEconomicChange
	}
	var price, policyID, expires int64
	var sku, shipping string
	var bundle *string
	if err = tx.QueryRowContext(ctx, "SELECT price_cents,policy_id,expires_at,sku,shipping_tier,bundle_sku FROM agreements WHERE id=?", id).Scan(&price, &policyID, &expires, &sku, &shipping, &bundle); err != nil {
		return err
	}
	c := snapshot.Candidate
	if expires < NowUnix() || price != int64(c.Gross) || policyID != snapshot.PolicyID || shipping != c.Shipping || len(c.Items) < 1 || c.Items[0].SKU != sku || (bundle == nil && len(c.Items) != 1) || (bundle != nil && (len(c.Items) != 2 || c.Items[1].SKU != *bundle)) {
		return ErrEconomicChange
	}
	return nil
}
