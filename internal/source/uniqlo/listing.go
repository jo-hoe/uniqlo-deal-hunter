package uniqlo

import (
	"context"
	"fmt"
	"time"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

// FetchDeals implements source.Source. Paginates through the products
// endpoint until every item has been retrieved.
func (c *Client) FetchDeals(ctx context.Context) ([]deal.Candidate, error) {
	var (
		out    []deal.Candidate
		offset int
	)
	for {
		page, err := c.fetchPage(ctx, offset)
		if err != nil {
			return nil, fmt.Errorf("fetch offset=%d: %w", offset, err)
		}
		if page.Result == nil || len(page.Result.Items) == 0 {
			break
		}
		for i := range page.Result.Items {
			cand, mapErr := mapItem(&page.Result.Items[i], c.cfg.Region, c.cfg.Language, c.cfg.BaseURL)
			if mapErr != nil {
				// A single malformed item must not fail the whole run — but
				// it must be visible in Grafana. Log with the raw product id
				// (if any) so operators can drill into upstream data drift.
				c.logger.Warn("skip malformed listing item",
					"productId", page.Result.Items[i].ProductID,
					"err", mapErr)
				continue
			}
			cand.FetchedAt = time.Now().UTC()
			out = append(out, cand)
		}
		offset += page.Result.Pagination.Count
		if offset >= page.Result.Pagination.Total || page.Result.Pagination.Count == 0 {
			break
		}
	}
	return out, nil
}

// fetchPage retrieves a single page of the listing.
func (c *Client) fetchPage(ctx context.Context, offset int) (*productsResponse, error) {
	var resp productsResponse
	if err := c.getJSON(ctx, c.listingURL(offset), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
