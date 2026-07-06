// Package source defines the abstract contract for a deal source. A source
// is responsible for two things: enumerating discounted candidate items,
// and — for a given candidate — returning authoritative per-size stock.
package source

import (
	"context"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// Source is the abstract contract implemented by every provider.
//
// Implementations MUST be safe for use by a single goroutine per Source
// instance. They MUST honour context cancellation and MUST NOT retain the
// context past the returning call.
type Source interface {
	// FetchDeals enumerates every currently-discounted item on the source,
	// paginating internally. The listing's Sizes field is best-effort and
	// callers should re-check via ResolveSizes before trusting it.
	FetchDeals(ctx context.Context) ([]deal.Candidate, error)

	// ResolveSizes returns the authoritative per-size stock state for a
	// specific product.
	ResolveSizes(ctx context.Context, id deal.ProductID) ([]deal.Size, error)
}
