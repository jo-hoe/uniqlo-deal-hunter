// Package sqlite implements store.Store on top of the pure-Go
// modernc.org/sqlite driver. Chosen so the container can stay CGO-free and
// cross-compile trivially.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// schema is applied on Open. It's an idempotent CREATE IF NOT EXISTS block
// so the same code can be run every startup without a real migration tool.
const schema = `
CREATE TABLE IF NOT EXISTS notified_deals (
    product_id  TEXT    NOT NULL,
    promo_cents INTEGER NOT NULL,
    name        TEXT    NOT NULL,
    rule_name   TEXT    NOT NULL,
    notified_at INTEGER NOT NULL,
    PRIMARY KEY (product_id, promo_cents)
);
CREATE INDEX IF NOT EXISTS idx_notified_at ON notified_deals(notified_at);
`

// Store persists dedup state to a SQLite database.
type Store struct {
	db *sql.DB
}

// Open connects to the sqlite file at path (creating it if missing) and
// applies the schema. Pass ":memory:" for in-memory tests.
func Open(path string) (*Store, error) {
	dsn := buildDSN(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	// modernc.org/sqlite does not enforce a max-conn default; keep it tiny.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// buildDSN turns a filesystem path into a modernc.org/sqlite DSN.
// Uses WAL for durability across the CronJob restarts, and foreign_keys
// on principle even though the schema has none today.
func buildDSN(path string) string {
	if path == ":memory:" {
		return ":memory:"
	}
	// clean the path to avoid double-slashes on Windows.
	return fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)",
		filepath.ToSlash(path))
}

// IsNew implements Store.
func (s *Store) IsNew(ctx context.Context, key deal.Key) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM notified_deals WHERE product_id = ? AND promo_cents = ?`,
		string(key.ProductID), key.PromoCents,
	).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return true, nil
	case err != nil:
		return false, fmt.Errorf("lookup dedup: %w", err)
	default:
		return false, nil
	}
}

// MarkSeen implements Store.
func (s *Store) MarkSeen(ctx context.Context, d deal.Candidate, ruleName string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notified_deals(product_id, promo_cents, name, rule_name, notified_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(product_id, promo_cents) DO UPDATE SET
			name = excluded.name,
			rule_name = excluded.rule_name,
			notified_at = excluded.notified_at
	`,
		string(d.ProductID), d.Key().PromoCents, d.Name, ruleName, at.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("mark seen: %w", err)
	}
	return nil
}

// Prune implements Store.
func (s *Store) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM notified_deals WHERE notified_at < ?`, olderThan.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("prune: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Close implements Store.
func (s *Store) Close() error { return s.db.Close() }
