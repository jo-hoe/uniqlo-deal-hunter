// Package deal holds the core domain types of the deal hunter.
//
// These types are the language spoken between the source, filter, store, and
// notifier packages. They deliberately use strong, non-fuzzy types (e.g.
// ProductID is a distinct string, EUR is decimal.Decimal, never float64) so
// that misuse is caught at compile time.
package deal

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ProductID is the stable Uniqlo product identifier (e.g. "E471954-000").
type ProductID string

// String satisfies fmt.Stringer.
func (p ProductID) String() string { return string(p) }

// EUR represents a price in euros. Uses shopspring/decimal to avoid the
// floating-point rounding hazards that plague monetary arithmetic.
type EUR = decimal.Decimal

// Size describes a product size and its stock state.
type Size struct {
	// Code is the internal Uniqlo size code (e.g. "MSC027").
	Code string
	// Label is the human-readable size (e.g. "M", "L", "27-29").
	Label string
	// InStock is true iff the item is currently purchasable in this size.
	InStock bool
}

// Candidate is a discounted item as returned by a Source's listing endpoint.
// It carries everything needed for filter evaluation except authoritative
// per-size stock; that is filled in later via Source.ResolveSizes.
type Candidate struct {
	ProductID      ProductID
	Name           string
	URL            string
	BasePrice      EUR
	PromoPrice     EUR
	Lowest30dPrice EUR
	// Sizes is the listing's best-guess size availability. It is deliberately
	// not trusted for the size filter — the runner overwrites it with the
	// authoritative per-item detail.
	Sizes     []Size
	FetchedAt time.Time
}

// Deal is a Candidate that has been enriched with authoritative size data.
type Deal struct {
	Candidate
}

// DiscountPercent returns the promo discount as an integer percent of the
// base price, rounded down. Returns 0 if the base price is zero or if the
// promo is not lower than base.
func (c Candidate) DiscountPercent() int {
	if c.BasePrice.IsZero() || !c.PromoPrice.LessThan(c.BasePrice) {
		return 0
	}
	off := c.BasePrice.Sub(c.PromoPrice)
	pct := off.Mul(decimal.NewFromInt(100)).Div(c.BasePrice)
	return int(pct.IntPart())
}

// Key is the notification-dedup identity: same product at the same promo
// price is considered "already-seen".
type Key struct {
	ProductID  ProductID
	PromoCents int64
}

// Key returns the dedup identity for a Candidate.
func (c Candidate) Key() Key {
	return Key{
		ProductID:  c.ProductID,
		PromoCents: c.PromoPrice.Mul(decimal.NewFromInt(100)).Round(0).IntPart(),
	}
}

// InStockSizeLabels returns the human-readable labels of sizes currently in
// stock. Handy for notifier templates and logging.
func (c Candidate) InStockSizeLabels() []string {
	out := make([]string, 0, len(c.Sizes))
	for _, s := range c.Sizes {
		if s.InStock {
			out = append(out, s.Label)
		}
	}
	return out
}

// ErrInvalidCandidate is returned by Validate when a Candidate has missing
// or contradictory fields.
var ErrInvalidCandidate = errors.New("invalid candidate")

// Validate reports whether a Candidate carries the minimum information the
// pipeline needs. Called by mappers before entering the pipeline.
func (c Candidate) Validate() error {
	switch {
	case strings.TrimSpace(string(c.ProductID)) == "":
		return fmt.Errorf("%w: missing product id", ErrInvalidCandidate)
	case strings.TrimSpace(c.Name) == "":
		return fmt.Errorf("%w: missing name for %s", ErrInvalidCandidate, c.ProductID)
	case strings.TrimSpace(c.URL) == "":
		return fmt.Errorf("%w: missing url for %s", ErrInvalidCandidate, c.ProductID)
	case c.PromoPrice.IsNegative() || c.BasePrice.IsNegative():
		return fmt.Errorf("%w: negative price for %s", ErrInvalidCandidate, c.ProductID)
	}
	return nil
}
