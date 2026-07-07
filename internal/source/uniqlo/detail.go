package uniqlo

import (
	"context"
	"fmt"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// ResolveSizes implements source.Source. Fetches the authoritative per-size
// stock state for a product across all its colors, then collapses to a
// unique-by-size-code list: "in stock in ANY color" wins.
func (c *Client) ResolveSizes(ctx context.Context, id deal.ProductID) ([]deal.Size, error) {
	pg, err := c.probePriceGroup(ctx, id)
	if err != nil {
		return nil, err
	}
	var resp l2sResponse
	if err := c.getJSON(ctx, c.detailURL(string(id), pg), &resp); err != nil {
		return nil, fmt.Errorf("fetch l2s for %s: %w", id, err)
	}
	if resp.Result == nil {
		return nil, nil
	}
	return collapseSizes(resp.Result.L2s), nil
}

// probePriceGroup fetches the single-product endpoint to learn the price
// group needed for the l2s URL. Uniqlo uses "00" for the vast majority of
// products but the API is the source of truth.
func (c *Client) probePriceGroup(ctx context.Context, id deal.ProductID) (string, error) {
	target := fmt.Sprintf("%s/%s/api/commerce/v5/%s/products?productIds=%s&httpFailure=true",
		c.cfg.BaseURL, c.cfg.Region, c.cfg.Language, id)
	var resp productsResponse
	if err := c.getJSON(ctx, target, &resp); err != nil {
		return "", fmt.Errorf("probe product %s: %w", id, err)
	}
	if resp.Result != nil && len(resp.Result.Items) > 0 && resp.Result.Items[0].PriceGroup != "" {
		return resp.Result.Items[0].PriceGroup, nil
	}
	return "00", nil
}

// collapseSizes reduces per-color-per-size rows to one deal.Size per size code.
// A size is InStock iff any color offers it in stock.
func collapseSizes(rows []l2) []deal.Size {
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
		if isStocked(row) {
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
// The API omits stockStatusCode for normally-stocked items; only an explicit
// "OUT_OF_STOCK" value means the item is unavailable.
func isStocked(row *l2) bool {
	return row.Sales && row.StockStatusCode != "OUT_OF_STOCK"
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
