package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

type MerchantPolicy struct {
	ID                 int64        `json:"id"`
	CreatedAt          int64        `json:"created_at"`
	Status             string       `json:"status"` // DRAFT, ACTIVE, ARCHIVED
	Source             string       `json:"source"` // seed, llm, heuristic, manual
	SourcePrompt       string       `json:"source_prompt"`
	Policy             PolicyConfig `json:"policy"`
	Adjustments        []string     `json:"adjustments"`
	HardFloorMarginPct float64      `json:"hard_floor_margin_pct"`
}

// GetActivePolicy returns the currently active merchant policy.
func (s *Store) GetActivePolicy(ctx context.Context) (*MerchantPolicy, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, created_at, status, source, source_prompt, policy_json, adjustments_json, hard_floor_margin_pct
		FROM merchant_policies WHERE status = 'ACTIVE' ORDER BY id DESC LIMIT 1
	`)

	var p MerchantPolicy
	var policyJSON, adjustmentsJSON string
	err := row.Scan(&p.ID, &p.CreatedAt, &p.Status, &p.Source, &p.SourcePrompt, &policyJSON, &adjustmentsJSON, &p.HardFloorMarginPct)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(policyJSON), &p.Policy); err != nil {
		return nil, fmt.Errorf("unmarshal policy: %w", err)
	}
	if err := json.Unmarshal([]byte(adjustmentsJSON), &p.Adjustments); err != nil {
		p.Adjustments = []string{}
	}

	return &p, nil
}

// CreatePolicyDraft creates a new policy draft with status DRAFT.
func (s *Store) CreatePolicyDraft(ctx context.Context, prompt string, source string, cfg PolicyConfig, adjustments []string, hardFloor float64) (*MerchantPolicy, error) {
	if err := ValidatePolicy(cfg, hardFloor); err != nil {
		return nil, err
	}
	policyJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	adjJSON, err := json.Marshal(adjustments)
	if err != nil {
		return nil, err
	}

	now := NowUnix()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO merchant_policies (created_at, status, source, source_prompt, policy_json, adjustments_json, hard_floor_margin_pct)
		VALUES (?, 'DRAFT', ?, ?, ?, ?, ?)
	`, now, source, prompt, string(policyJSON), string(adjJSON), hardFloor)
	if err != nil {
		return nil, fmt.Errorf("insert policy draft: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &MerchantPolicy{
		ID:                 id,
		CreatedAt:          now,
		Status:             "DRAFT",
		Source:             source,
		SourcePrompt:       prompt,
		Policy:             cfg,
		Adjustments:        adjustments,
		HardFloorMarginPct: hardFloor,
	}, nil
}

// ActivatePolicy transactionally archives all existing ACTIVE policies and activates the specified policy.
func (s *Store) ActivatePolicy(ctx context.Context, policyID int64) (*MerchantPolicy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Revalidate stored drafts too: no activation path bypasses merchant bounds.
	var raw string
	var hardFloor float64
	if err := tx.QueryRowContext(ctx, "SELECT policy_json,hard_floor_margin_pct FROM merchant_policies WHERE id=?", policyID).Scan(&raw, &hardFloor); err != nil {
		return nil, err
	}
	var cfg PolicyConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	if err := ValidatePolicy(cfg, hardFloor); err != nil {
		return nil, err
	}
	// Verify policy exists
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM merchant_policies WHERE id = ?", policyID).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrNotFound
	}

	// Archive any active
	if _, err := tx.ExecContext(ctx, "UPDATE merchant_policies SET status = 'ARCHIVED' WHERE status = 'ACTIVE'"); err != nil {
		return nil, fmt.Errorf("archive active policies: %w", err)
	}

	// Set target to ACTIVE
	if _, err := tx.ExecContext(ctx, "UPDATE merchant_policies SET status = 'ACTIVE' WHERE id = ?", policyID); err != nil {
		return nil, fmt.Errorf("activate policy %d: %w", policyID, err)
	}

	if _, err := tx.ExecContext(ctx, "UPDATE agreements SET status='EXPIRED',status_reason='POLICY_CHANGED',updated_at=? WHERE status='OFFERED'", NowUnix()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetActivePolicy(ctx)
}

// ListPolicyDrafts returns the latest 10 policy drafts.
func (s *Store) ListPolicyDrafts(ctx context.Context) ([]MerchantPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, status, source, source_prompt, policy_json, adjustments_json, hard_floor_margin_pct
		FROM merchant_policies WHERE status = 'DRAFT' ORDER BY id DESC LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MerchantPolicy
	for rows.Next() {
		var p MerchantPolicy
		var policyJSON, adjustmentsJSON string
		if err := rows.Scan(&p.ID, &p.CreatedAt, &p.Status, &p.Source, &p.SourcePrompt, &policyJSON, &adjustmentsJSON, &p.HardFloorMarginPct); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(policyJSON), &p.Policy)
		_ = json.Unmarshal([]byte(adjustmentsJSON), &p.Adjustments)
		list = append(list, p)
	}
	return list, rows.Err()
}

// ValidatePolicy is used for manual drafts and activation, independently of AI.
func ValidatePolicy(c PolicyConfig, hardFloor float64) error {
	if math.IsNaN(c.MinMarginPct) || math.IsInf(c.MinMarginPct, 0) || c.MinMarginPct < hardFloor || c.MinMarginPct > 0.60 || math.IsNaN(c.MaxDiscountPct) || c.MaxDiscountPct < 0 || c.MaxDiscountPct > 0.30 || math.IsNaN(c.BuyerPower) || c.BuyerPower < 0.20 || c.BuyerPower > 0.50 {
		return fmt.Errorf("policy outside authorized bounds")
	}
	return c.Economics.Validate()
}
