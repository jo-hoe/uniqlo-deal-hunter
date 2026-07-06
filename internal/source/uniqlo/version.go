package uniqlo

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// versionExpr matches the SSR-embedded declaration
//     window.__BUILD_VERSION__ = "3.2509.1";
// The version string is any dotted numeric release identifier.
var versionExpr = regexp.MustCompile(`__BUILD_VERSION__\s*=\s*"([0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.]+)?)"`)

// clientVersionResolver returns the current Uniqlo SPA build version to send
// in the x-fr-client-version header. It is invoked at most once per Client;
// the result is cached for the process lifetime. A nil resolver disables
// dynamic discovery and the config's ClientVersion value is used as-is.
type clientVersionResolver interface {
	Resolve(ctx context.Context) (string, error)
}

// httpVersionResolver reads window.__BUILD_VERSION__ from the storefront
// root. Cheap: the root page is small and heavily edge-cached.
type httpVersionResolver struct {
	http    *http.Client
	baseURL string
	region  string
	lang    string
	ua      string
	logger  *slog.Logger
}

func newHTTPVersionResolver(client *http.Client, baseURL, region, lang, userAgent string, logger *slog.Logger) *httpVersionResolver {
	return &httpVersionResolver{
		http: client, baseURL: baseURL, region: region, lang: lang, ua: userAgent,
		logger: logger,
	}
}

// Resolve implements clientVersionResolver.
func (r *httpVersionResolver) Resolve(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/%s/%s/", r.baseURL, r.region, r.lang)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", r.ua)
	req.Header.Set("Accept", "text/html")
	resp, err := r.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch storefront: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("storefront returned status %d", resp.StatusCode)
	}
	// Cap the read so a hostile response can't OOM the pod.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", fmt.Errorf("read storefront: %w", err)
	}
	m := versionExpr.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("build version not found in storefront HTML")
	}
	return string(m[1]), nil
}

// cachedVersion holds a lazily-resolved client version with sync.Once semantics.
type cachedVersion struct {
	once     sync.Once
	value    string
	fallback string
	resolver clientVersionResolver
	timeout  time.Duration
	logger   *slog.Logger
}

// Get returns the resolved version, or the fallback if resolution fails.
// Uses the passed-in context solely to bound the initial resolution call;
// after that the cached value is returned immediately.
func (c *cachedVersion) Get(ctx context.Context) string {
	c.once.Do(func() {
		if c.resolver == nil {
			c.value = c.fallback
			return
		}
		rctx, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()
		v, err := c.resolver.Resolve(rctx)
		if err != nil || v == "" {
			c.logger.Warn("client-version discovery failed, using fallback",
				"fallback", c.fallback, "err", err)
			c.value = c.fallback
			return
		}
		c.logger.Info("resolved uniqlo client version", "version", v)
		c.value = v
	})
	return c.value
}
