package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
	notif "github.com/jo-hoe/uniqlo-deal-hunter/internal/notifier"
)

type fakeWriter struct {
	buf    bytes.Buffer
	closed bool
}

func (f *fakeWriter) Write(p []byte) (int, error) { return f.buf.Write(p) }
func (f *fakeWriter) Close() error                 { f.closed = true; return nil }

type fakeClient struct {
	starttlsCalled bool
	authCalled     bool
	mailFrom       string
	rcptTo         []string
	data           *fakeWriter
	quitCalled     bool
}

func (c *fakeClient) StartTLS(_ *tls.Config) error { c.starttlsCalled = true; return nil }
func (c *fakeClient) Auth(_ smtp.Auth) error       { c.authCalled = true; return nil }
func (c *fakeClient) Mail(from string) error       { c.mailFrom = from; return nil }
func (c *fakeClient) Rcpt(to string) error         { c.rcptTo = append(c.rcptTo, to); return nil }
func (c *fakeClient) Data() (writeCloser, error) {
	c.data = &fakeWriter{}
	return c.data, nil
}
func (c *fakeClient) Quit() error                          { c.quitCalled = true; return nil }
func (c *fakeClient) Close() error                         { return nil }
func (c *fakeClient) Extension(_ string) (bool, string)    { return false, "" }

// Compile-time check that the fake really implements the interface.
var _ Session = (*fakeClient)(nil)

func writePwFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func newNotifier(t *testing.T, fc *fakeClient) *Notifier {
	t.Helper()
	pw := writePwFile(t, "s3cret\n")
	n, err := New(config.SMTP{
		Host: "mail", Port: 587, StartTLS: true,
		From: "a@b.c", To: []string{"x@y.z"},
		Username: "u", PasswordFile: pw,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	n.dial = func(addr string) (Session, error) { return fc, nil }
	return n
}

func makeDeal(name, promo, base, lowest string, sizes ...deal.Size) notif.MatchedDeal {
	d := deal.Candidate{
		ProductID: "E1", Name: name, URL: "https://uniqlo.example/x",
		PromoPrice:     mustDec(promo),
		BasePrice:      mustDec(base),
		Lowest30dPrice: mustDec(lowest),
		Sizes:          sizes,
	}
	return notif.MatchedDeal{Deal: d, RuleName: "test-rule"}
}

func mustDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestNotify_RendersBulletsAndCallsSMTP(t *testing.T) {
	fc := &fakeClient{}
	n := newNotifier(t, fc)

	deals := []notif.MatchedDeal{
		makeDeal("Socks", "2.90", "4.90", "4.90", deal.Size{Label: "M", InStock: true}),
		makeDeal("Hat", "9.90", "19.90", "15.00"),
	}
	if err := n.Notify(context.Background(), deals); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	body := fc.data.buf.String()
	if !strings.Contains(body, "Socks") || !strings.Contains(body, "Hat") {
		t.Errorf("body missing product names: %s", body)
	}
	if !strings.Contains(body, "2.90") {
		t.Errorf("body missing promo price: %s", body)
	}
	if !strings.Contains(body, "test-rule") {
		t.Errorf("body missing rule name: %s", body)
	}
	if !strings.Contains(body, "https://uniqlo.example/x") {
		t.Errorf("body missing URL: %s", body)
	}
	if !fc.starttlsCalled || !fc.authCalled || fc.mailFrom != "a@b.c" || len(fc.rcptTo) != 1 || !fc.quitCalled {
		t.Errorf("smtp dialog incomplete: %+v", fc)
	}
}

func TestNotify_EmptyDeals_NoSend(t *testing.T) {
	fc := &fakeClient{}
	n := newNotifier(t, fc)
	if err := n.Notify(context.Background(), nil); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if fc.mailFrom != "" {
		t.Errorf("expected no dial on empty deals; got mail from=%q", fc.mailFrom)
	}
}

func TestReadPasswordFile_TrimsTrailingWhitespace(t *testing.T) {
	path := writePwFile(t, "hello \n")
	got, err := readPasswordFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("password = %q, want %q", got, "hello")
	}
}

// Sanity check: the fake writer's Close is called (so the SMTP body is
// completed) — mostly documents the contract for future maintenance.
func TestNotify_ClosesDataWriter(t *testing.T) {
	fc := &fakeClient{}
	n := newNotifier(t, fc)
	deals := []notif.MatchedDeal{makeDeal("Socks", "2.90", "4.90", "4.90")}
	if err := n.Notify(context.Background(), deals); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if fc.data == nil || !fc.data.closed {
		t.Errorf("data writer not closed: %+v", fc.data)
	}
	// Avoid an unused-import warning if io.Discard usage disappears.
	_ = io.Discard
}
