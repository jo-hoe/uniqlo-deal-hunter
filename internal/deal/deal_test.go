package deal

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestCandidate_DiscountPercent(t *testing.T) {
	tests := []struct {
		name       string
		base, promo string
		want       int
	}{
		{"half off", "10.00", "5.00", 50},
		{"third off", "9.00", "6.00", 33},
		{"no discount", "10.00", "10.00", 0},
		{"promo higher", "10.00", "12.00", 0},
		{"zero base", "0.00", "5.00", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Candidate{BasePrice: dec(tt.base), PromoPrice: dec(tt.promo)}
			if got := c.DiscountPercent(); got != tt.want {
				t.Errorf("DiscountPercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCandidate_Key(t *testing.T) {
	c := Candidate{ProductID: "E1", PromoPrice: dec("4.99")}
	got := c.Key()
	if got.ProductID != "E1" || got.PromoCents != 499 {
		t.Errorf("Key() = %+v, want {E1 499}", got)
	}
}

func TestCandidate_InStockSizeLabels(t *testing.T) {
	c := Candidate{Sizes: []Size{
		{Label: "M", InStock: true},
		{Label: "L", InStock: false},
		{Label: "XL", InStock: true},
	}}
	got := c.InStockSizeLabels()
	if len(got) != 2 || got[0] != "M" || got[1] != "XL" {
		t.Errorf("InStockSizeLabels() = %v", got)
	}
}

func TestCandidate_Validate(t *testing.T) {
	valid := Candidate{
		ProductID: "E1", Name: "Sock", URL: "http://x",
		BasePrice: dec("5"), PromoPrice: dec("3"),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid candidate returned err: %v", err)
	}
	invalid := []Candidate{
		{Name: "n", URL: "u"},
		{ProductID: "p", URL: "u"},
		{ProductID: "p", Name: "n"},
		{ProductID: "p", Name: "n", URL: "u", PromoPrice: dec("-1"), BasePrice: dec("5")},
	}
	for i, c := range invalid {
		if err := c.Validate(); !errors.Is(err, ErrInvalidCandidate) {
			t.Errorf("case %d: want ErrInvalidCandidate, got %v", i, err)
		}
	}
}
