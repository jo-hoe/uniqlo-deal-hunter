//go:build integration

// Package integration exercises the full pipeline end-to-end against
// in-process fakes for the Uniqlo API and an SMTP server. No real network
// egress. Run via: go test -tags=integration ./test/integration/...
package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/app"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/filter"
	smtpn "github.com/jo-hoe/uniqlo-deal-hunter/internal/notifier/smtp"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/source/uniqlo"
	sqlitestore "github.com/jo-hoe/uniqlo-deal-hunter/internal/store/sqlite"
)

// Silence unused-import complaint if a future edit drops context/io.
var _ = context.Background
var _ = io.Discard

// fakeSMTPServer is a bare-bones SMTP server that accepts one message per
// connection and records the DATA body. Sufficient to verify the pipeline
// sends what we expect.
type fakeSMTPServer struct {
	addr    string
	closeFn func()
	mu      sync.Mutex
	bodies  []string
}

func newFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTPServer{addr: l.Addr().String()}
	s.closeFn = func() { _ = l.Close() }

	go s.serve(l)
	t.Cleanup(s.closeFn)
	return s
}

func (s *fakeSMTPServer) serve(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

// handleConn runs a scripted SMTP dialog: greet, accept HELO/EHLO, MAIL,
// RCPT, DATA (collect until "\r\n.\r\n"), QUIT.
func (s *fakeSMTPServer) handleConn(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	writeLine(c, "220 fake ESMTP")

	var body strings.Builder
	inData := false
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				writeLine(c, "250 OK")
				inData = false
				s.mu.Lock()
				s.bodies = append(s.bodies, body.String())
				s.mu.Unlock()
				body.Reset()
				continue
			}
			body.WriteString(line)
			body.WriteString("\r\n")
			continue
		}
		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			writeLine(c, "250-fake\r\n250 AUTH PLAIN")
		case strings.HasPrefix(line, "AUTH"):
			writeLine(c, "235 OK")
		case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
			writeLine(c, "250 OK")
		case strings.HasPrefix(line, "DATA"):
			writeLine(c, "354 send data")
			inData = true
		case strings.HasPrefix(line, "QUIT"):
			writeLine(c, "221 bye")
			return
		default:
			writeLine(c, "250 OK")
		}
	}
}

func writeLine(c net.Conn, s string) { fmt.Fprintf(c, "%s\r\n", s) }

func (s *fakeSMTPServer) BodyCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *fakeSMTPServer) LastBody() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.bodies) == 0 {
		return ""
	}
	return s.bodies[len(s.bodies)-1]
}

// fakeUniqlo returns an httptest server that serves one listing page with
// one discounted item and an l2s endpoint claiming size M is in stock.
func fakeUniqlo(t *testing.T) *httptest.Server {
	t.Helper()
	item := map[string]any{
		"productId":  "E1",
		"name":       "Cotton Socks",
		"priceGroup": "00",
		"prices": map[string]any{
			"base":  map[string]any{"value": 4.9, "currency": map[string]string{"code": "EUR"}},
			"promo": map[string]any{"value": 2.9, "currency": map[string]string{"code": "EUR"}},
			"lowestPriceDetails": map[string]any{
				"canDisplayLowestPrice": true,
				"lowestPeriod":          30,
				"lowestPrice":           4.9,
			},
		},
		"sizes":  []map[string]any{{"code": "M", "name": "Medium"}},
		"colors": []map[string]string{{"code": "C1", "displayCode": "01"}},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/de/en/" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><script>window.__BUILD_VERSION__ = "9.9.9";</script></html>`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/l2s") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"result": map[string]any{
					"l2s": []map[string]any{
						{"l2Id": "L1", "size": map[string]any{"code": "M", "name": "Medium"},
							"color": map[string]string{"code": "C1"}, "sales": true},
					},
					"stocks": map[string]any{
						"L1": map[string]any{"statusCode": "IN_STOCK", "quantity": 3},
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"result": map[string]any{
				"items":      []any{item},
				"pagination": map[string]int{"total": 1, "offset": 0, "count": 1},
			},
		})
	})
	return httptest.NewServer(handler)
}

// buildRunner wires the real production packages against the fakes.
func buildRunner(t *testing.T, apiURL, smtpAddr, dbPath string) *app.Runner {
	t.Helper()
	host, port := splitAddr(t, smtpAddr)
	pw := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(pw, []byte("s3cret"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Source: config.Source{
			Kind: config.SourceKindUniqlo, BaseURL: apiURL,
			Region: "de", Language: "en", Segment: config.SegmentMen,
			ClientID: "test", RequestsPerSecond: 100, Timeout: 5 * time.Second, MaxRetries: 1,
			UserAgent: "test",
		},
		Rules: []config.Rule{{Name: "cheap-socks", MaxPriceEUR: decimal.RequireFromString("5")}},
		Notifier: config.Notifier{
			Kind: config.NotifierKindSMTP,
			SMTP: config.SMTP{
				Host: host, Port: port, StartTLS: false,
				From: "from@test", To: []string{"to@test"},
				Username: "u", PasswordFile: pw,
			},
		},
		Store:   config.Store{Kind: config.StoreKindSQLite, Path: dbPath, RetentionDays: 30},
		Logging: config.Logging{Level: "error"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	src := uniqlo.NewClient(cfg.Source, logger)
	eval := filter.New(cfg.Rules)
	notif, err := smtpn.New(cfg.Notifier.SMTP)
	if err != nil {
		t.Fatal(err)
	}
	st, err := sqlitestore.Open(cfg.Store.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	return app.NewRunner(cfg, src, eval, notif, st, logger)
}

func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var p int
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		t.Fatal(err)
	}
	return host, p
}
