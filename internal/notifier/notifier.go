// Package notifier defines the abstract contract for delivering deal digests.
package notifier

import (
	"context"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// MatchedDeal pairs a deal with the rule that admitted it, so notifiers can
// surface provenance (which rule made this alert fire).
type MatchedDeal struct {
	Deal     deal.Candidate
	RuleName string
}

// Notifier sends a digest of matched deals.
//
// A single Notify call MUST be atomic from the caller's perspective: either
// every recipient receives the digest and the call returns nil, or it
// returns non-nil and none of the deals in it should be recorded as seen.
type Notifier interface {
	Notify(ctx context.Context, deals []MatchedDeal) error
}
