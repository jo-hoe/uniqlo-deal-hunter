package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfig = `
source:
  kind: uniqlo
  baseURL: https://www.uniqlo.com
  region: de
  language: en
  gender: men
  sizeCodes: ["MSC027"]
  sort: 2
  clientID: uq.de.web-spa
  requestsPerSecond: 1
rules:
  - name: any
    namePattern: "(?i)test"
notifier:
  kind: smtp
  smtp:
    host: smtp.example.com
    port: 587
    startTLS: true
    from: a@b.c
    to: ["x@y.z"]
    username: u
    passwordFile: /tmp/pw
store:
  kind: sqlite
  path: /var/db.sqlite
logging:
  level: debug
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeConfig(t, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Source.Kind != SourceKindUniqlo {
		t.Errorf("Source.Kind = %q", cfg.Source.Kind)
	}
	if len(cfg.Rules) != 1 || cfg.Rules[0].NameRegex() == nil {
		t.Errorf("rule regex not compiled: %+v", cfg.Rules)
	}
	if cfg.Store.RetentionDays != 90 {
		t.Errorf("default retentionDays = %d, want 90", cfg.Store.RetentionDays)
	}
	if cfg.Source.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d, want 3", cfg.Source.MaxRetries)
	}
}

func TestLoad_InvalidRegex(t *testing.T) {
	bad := `
source:
  kind: uniqlo
  baseURL: https://x
  region: de
  language: en
  gender: men
  clientID: c
rules:
  - name: r
    namePattern: "(unclosed"
notifier:
  kind: smtp
  smtp: {host: s, port: 25, from: a, to: [x], passwordFile: /p}
store: {kind: sqlite, path: /db}
`
	path := writeConfig(t, bad)
	_, err := Load(path)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig, got %v", err)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	cases := map[string]string{
		"no source kind": `
source: {baseURL: x, region: de, language: en, gender: men, clientID: c}
rules: [{name: r}]
notifier: {kind: smtp, smtp: {host: s, port: 25, from: a, to: [x], passwordFile: /p}}
store: {kind: sqlite, path: /db}
`,
		"no rules": `
source: {kind: uniqlo, baseURL: x, region: de, language: en, gender: men, clientID: c}
rules: []
notifier: {kind: smtp, smtp: {host: s, port: 25, from: a, to: [x], passwordFile: /p}}
store: {kind: sqlite, path: /db}
`,
		"no smtp host": `
source: {kind: uniqlo, baseURL: x, region: de, language: en, gender: men, clientID: c}
rules: [{name: r}]
notifier: {kind: smtp, smtp: {port: 25, from: a, to: [x], passwordFile: /p}}
store: {kind: sqlite, path: /db}
`,
		"bad port": `
source: {kind: uniqlo, baseURL: x, region: de, language: en, gender: men, clientID: c}
rules: [{name: r}]
notifier: {kind: smtp, smtp: {host: s, port: 0, from: a, to: [x], passwordFile: /p}}
store: {kind: sqlite, path: /db}
`,
		"no store path": `
source: {kind: uniqlo, baseURL: x, region: de, language: en, gender: men, clientID: c}
rules: [{name: r}]
notifier: {kind: smtp, smtp: {host: s, port: 25, from: a, to: [x], passwordFile: /p}}
store: {kind: sqlite}
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, body)
			_, err := Load(path)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("want ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestLoad_FileMissing(t *testing.T) {
	_, err := Load("/does/not/exist.yaml")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestLoad_DefaultUserAgentIsBrowserShaped(t *testing.T) {
	path := writeConfig(t, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Source.UserAgent != DefaultUserAgent {
		t.Errorf("UA default did not apply; got %q", cfg.Source.UserAgent)
	}
	// Guard against regressing to obviously-bot UAs like "uniqlo-deal-hunter/1.0".
	if !strings.HasPrefix(DefaultUserAgent, "Mozilla/5.0") {
		t.Errorf("DefaultUserAgent should be browser-shaped, got %q", DefaultUserAgent)
	}
}
