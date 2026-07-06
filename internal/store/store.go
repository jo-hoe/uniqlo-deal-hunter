// Package store defines the persistence contract for notification dedup state.
package store

import (
	"context"
	"time"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// Store persists which (product, promo-price) pairs have already been
// announced, so the runner can avoid double-notifying the user.
//
// Implementations MUST be safe for the CronJob's single-goroutine use.
// Callers are responsible for calling Close on the returned Store.
type Store interface {
	// IsNew reports whether the given key has NOT been marked seen yet.
	IsNew(ctx context.Context, key deal.Key) (bool, error)

	// MarkSeen records that a deal was successfully notified about.
	MarkSeen(ctx context.Context, d deal.Candidate, ruleName string, at time.Time) error

	// Prune deletes rows older than the given cutoff. Implementations may
	// return the number of rows deleted; callers may ignore it.
	Prune(ctx context.Context, olderThan time.Time) (int64, error)

	// Close releases any resources held by the store.
	Close() error
}
