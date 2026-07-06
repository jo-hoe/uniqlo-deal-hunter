package uniqlo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPUserAgentResolver_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"milestone": 149, "version": "149.0.7827.201", "platform": "Windows", "channel": "Stable"}]`)
	}))
	defer srv.Close()

	r := newHTTPUserAgentResolver(&http.Client{Timeout: time.Second}, newDiscardLogger())
	r.feedURL = srv.URL

	got, err := r.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
	if got != want {
		t.Errorf("Resolve() =\n  %q\nwant\n  %q", got, want)
	}
}

func TestHTTPUserAgentResolver_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	r := newHTTPUserAgentResolver(&http.Client{Timeout: time.Second}, newDiscardLogger())
	r.feedURL = srv.URL
	_, err := r.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected error on empty release list")
	}
}

func TestHTTPUserAgentResolver_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()

	r := newHTTPUserAgentResolver(&http.Client{Timeout: time.Second}, newDiscardLogger())
	r.feedURL = srv.URL
	_, err := r.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestHTTPUserAgentResolver_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	r := newHTTPUserAgentResolver(&http.Client{Timeout: time.Second}, newDiscardLogger())
	r.feedURL = srv.URL
	_, err := r.Resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("expected 503 error, got %v", err)
	}
}

// stubUAResolver is a test-only userAgentResolver.
type stubUAResolver struct {
	value string
	err   error
	calls int
}

func (s *stubUAResolver) Resolve(_ context.Context) (string, error) {
	s.calls++
	return s.value, s.err
}

func TestCachedUserAgent_ResolvesOnce(t *testing.T) {
	stub := &stubUAResolver{value: "Chrome/999"}
	cu := &cachedUserAgent{
		resolver: stub, fallback: "fallback", timeout: time.Second, logger: newDiscardLogger(),
	}
	for range 3 {
		if got := cu.Get(context.Background()); got != "Chrome/999" {
			t.Errorf("Get() = %q", got)
		}
	}
	if stub.calls != 1 {
		t.Errorf("resolver called %d times, want 1", stub.calls)
	}
}

func TestCachedUserAgent_FallsBackOnError(t *testing.T) {
	stub := &stubUAResolver{err: errors.New("boom")}
	cu := &cachedUserAgent{
		resolver: stub, fallback: "fallback", timeout: time.Second, logger: newDiscardLogger(),
	}
	if got := cu.Get(context.Background()); got != "fallback" {
		t.Errorf("Get() = %q, want fallback", got)
	}
}

func TestCachedUserAgent_FallsBackOnEmpty(t *testing.T) {
	stub := &stubUAResolver{value: ""}
	cu := &cachedUserAgent{
		resolver: stub, fallback: "fallback", timeout: time.Second, logger: newDiscardLogger(),
	}
	if got := cu.Get(context.Background()); got != "fallback" {
		t.Errorf("Get() = %q, want fallback", got)
	}
}

func TestCachedUserAgent_NilResolverUsesFallback(t *testing.T) {
	cu := &cachedUserAgent{
		resolver: nil, fallback: "only-choice", timeout: time.Second, logger: newDiscardLogger(),
	}
	if got := cu.Get(context.Background()); got != "only-choice" {
		t.Errorf("Get() = %q, want only-choice", got)
	}
}
