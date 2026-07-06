package uniqlo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestHTTPVersionResolver_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><head>
			<script>window.__BUILD_VERSION__ = "9.9999.9";
			window.__LOCALISATION_VERSION__ = "1.0.0";</script>
		</head></html>`)
	}))
	defer srv.Close()

	r := newHTTPVersionResolver(&http.Client{Timeout: time.Second}, srv.URL, "de", "en", "test", newDiscardLogger())
	got, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "9.9999.9" {
		t.Errorf("Resolve() = %q, want %q", got, "9.9999.9")
	}
}

func TestHTTPVersionResolver_MissingSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html>no build version here</html>`)
	}))
	defer srv.Close()

	r := newHTTPVersionResolver(&http.Client{Timeout: time.Second}, srv.URL, "de", "en", "test", newDiscardLogger())
	_, err := r.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected error when sentinel is absent")
	}
}

func TestHTTPVersionResolver_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	r := newHTTPVersionResolver(&http.Client{Timeout: time.Second}, srv.URL, "de", "en", "test", newDiscardLogger())
	_, err := r.Resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("expected 503 error, got %v", err)
	}
}

// stubResolver is a test-only clientVersionResolver.
type stubResolver struct {
	value string
	err   error
	calls int
}

func (s *stubResolver) Resolve(_ context.Context) (string, error) {
	s.calls++
	return s.value, s.err
}

func TestCachedVersion_ResolvesOnce(t *testing.T) {
	stub := &stubResolver{value: "5.0.0"}
	cv := &cachedVersion{
		resolver: stub, fallback: "fallback", timeout: time.Second, logger: newDiscardLogger(),
	}

	for range 3 {
		if got := cv.Get(context.Background()); got != "5.0.0" {
			t.Errorf("Get() = %q, want %q", got, "5.0.0")
		}
	}
	if stub.calls != 1 {
		t.Errorf("resolver called %d times, want 1 (once semantics)", stub.calls)
	}
}

func TestCachedVersion_FallsBackOnError(t *testing.T) {
	stub := &stubResolver{err: errors.New("boom")}
	cv := &cachedVersion{
		resolver: stub, fallback: "fallback", timeout: time.Second, logger: newDiscardLogger(),
	}
	if got := cv.Get(context.Background()); got != "fallback" {
		t.Errorf("Get() = %q, want fallback", got)
	}
}

func TestCachedVersion_FallsBackOnEmpty(t *testing.T) {
	stub := &stubResolver{value: ""}
	cv := &cachedVersion{
		resolver: stub, fallback: "fallback", timeout: time.Second, logger: newDiscardLogger(),
	}
	if got := cv.Get(context.Background()); got != "fallback" {
		t.Errorf("Get() = %q, want fallback", got)
	}
}

func TestCachedVersion_NilResolverUsesFallback(t *testing.T) {
	cv := &cachedVersion{
		resolver: nil, fallback: "only-choice", timeout: time.Second, logger: newDiscardLogger(),
	}
	if got := cv.Get(context.Background()); got != "only-choice" {
		t.Errorf("Get() = %q, want only-choice", got)
	}
}
