package uniqlo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

func testConfig(base string) config.Source {
	return config.Source{
		Kind:              config.SourceKindUniqlo,
		BaseURL:           base,
		Region:            "de",
		Language:          "en",
		Segment:           config.SegmentMen,
		ClientID:          "uq.de.web-spa",
		ClientVersion:     "3.2509.1",
		RequestsPerSecond: 100, // don't slow the test suite
		Timeout:           2 * time.Second,
		MaxRetries:        2,
		UserAgent:         "test-agent",
	}
}

// newTestClient builds a Client for tests and disables the live
// user-agent resolver so tests never touch chromiumdash.appspot.com.
// The compiled-in fallback (cfg.UserAgent) is used instead.
func newTestClient(t *testing.T, cfg config.Source, logger *slog.Logger) *Client {
	t.Helper()
	c := NewClient(cfg, logger)
	c.userAgent.resolver = nil
	return c
}

// versionHandler responds to the storefront-root GET that the client makes
// once per instance to discover x-fr-client-version. Composed with the
// endpoint-specific handler used by each test.
func versionHandler(t *testing.T, next http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/de/en/" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><script>window.__BUILD_VERSION__ = "9.9.9";</script></html>`)
			return
		}
		next(w, r)
	}
}
func fakeItem(id, name string, base, promo, lowest float64) map[string]any {
	return map[string]any{
		"productId":  id,
		"name":       name,
		"priceGroup": "00",
		"prices": map[string]any{
			"base":  map[string]any{"value": base, "currency": map[string]string{"code": "EUR"}},
			"promo": map[string]any{"value": promo, "currency": map[string]string{"code": "EUR"}},
			"lowestPriceDetails": map[string]any{
				"canDisplayLowestPrice": true,
				"lowestPeriod":          30,
				"lowestPrice":           lowest,
			},
		},
		"sizes": []map[string]any{
			{"code": "MSC027", "name": "Medium"},
		},
		"colors": []map[string]any{
			{"code": "COL01", "displayCode": "01", "name": "Black"},
		},
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatal(err)
	}
}

