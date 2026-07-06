// Package smtp implements notifier.Notifier over SMTP with STARTTLS. Uses
// only stdlib net/smtp + crypto/tls so there is no third-party mail library
// to audit.
package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/notifier"
)

// Notifier is the SMTP implementation of notifier.Notifier.
type Notifier struct {
	cfg      config.SMTP
	password string
	// dial and send are injected for testing.
	dial func(addr string) (SMTPClient, error)
	now  func() time.Time
}

// SMTPClient captures the subset of *smtp.Client that this notifier needs.
// Exposed for test doubles.
type SMTPClient interface {
	StartTLS(config *tls.Config) error
	Auth(a smtp.Auth) error
	Mail(from string) error
	Rcpt(to string) error
	Data() (writeCloser, error)
	Quit() error
	Close() error
	Extension(name string) (bool, string)
}

// writeCloser is the interface Data returns. Split out for test doubles.
type writeCloser interface {
	Write(p []byte) (int, error)
	Close() error
}

// New constructs an SMTP notifier from config. Reads the password file at
// construction time and keeps the value in memory.
func New(cfg config.SMTP) (*Notifier, error) {
	pw, err := readPasswordFile(cfg.PasswordFile)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		cfg:      cfg,
		password: pw,
		dial:     defaultDial,
		now:      time.Now,
	}, nil
}

// readPasswordFile loads a secret file mounted from a k8s Secret.
// The trailing newline (common when k8s mounts secrets) is stripped.
func readPasswordFile(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-controlled path
	if err != nil {
		return "", fmt.Errorf("read smtp password file %q: %w", path, err)
	}
	return strings.TrimRight(string(b), "\r\n\t "), nil
}

// Notify implements notifier.Notifier.
func (n *Notifier) Notify(ctx context.Context, deals []notifier.MatchedDeal) error {
	if len(deals) == 0 {
		return nil
	}
	msg, err := n.renderMessage(deals)
	if err != nil {
		return err
	}
	return n.sendWithContext(ctx, msg)
}

// sendWithContext wraps the blocking net/smtp send in a context timeout so
// a hanging SMTP server cannot pin the CronJob forever.
func (n *Notifier) sendWithContext(ctx context.Context, msg []byte) error {
	timeout := n.cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- n.send(msg) }()
	select {
	case <-sendCtx.Done():
		return fmt.Errorf("smtp send: %w", sendCtx.Err())
	case err := <-errCh:
		return err
	}
}

// send performs the actual SMTP dialogue. Kept small; helpers below hold
// the STARTTLS / AUTH / RCPT ceremony.
func (n *Notifier) send(msg []byte) error {
	addr := fmt.Sprintf("%s:%d", n.cfg.Host, n.cfg.Port)
	client, err := n.dial(addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = client.Close() }()

	if err := n.negotiate(client); err != nil {
		return err
	}
	if err := writeEnvelope(client, n.cfg.From, n.cfg.To); err != nil {
		return err
	}
	if err := writeBody(client, msg); err != nil {
		return err
	}
	return client.Quit()
}

