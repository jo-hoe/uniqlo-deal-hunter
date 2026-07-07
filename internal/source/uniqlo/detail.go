package uniqlo

import (
	"context"
	"fmt"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// ResolveSizes implements source.Source. Fetches the authoritative per-size
// stock state for a product across all its colors, then collapses to a
// unique-by-size-code list: "in stock in ANY color" wins.
//
// The correct priceGroup is passed in via Candidate.ProviderRef — populated
// by FetchDeals from the listing endpoint. A stale probe endpoint used to be
// consulted here but returns the base variant's priceGroup, which for many
// products differs from the discounted variant and yields l2s data for the
// wrong item entirely.
func (c *Client) ResolveSizes(ctx context.Context, cand deal.Candidate) ([]deal.Size, error) {
	pg := cand.ProviderRef
	if pg == "" {
		// Older callers or non-Uniqlo tests may leave the ref blank. "00" is
		// the safest historical default; ResolveSizes may return an empty
		// result if the guess is wrong, which the runner tolerates.
		pg = "00"
	}
	var resp l2sResponse
	if err := c.getJSON(ctx, c.detailURL(string(cand.ProductID), pg), &resp); err != nil {
		return nil, fmt.Errorf("fetch l2s for %s: %w", cand.ProductID, err)
	}
	if resp.Result == nil {
		return nil, nil
	}
	return collapseSizes(resp.Result.L2s, resp.Result.Stocks), nil
}

// collapseSizes reduces per-color-per-size rows to one deal.Size per size code.
// A size is InStock iff any color offers it in stock according to the stocks
// map keyed by l2Id.
func collapseSizes(rows []l2, stocks map[string]stockRow) []deal.Size {
	seen := make(map[string]*deal.Size, len(rows))
	order := make([]string, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		if row.Size.Code == "" {
			continue
		}
		s, ok := seen[row.Size.Code]
		if !ok {
			s = &deal.Size{Code: row.Size.Code, Label: displayOrName(row.Size)}
			seen[row.Size.Code] = s
			order = append(order, row.Size.Code)
		}
		if isStocked(row, stocks) {
			s.InStock = true
		}
	}
	out := make([]deal.Size, 0, len(order))
	for _, code := range order {
		out = append(out, *seen[code])
	}
	return out
}

// isStocked reports whether a single l2 row is currently purchasable.
//
// Uniqlo's real API returns stock info in a top-level `stocks` map keyed by
// l2Id, NOT on the l2 row (the row's `stockStatusCode` is typically empty).
// A size chip on the PDP is enabled iff:
//   - the row's `sales` flag is true,
//   - the matching stock entry says `IN_STOCK`,
//   - quantity is positive, and
//   - the size chip is not explicitly disabled.
//
// Any of these missing means the frontend renders the size as unavailable,
// so the notifier must treat it the same way.
func isStocked(row *l2, stocks map[string]stockRow) bool {
	if !row.Sales {
		return false
	}
	s, ok := stocks[row.L2ID]
	if !ok {
		// No stock row for this l2Id means the product page has nothing to
		// render for it — treat as out of stock, not "unknown".
		return false
	}
	return s.StatusCode == "IN_STOCK" && s.Quantity > 0 && !s.DisableSizeChip
}

// displayOrName picks the best user-facing size label available.
// The API's `display` field is metadata (an object), not a label, so we
// prefer the human-readable `name` (e.g. "42-46(27-29cm)"). displayCode
// (e.g. "027") is a stable fallback if name is unset.
func displayOrName(s size) string {
	if s.Name != "" {
		return s.Name
	}
	if s.DisplayCode != "" {
		return s.DisplayCode
	}
	return s.Code
}
