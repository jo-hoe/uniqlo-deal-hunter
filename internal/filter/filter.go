// Package filter evaluates a Deal against the user-supplied rule set.
// Semantics: a deal matches iff any rule matches; a rule matches iff every
// non-empty condition inside it matches. Rules are evaluated in declared
// order; the first matching rule wins and is returned for provenance.
package filter

import (
	"github.com/shopspring/decimal"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// Evaluator matches deals against a rule set.
type Evaluator interface {
	// Match returns the first matching rule, or nil if none match.
	Match(d deal.Candidate) *config.Rule
}

// ruleEvaluator is the default Evaluator, built once from a []config.Rule.
type ruleEvaluator struct {
	rules []config.Rule
}

// New builds an Evaluator from a rule set. The rules must have been
// validated (and their regex compiled) by the config package.
func New(rules []config.Rule) Evaluator {
	return &ruleEvaluator{rules: rules}
}

// Match implements Evaluator.
func (e *ruleEvaluator) Match(d deal.Candidate) *config.Rule {
	for i := range e.rules {
		r := &e.rules[i]
		if ruleMatches(r, d) {
			return r
		}
	}
	return nil
}

// ruleMatches is the AND-composition of a rule's individual conditions.
// Each condition is short-circuiting to keep this cheap on hot paths.
func ruleMatches(r *config.Rule, d deal.Candidate) bool {
	return matchesName(r, d) &&
		matchesSizes(r, d) &&
		matchesMaxPrice(r, d) &&
		matchesMinDiscount(r, d)
}

// matchesName is true when the rule sets no NamePattern or when the pattern
// matches the deal name.
func matchesName(r *config.Rule, d deal.Candidate) bool {
	re := r.NameRegex()
	if re == nil {
		return true
	}
	return re.MatchString(d.Name)
}

// matchesSizes is true when the rule sets no Sizes or when at least one of
// the rule's sizes is in stock on the deal.
func matchesSizes(r *config.Rule, d deal.Candidate) bool {
	if len(r.Sizes) == 0 {
		return true
	}
	wanted := make(map[string]struct{}, len(r.Sizes))
	for _, s := range r.Sizes {
		wanted[s] = struct{}{}
	}
	for _, s := range d.Sizes {
		if !s.InStock {
			continue
		}
		if _, ok := wanted[s.Label]; ok {
			return true
		}
		if _, ok := wanted[s.Code]; ok {
			return true
		}
	}
	return false
}

// matchesMaxPrice is true when the rule sets no MaxPriceEUR (zero) or the
// promo price is at or below it.
func matchesMaxPrice(r *config.Rule, d deal.Candidate) bool {
	if r.MaxPriceEUR.IsZero() {
		return true
	}
	return d.PromoPrice.LessThanOrEqual(r.MaxPriceEUR)
}

// matchesMinDiscount is true when the rule sets no MinDiscountPercent or the
// deal's computed discount is at or above it.
func matchesMinDiscount(r *config.Rule, d deal.Candidate) bool {
	if r.MinDiscountPercent == 0 {
		return true
	}
	return d.DiscountPercent() >= r.MinDiscountPercent
}

// Ensure the decimal package is retained even if we later remove uses in
// helpers — decimal comparisons are the whole point of using it here.
var _ = decimal.Zero
