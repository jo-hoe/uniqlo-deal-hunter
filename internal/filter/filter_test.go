package filter

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

func cand(name string, promo, base string, sizes ...deal.Size) deal.Candidate {
	return deal.Candidate{
		ProductID:  "E1",
		Name:       name,
		PromoPrice: mustDec(promo),
		BasePrice:  mustDec(base),
		Sizes:      sizes,
	}
}

func mustDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func inStock(label string) deal.Size  { return deal.Size{Label: label, InStock: true} }
func outStock(label string) deal.Size { return deal.Size{Label: label, InStock: false} }

func TestEvaluator_MatchesMaxPrice(t *testing.T) {
	rules := []config.Rule{{Name: "cheap", MaxPriceEUR: mustDec("5")}}
	e := New(rules)

	if got := e.Match(cand("x", "3", "10")); got == nil || got.Name != "cheap" {
		t.Errorf("expected match, got %v", got)
	}
	if got := e.Match(cand("x", "5.01", "10")); got != nil {
		t.Errorf("expected no match, got %v", got)
	}
}

func TestEvaluator_MatchesMinDiscount(t *testing.T) {
	rules := []config.Rule{{Name: "big-discount", MinDiscountPercent: 40}}
	e := New(rules)

	if got := e.Match(cand("x", "6", "10")); got == nil { // 40 pct off -> matches
		t.Errorf("expected match for 40 pct discount")
	}
	if got := e.Match(cand("x", "7", "10")); got != nil { // 30 pct off -> miss
		t.Errorf("expected no match for 30 pct discount, got %v", got)
	}
}

func TestEvaluator_MatchesSizes(t *testing.T) {
	rules := []config.Rule{{Name: "m-only", Sizes: []string{"M"}}}
	e := New(rules)

	if got := e.Match(cand("x", "5", "10", inStock("M"))); got == nil {
		t.Errorf("expected match")
	}
	if got := e.Match(cand("x", "5", "10", outStock("M"), inStock("L"))); got != nil {
		t.Errorf("expected no match: M out of stock, L not requested")
	}
	if got := e.Match(cand("x", "5", "10")); got != nil {
		t.Errorf("expected no match: no sizes")
	}
}

func TestEvaluator_MatchesName(t *testing.T) {
	rules := []config.Rule{
		config.NewRuleForTest("socks", "(?i)socks", nil, "", 0),
	}
	e := New(rules)

	if got := e.Match(cand("Cotton Socks", "3", "10")); got == nil {
		t.Errorf("expected match for name 'Cotton Socks'")
	}
	if got := e.Match(cand("Wool Coat", "3", "10")); got != nil {
		t.Errorf("expected no match for name 'Wool Coat', got %v", got)
	}
}

func TestEvaluator_ANDsAndORs(t *testing.T) {
	rules := []config.Rule{
		{Name: "cheap-socks", MaxPriceEUR: mustDec("5")},
		{Name: "any-big-discount", MinDiscountPercent: 50},
	}
	e := New(rules)

	// Matches only rule 1.
	if got := e.Match(cand("Cotton Socks", "3", "10")); got == nil || got.Name != "cheap-socks" {
		t.Errorf("wanted cheap-socks, got %v", got)
	}
	// Matches only rule 2 (rule 1's max price is 5, promo is 20 -> fails 1).
	if got := e.Match(cand("Coat", "20", "50")); got == nil || got.Name != "any-big-discount" {
		t.Errorf("wanted any-big-discount, got %v", got)
	}
	// Matches neither.
	if got := e.Match(cand("Coat", "40", "50")); got != nil {
		t.Errorf("wanted no match, got %v", got)
	}
}

func TestEvaluator_FirstMatchWins(t *testing.T) {
	rules := []config.Rule{
		{Name: "first"},
		{Name: "second"},
	}
	e := New(rules)
	if got := e.Match(cand("x", "5", "10")); got == nil || got.Name != "first" {
		t.Errorf("wanted first, got %v", got)
	}
}