func TestFetchDeals_SinglePage(t *testing.T) {
	var gotClientID string
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, r *http.Request) {
		gotClientID = r.Header.Get("x-fr-clientid")
		writeJSON(t, w, map[string]any{
			"status": "ok",
			"result": map[string]any{
				"items": []any{
					fakeItem("E1", "Socks", 10, 5, 4.9),
					fakeItem("E2", "Hat", 20, 10, 9.9),
				},
				"pagination": map[string]int{"total": 2, "offset": 0, "count": 2},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	deals, err := c.FetchDeals(context.Background())
	if err != nil {
		t.Fatalf("FetchDeals: %v", err)
	}
	if len(deals) != 2 {
		t.Fatalf("want 2 deals, got %d", len(deals))
	}
	if deals[0].ProductID != "E1" || deals[0].Name != "Socks" {
		t.Errorf("unexpected first deal: %+v", deals[0])
	}
	if gotClientID != "uq.de.web-spa" {
		t.Errorf("client-id header not forwarded: %q", gotClientID)
	}
	if deals[0].DiscountPercent() != 50 {
		t.Errorf("discount pct = %d, want 50", deals[0].DiscountPercent())
	}
}

func TestFetchDeals_Pagination(t *testing.T) {
	items := make([]map[string]any, 50)
	for i := range items {
		items[i] = fakeItem(fmt.Sprintf("E%03d", i), "Item", 10, 5, 4)
	}
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, r *http.Request) {
		off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		end := min(off+pageSize, len(items))
		writeJSON(t, w, map[string]any{
			"status": "ok",
			"result": map[string]any{
				"items":      items[off:end],
				"pagination": map[string]int{"total": len(items), "offset": off, "count": end - off},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	deals, err := c.FetchDeals(context.Background())
	if err != nil {
		t.Fatalf("FetchDeals: %v", err)
	}
	if len(deals) != 50 {
		t.Fatalf("expected 50 deals, got %d", len(deals))
	}
}

func TestFetchDeals_RetryOn5xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		writeJSON(t, w, map[string]any{
			"status": "ok",
			"result": map[string]any{
				"items":      []any{fakeItem("E1", "Socks", 10, 5, 5)},
				"pagination": map[string]int{"total": 1, "offset": 0, "count": 1},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	deals, err := c.FetchDeals(context.Background())
	if err != nil {
		t.Fatalf("FetchDeals: %v", err)
	}
	if len(deals) != 1 || calls != 2 {
		t.Errorf("want 1 deal after 2 calls, got %d deals in %d calls", len(deals), calls)
	}
}

func TestFetchDeals_FailsAfterRetries(t *testing.T) {
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	_, err := c.FetchDeals(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error, got %v", err)
	}
}

func TestResolveSizes_CollapsesColorsAndDetectsStock(t *testing.T) {
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/products/E1/price-groups/") {
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"result": map[string]any{
					"l2s": []map[string]any{
						{"l2Id": "1", "size": map[string]any{"code": "M", "name": "Medium"},
							"color": map[string]string{"code": "R"}, "sales": true},
						{"l2Id": "2", "size": map[string]any{"code": "M", "name": "Medium"},
							"color": map[string]string{"code": "B"}, "sales": true},
						{"l2Id": "3", "size": map[string]any{"code": "L", "name": "Large"},
							"color": map[string]string{"code": "B"}, "sales": true},
					},
					"stocks": map[string]any{
						"1": map[string]any{"statusCode": "IN_STOCK", "quantity": 5},
						"2": map[string]any{"statusCode": "OUT_OF_STOCK", "quantity": 0},
						"3": map[string]any{"statusCode": "OUT_OF_STOCK", "quantity": 0},
					},
				},
			})
			return
		}
		// Any other endpoint (should not be hit — ResolveSizes no longer probes).
		http.Error(w, "unexpected: "+r.URL.Path, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	sizes, err := c.ResolveSizes(context.Background(), deal.Candidate{ProductID: "E1", ProviderRef: "00"})
	if err != nil {
		t.Fatalf("ResolveSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("want 2 unique sizes, got %d", len(sizes))
	}
	// Size M is in stock in color R even though out in B.
	var m *deal.Size
	for i := range sizes {
		if sizes[i].Code == "M" {
			m = &sizes[i]
		}
	}
	if m == nil || !m.InStock {
		t.Errorf("expected M in stock, got %+v", sizes)
	}
}

func TestResolveSizes_OutOfStockWhenStockRowMissing(t *testing.T) {
	// The real Uniqlo API leaves the row-level stockStatusCode empty even
	// for out-of-stock items — authoritative stock is only in the top-level
	// stocks map. A row without a matching entry must be treated as
	// unavailable (that's how the frontend renders it).
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/products/E1/price-groups/") {
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"result": map[string]any{
					"l2s": []map[string]any{
						{"l2Id": "1", "size": map[string]any{"code": "M", "name": "Medium"},
							"color": map[string]string{"code": "R"}, "sales": true},
					},
					// stocks: entry deliberately absent for l2Id "1".
					"stocks": map[string]any{},
				},
			})
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	sizes, err := c.ResolveSizes(context.Background(), deal.Candidate{ProductID: "E1", ProviderRef: "00"})
	if err != nil {
		t.Fatalf("ResolveSizes: %v", err)
	}
	if len(sizes) != 1 || sizes[0].InStock {
		t.Errorf("expected M reported OUT of stock when stocks map has no entry, got %+v", sizes)
	}
}

func TestResolveSizes_HonoursDisableSizeChip(t *testing.T) {
	// A stock row with IN_STOCK + positive quantity but disableSizeChip=true
	// must be treated as unavailable (the PDP would grey out the chip).
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/products/E1/price-groups/") {
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"result": map[string]any{
					"l2s": []map[string]any{
						{"l2Id": "1", "size": map[string]any{"code": "M", "name": "Medium"},
							"color": map[string]string{"code": "R"}, "sales": true},
					},
					"stocks": map[string]any{
						"1": map[string]any{"statusCode": "IN_STOCK", "quantity": 5, "disableSizeChip": true},
					},
				},
			})
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	sizes, err := c.ResolveSizes(context.Background(), deal.Candidate{ProductID: "E1", ProviderRef: "00"})
	if err != nil {
		t.Fatalf("ResolveSizes: %v", err)
	}
	if len(sizes) != 1 || sizes[0].InStock {
		t.Errorf("expected M reported OUT of stock when size chip disabled, got %+v", sizes)
	}
}

func TestResolveSizes_UsesCandidatePriceGroup(t *testing.T) {
	// The listing's priceGroup is authoritative for the discounted variant.
	// Verify ResolveSizes hits the exact URL segment given by ProviderRef
	// and does NOT fall back to "00".
	var seenPath string
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/l2s") {
			seenPath = r.URL.Path
			writeJSON(t, w, map[string]any{"status": "ok", "result": map[string]any{
				"l2s":    []map[string]any{},
				"stocks": map[string]any{},
			}})
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	_, err := c.ResolveSizes(context.Background(), deal.Candidate{ProductID: "E1", ProviderRef: "02"})
	if err != nil {
		t.Fatalf("ResolveSizes: %v", err)
	}
	if !strings.Contains(seenPath, "/price-groups/02/l2s") {
		t.Errorf("expected priceGroup 02 in URL, got %q", seenPath)
	}
}

func TestFetchDeals_LogsAndSkipsMalformedItem(t *testing.T) {
	// One good item and one item with a nil promo price — the mapper must
	// reject the bad item, keep the good one, and emit a warn-level log.
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		bad := map[string]any{
			"productId":  "E-bad",
			"name":       "Broken",
			"priceGroup": "00",
			// prices.promo intentionally missing -> pickPrice returns error.
			"prices": map[string]any{
				"base": map[string]any{"value": 10.0, "currency": map[string]string{"code": "EUR"}},
			},
		}
		writeJSON(t, w, map[string]any{
			"status": "ok",
			"result": map[string]any{
				"items":      []any{fakeItem("E1", "Socks", 10, 5, 5), bad},
				"pagination": map[string]int{"total": 2, "offset": 0, "count": 2},
			},
		})
	}))
	defer srv.Close()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	c := newTestClient(t, testConfig(srv.URL), logger)

	deals, err := c.FetchDeals(context.Background())
	if err != nil {
		t.Fatalf("FetchDeals: %v", err)
	}
	if len(deals) != 1 || deals[0].ProductID != "E1" {
		t.Fatalf("want 1 good deal (E1), got %+v", deals)
	}
	if !strings.Contains(logBuf.String(), "skip malformed listing item") {
		t.Errorf("expected warning log for malformed item; got: %s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "E-bad") {
		t.Errorf("log should mention productId E-bad: %s", logBuf.String())
	}
}

func TestGetJSON_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(versionHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		writeJSON(t, w, map[string]any{"status": "ok", "result": map[string]any{"items": []any{}}})
	}))
	defer srv.Close()

	c := newTestClient(t, testConfig(srv.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := c.FetchDeals(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
