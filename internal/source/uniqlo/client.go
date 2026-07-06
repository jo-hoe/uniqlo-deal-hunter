// Package uniqlo implements source.Source against Uniqlo's public commerce
// JSON API. Nothing in this package touches HTML — the API returns every
// field the domain needs, including the EU-mandated 30-day lowest price.
package uniqlo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
)

// Client talks to the Uniqlo commerce API.
type Client struct {
	http      *http.Client
	limiter   *rate.Limiter
	cfg       config.Source
	logger    *slog.Logger
	version   *cachedVersion
	userAgent *cachedUserAgent
}

// pageSize is the maximum items the Uniqlo API returns per page.
const pageSize = 36

// versionDiscoveryTimeout bounds the one-off client-version resolution.
// Kept small: if the storefront is slow we'd rather fall back to the
// compiled-in default than delay the run.
const versionDiscoveryTimeout = 5 * time.Second

// userAgentDiscoveryTimeout bounds the one-off User-Agent resolution.
const userAgentDiscoveryTimeout = 5 * time.Second

// NewClient constructs a Client from the source-config block. The returned
// Client is safe for the CronJob's single-goroutine use pattern.
// A nil logger is replaced with slog.Default() so callers may pass nil.
//
// On first use the client performs two one-off probes and caches the
// results for the process lifetime:
//   - x-fr-client-version: fetched from the Uniqlo storefront's
//     window.__BUILD_VERSION__ (falls back to cfg.ClientVersion).
//   - User-Agent: fetched from Google's Chromium Dashboard, then
//     formatted as the current Chrome-on-Windows UA (falls back to
//     cfg.UserAgent, itself defaulting to config.DefaultUserAgent).
func NewClient(cfg config.Source, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	httpClient := &http.Client{Timeout: cfg.Timeout}
	verResolver := newHTTPVersionResolver(httpClient, cfg.BaseURL, cfg.Region, cfg.Language, cfg.UserAgent, logger)
	uaResolver := newHTTPUserAgentResolver(httpClient, logger)
	return &Client{
		http:    httpClient,
		limiter: rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), 1),
		cfg:     cfg,
		logger:  logger,
		version: &cachedVersion{
			resolver: verResolver,
			fallback: cfg.ClientVersion,
			timeout:  versionDiscoveryTimeout,
			logger:   logger,
		},
		userAgent: &cachedUserAgent{
			resolver: uaResolver,
			fallback: cfg.UserAgent,
			timeout:  userAgentDiscoveryTimeout,
			logger:   logger,
		},
	}
}

// listingURL builds the products endpoint URL for a specific offset.
//
// The path parameter encodes the Uniqlo category tree as
// "<genderId>,<l1>,<l2>,<l3>" — for a plain gender-level browse we send
// the ID plus three empty segments (that's exactly what the SPA does).
// The genderId query parameter is sent redundantly, matching browser
// behaviour so any server-side validation stays happy.
func (c *Client) listingURL(offset int) string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	gid := c.cfg.Gender.GenderID()
	q := url.Values{}
	q.Set("path", fmt.Sprintf("%d,,,", gid))
	q.Set("flagCodes", "discount")
	if len(c.cfg.SizeCodes) > 0 {
		q.Set("sizeCodes", strings.Join(c.cfg.SizeCodes, ","))
	}
	q.Set("sort", strconv.Itoa(c.cfg.Sort))
	q.Set("genderId", strconv.Itoa(gid))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("httpFailure", "true")
	return fmt.Sprintf("%s/%s/api/commerce/v5/%s/products?%s",
		base, c.cfg.Region, c.cfg.Language, q.Encode())
}

// detailURL builds the l2s endpoint URL for a product.
func (c *Client) detailURL(id, priceGroup string) string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	return fmt.Sprintf(
		"%s/%s/api/commerce/v5/%s/products/%s/price-groups/%s/l2s?withPrices=true&withStocks=true&httpFailure=true",
		base, c.cfg.Region, c.cfg.Language, url.PathEscape(id), url.PathEscape(priceGroup))
}

// getJSON issues a GET with headers, rate limiting, retries, and JSON decode.
func (c *Client) getJSON(ctx context.Context, target string, out any) error {
	backoff := 500 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate-limiter wait: %w", err)
		}
		status, body, err := c.doOnce(ctx, target)
		if err == nil && status < 400 {
			return json.Unmarshal(body, out)
		}
		lastErr = classifyErr(err, status, body)
		if !isRetryable(status, err) || attempt == c.cfg.MaxRetries {
			return lastErr
		}
		if err := sleep(ctx, backoff+jitter(backoff)); err != nil {
			return err
		}
		backoff *= 2
	}
	return lastErr
}

// doOnce performs one HTTP round trip and returns status + body.
func (c *Client) doOnce(ctx context.Context, target string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-DE,en;q=0.9")
	req.Header.Set("User-Agent", c.userAgent.Get(ctx))
	req.Header.Set("x-fr-clientid", c.cfg.ClientID)
	if v := c.version.Get(ctx); v != "" {
		req.Header.Set("x-fr-client-version", v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read body: %w", err)
	}
	return resp.StatusCode, body, nil
}

// isRetryable reports whether an error/status warrants another attempt.
// Transport-level errors (net.OpError, HTTP/2 stream reset, EOF, timeout)
// are retried; so are 429 and 5xx status codes. Client-side 4xx are not.
func isRetryable(status int, err error) bool {
	if err != nil {
		// Any transport-level error is retryable. If it were a context
		// cancellation the request would have returned ctx.Err() at the
		// limiter.Wait step, so by the time we're here it's a real network
		// problem worth retrying.
		return true
	}
	return status == http.StatusTooManyRequests || status >= 500
}

// classifyErr produces a stable, wrapped error for a failed request.
func classifyErr(err error, status int, body []byte) error {
	if err != nil {
		return err
	}
	trim := string(body)
	if len(trim) > 200 {
		trim = trim[:200]
	}
	return fmt.Errorf("upstream returned %d: %s", status, trim)
}

// sleep is a context-aware time.Sleep.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// jitter returns a random duration in [0, d/2) to spread retry storms.
// math/rand/v2 is deliberately used here — this is not a security-sensitive
// value, it's a backoff jitter, and crypto/rand would be excessive.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(d / 2))) //nolint:gosec // non-crypto jitter
}

// ErrEmptyResponse signals a well-formed but empty API response, useful for
// tests to distinguish "no data" from decode errors.
var ErrEmptyResponse = errors.New("empty response")
