package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewWithWriter_JSON(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, "info")
	l.Info("hello", "k", "v")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v // %s", err, buf.String())
	}
	if got["msg"] != "hello" || got["k"] != "v" || got["level"] != "INFO" {
		t.Errorf("unexpected fields: %+v", got)
	}
}

func TestNewWithWriter_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, "warn")
	l.Info("suppressed")
	l.Warn("shown")
	if strings.Contains(buf.String(), "suppressed") {
		t.Errorf("info line leaked at warn level: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "shown") {
		t.Errorf("warn line missing: %s", buf.String())
	}
}

func TestNewWithWriter_UnknownLevelFallsBack(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, "bogus")
	l.Info("ok")
	if !strings.Contains(buf.String(), "ok") {
		t.Errorf("expected info to be emitted after fallback: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "unknown log level") {
		t.Errorf("expected fallback warning: %s", buf.String())
	}
}
