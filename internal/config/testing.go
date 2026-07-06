package config

import "regexp"

// NewRuleForTest constructs a Rule with a pre-compiled regex. Exported for
// use by external test packages that need to exercise regex-based rules
// without going through Load().
func NewRuleForTest(name, pattern string, sizes []string, maxPriceEUR string, minDiscountPercent int) Rule {
	r := Rule{
		Name:               name,
		NamePattern:        pattern,
		Sizes:              sizes,
		MinDiscountPercent: minDiscountPercent,
	}
	if pattern != "" {
		r.compiled = regexp.MustCompile(pattern)
	}
	if maxPriceEUR != "" {
		r.MaxPriceEUR = mustDecForTest(maxPriceEUR)
	}
	return r
}