// negotiate performs STARTTLS + AUTH according to config.
func (n *Notifier) negotiate(c SMTPClient) error {
	if n.cfg.StartTLS {
		if err := c.StartTLS(&tls.Config{ServerName: n.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if n.cfg.Username != "" {
		auth := smtp.PlainAuth("", n.cfg.Username, n.password, n.cfg.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	return nil
}

// writeEnvelope issues MAIL FROM and RCPT TO for every recipient.
func writeEnvelope(c SMTPClient, from string, to []string) error {
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("rcpt to %s: %w", r, err)
		}
	}
	return nil
}

// writeBody streams the message body via DATA.
func writeBody(c SMTPClient, msg []byte) error {
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("write body: %w", err)
	}
	return w.Close()
}

// renderMessage builds a full MIME multipart message (text + html).
func (n *Notifier) renderMessage(deals []notifier.MatchedDeal) ([]byte, error) {
	subject := fmt.Sprintf("[uniqlo-deal-hunter] %d new deal(s)", len(deals))
	view := buildView(deals)

	var htmlBuf, textBuf bytes.Buffer
	if err := htmlTemplate.Execute(&htmlBuf, view); err != nil {
		return nil, fmt.Errorf("render html: %w", err)
	}
	if err := textTemplate.Execute(&textBuf, view); err != nil {
		return nil, fmt.Errorf("render text: %w", err)
	}

	var out bytes.Buffer
	writeHeaders(&out, subject, n.cfg.From, n.cfg.To)
	writeMultipart(&out, textBuf.Bytes(), htmlBuf.Bytes())
	return out.Bytes(), nil
}

// writeHeaders writes the fixed MIME headers.
func writeHeaders(w *bytes.Buffer, subject, from string, to []string) {
	fmt.Fprintf(w, "From: %s\r\n", from)
	fmt.Fprintf(w, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(w, "Subject: %s\r\n", subject)
	fmt.Fprintf(w, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(w, "Content-Type: multipart/alternative; boundary=%s\r\n", mimeBoundary)
	fmt.Fprint(w, "\r\n")
}

// writeMultipart emits the text/plain and text/html parts.
func writeMultipart(w *bytes.Buffer, text, html []byte) {
	fmt.Fprintf(w, "--%s\r\n", mimeBoundary)
	fmt.Fprint(w, "Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	w.Write(text)
	fmt.Fprint(w, "\r\n")
	fmt.Fprintf(w, "--%s\r\n", mimeBoundary)
	fmt.Fprint(w, "Content-Type: text/html; charset=UTF-8\r\n\r\n")
	w.Write(html)
	fmt.Fprint(w, "\r\n")
	fmt.Fprintf(w, "--%s--\r\n", mimeBoundary)
}

const mimeBoundary = "uniqlo-deal-hunter-boundary"

// dealView is the template input for one bullet.
type dealView struct {
	Name           string
	URL            string
	PromoEUR       string
	BaseEUR        string
	Lowest30dEUR   string
	DiscountPct    int
	MatchedRule    string
	InStockSizes   string
	Available      bool
}

// digestView is the top-level template input.
type digestView struct {
	Count int
	Deals []dealView
}

func buildView(matches []notifier.MatchedDeal) digestView {
	out := digestView{Count: len(matches), Deals: make([]dealView, 0, len(matches))}
	for _, m := range matches {
		out.Deals = append(out.Deals, dealView{
			Name:         m.Deal.Name,
			URL:          m.Deal.URL,
			PromoEUR:     m.Deal.PromoPrice.StringFixedBank(2),
			BaseEUR:      m.Deal.BasePrice.StringFixedBank(2),
			Lowest30dEUR: m.Deal.Lowest30dPrice.StringFixedBank(2),
			DiscountPct:  m.Deal.DiscountPercent(),
			MatchedRule:  m.RuleName,
			InStockSizes: strings.Join(m.Deal.InStockSizeLabels(), ", "),
			Available:    len(m.Deal.InStockSizeLabels()) > 0,
		})
	}
	return out
}

// defaultDial opens a plain TCP connection and returns a wrapper around
// *smtp.Client that satisfies our SMTPClient interface.
func defaultDial(addr string) (SMTPClient, error) {
	c, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}
	return &smtpAdapter{c: c}, nil
}

// smtpAdapter adapts *smtp.Client to our SMTPClient interface. The only
// mismatch is Data(): *smtp.Client returns io.WriteCloser, we want a
// writeCloser interface for test doubles.
type smtpAdapter struct{ c *smtp.Client }

func (a *smtpAdapter) StartTLS(cfg *tls.Config) error { return a.c.StartTLS(cfg) }
func (a *smtpAdapter) Auth(auth smtp.Auth) error      { return a.c.Auth(auth) }
func (a *smtpAdapter) Mail(from string) error         { return a.c.Mail(from) }
func (a *smtpAdapter) Rcpt(to string) error           { return a.c.Rcpt(to) }
func (a *smtpAdapter) Data() (writeCloser, error) {
	w, err := a.c.Data()
	return w, err
}
func (a *smtpAdapter) Quit() error                             { return a.c.Quit() }
func (a *smtpAdapter) Close() error                            { return a.c.Close() }
func (a *smtpAdapter) Extension(name string) (bool, string)    { return a.c.Extension(name) }

var (
	htmlTemplate = template.Must(template.New("digest.html").Parse(`
<!doctype html><html><body>
<p>{{.Count}} new deal(s):</p>
<ul>
{{range .Deals}}
  <li>
    <a href="{{.URL}}">{{.Name}}</a> —
    <strong>{{.PromoEUR}} €</strong>
    (was {{.BaseEUR}} €, 30-day low {{.Lowest30dEUR}} €, −{{.DiscountPct}}%)
    <br/>
    rule: <code>{{.MatchedRule}}</code>
    {{if .Available}}| sizes in stock: {{.InStockSizes}}{{else}}| <em>no sizes in stock</em>{{end}}
  </li>
{{end}}
</ul>
</body></html>
`))
	textTemplate = template.Must(template.New("digest.txt").Parse(
		`{{.Count}} new deal(s):
{{range .Deals}}
- {{.Name}} — {{.PromoEUR}} EUR (was {{.BaseEUR}}, 30-day low {{.Lowest30dEUR}}, -{{.DiscountPct}}%)
  rule: {{.MatchedRule}}{{if .Available}} | sizes: {{.InStockSizes}}{{else}} | no sizes in stock{{end}}
  {{.URL}}
{{end}}
`))
)
