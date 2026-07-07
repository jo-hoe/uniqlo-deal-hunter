package uniqlo

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// mapItem converts one API item into a domain Candidate. Returns an error
// if the item lacks the fields the pipeline needs.
func mapItem(it *item, region, language, baseURL string) (deal.Candidate, error) {
	base, err := pickPrice(it.Prices.Base)
	if err != nil {
		return deal.Candidate{}, fmt.Errorf("%s: base price: %w", it.ProductID, err)
	}
	promo, err := pickPrice(it.Prices.Promo)
	if err != nil {
		// Not a real deal without a promo price.
		return deal.Candidate{}, fmt.Errorf("%s: promo price: %w", it.ProductID, err)
	}
	c := deal.Candidate{
		ProductID:      deal.ProductID(it.ProductID),
		Name:           it.Name,
		URL:            productURL(baseURL, region, language, it.ProductID, it.PriceGroup, it.Colors),
		BasePrice:      base,
		PromoPrice:     promo,
		Lowest30dPrice: lowestPrice(it.Prices.LowestPriceDetails, base),
		Sizes:          mapListingSizes(it.Sizes),
		// The listing endpoint is the only place the correct priceGroup lives
		// for the discounted colour of this item — the probe endpoint returns
		// the base variant's priceGroup, which is often wrong. Carry it as an
		// opaque hint so ResolveSizes can hit the right l2s URL.
		ProviderRef: it.PriceGroup,
	}
	if err := c.Validate(); err != nil {
		return deal.Candidate{}, err
	}
	return c, nil
}

// pickPrice safely converts an optional *priceValue to a decimal.
func pickPrice(p *priceValue) (deal.EUR, error) {
	if p == nil {
		return decimal.Zero, fmt.Errorf("price is nil")
	}
	return decimal.NewFromFloat(p.Value), nil
}

// lowestPrice returns the 30-day lowest price if the API supplied one that
// may be legally displayed; otherwise falls back to the base price.
func lowestPrice(l *lowestPriceDetails, fallback deal.EUR) deal.EUR {
	if l == nil || !l.CanDisplayLowestPrice {
		return fallback
	}
	return decimal.NewFromFloat(l.LowestPrice)
}

// mapListingSizes converts listing-level sizes to domain sizes. Availability
// is unknown at this stage — the listing endpoint doesn't provide it — so
// InStock is left false. The runner overwrites this with authoritative data.
func mapListingSizes(sizes []size) []deal.Size {
	out := make([]deal.Size, 0, len(sizes))
	for _, s := range sizes {
		out = append(out, deal.Size{
			Code:    s.Code,
			Label:   displayOrName(s),
			InStock: false,
		})
	}
	return out
}

// productURL builds the canonical PDP URL for a product+priceGroup+color.
func productURL(baseURL, region, language, productID, priceGroup string, colors []color) string {
	pg := priceGroup
	if pg == "" {
		pg = "00"
	}
	u := fmt.Sprintf("%s/%s/%s/products/%s/%s",
		strings.TrimRight(baseURL, "/"), region, language, productID, pg)
	if len(colors) > 0 && colors[0].DisplayCode != "" {
		u += "?colorDisplayCode=" + colors[0].DisplayCode
	}
	return u
}
