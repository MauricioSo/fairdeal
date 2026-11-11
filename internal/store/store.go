package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

var (
	ErrNotFound          = errors.New("record not found")
	ErrInvalidTransition = errors.New("invalid agreement status transition")
	ErrAlreadyUsed       = errors.New("agreement token already redeemed or not in offered state")
	ErrExpired           = errors.New("agreement has expired")
)

type Store struct {
	db *sql.DB
}

// DB returns the underlying sql.DB instance for testing or advanced transactions.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Open opens a SQLite database at dbPath, applies PRAGMAs, runs migrations and seeds default data.
func Open(ctx context.Context, dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Optimize connection settings for SQLite WAL
	db.SetMaxOpenConns(1) // Single writer ensures zero lock contention while WAL handles concurrency
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("execute pragma %q: %w", p, err)
		}
	}

	// Apply schema
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	s := &Store{db: db}

	// Run idempotent seed
	if err := s.seedIfEmpty(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seed initial data: %w", err)
	}

	return s, nil
}

// Ping verifies database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// NowUnix returns current Unix timestamp in seconds.
func NowUnix() int64 {
	return time.Now().Unix()
}
