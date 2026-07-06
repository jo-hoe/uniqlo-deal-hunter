package uniqlo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// chromiumDashURL is Google's official Chrome release dashboard. It exposes
// a small JSON endpoint that returns the current Stable release the moment
// it ships. Much fresher than any third-party UA list. Documented at
// https://chromiumdash.appspot.com/help
const chromiumDashURL = "https://chromiumdash.appspot.com/fetch_releases" +
	"?channel=Stable&platform=Windows&num=1"

// userAgentTemplate produces the exact User-Agent string a stock Chrome on
// Windows sends. Since Chrome's UA-Reduction (M100+, GA in M110), the
// build/patch numbers are frozen at zero, so we only need the milestone.
const userAgentTemplate = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36"

// userAgentResolver returns a browser-shaped User-Agent string. Invoked at
// most once per Client; the result is cached for the process lifetime.
type userAgentResolver interface {
	Resolve(ctx context.Context) (string, error)
}

// httpUserAgentResolver reads the current Chrome Stable milestone from
// Google's Chromium Dashboard and formats a matching Windows UA. This is
// the same source Chrome's own release tooling uses.
type httpUserAgentResolver struct {
	http    *http.Client
	feedURL string
	logger  *slog.Logger
}

func newHTTPUserAgentResolver(client *http.Client, logger *slog.Logger) *httpUserAgentResolver {
	return &httpUserAgentResolver{http: client, feedURL: chromiumDashURL, logger: logger}
}

// chromiumRelease is the subset of a Chromium Dash release entry we care
// about. The endpoint returns many other fields; we ignore them.
type chromiumRelease struct {
	Milestone int    `json:"milestone"`
	Version   string `json:"version"`
}

// Resolve implements userAgentResolver.
func (r *httpUserAgentResolver) Resolve(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.feedURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch chromium dash: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("chromium dash returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read chromium dash: %w", err)
	}
	var releases []chromiumRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("parse chromium dash: %w", err)
	}
	if len(releases) == 0 || releases[0].Milestone <= 0 {
		return "", fmt.Errorf("chromium dash returned no stable release")
	}
	return fmt.Sprintf(userAgentTemplate, releases[0].Milestone), nil
}

// cachedUserAgent lazily resolves the User-Agent once, then serves the
// cached value. Mirrors cachedVersion for symmetry.
type cachedUserAgent struct {
	once     sync.Once
	value    string
	fallback string
	resolver userAgentResolver
	timeout  time.Duration
	logger   *slog.Logger
}

// Get returns the resolved User-Agent, or the fallback if resolution fails.
// The passed-in context only bounds the initial resolve; later calls return
// the cached value immediately.
func (c *cachedUserAgent) Get(ctx context.Context) string {
	c.once.Do(func() {
		if c.resolver == nil {
			c.value = c.fallback
			return
		}
		rctx, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()
		v, err := c.resolver.Resolve(rctx)
		if err != nil || v == "" {
			c.logger.Warn("user-agent discovery failed, using fallback",
				"fallback", c.fallback, "err", err)
			c.value = c.fallback
			return
		}
		c.logger.Info("resolved user-agent from chromium dash", "userAgent", v)
		c.value = v
	})
	return c.value
}
