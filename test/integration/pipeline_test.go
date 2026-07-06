//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestPipeline_FirstRunNotifiesOnce_SecondRunDedups is the whole point of
// this integration test: run the real Runner against fake collaborators,
// once, assert one email lands; run it a second time with the same
// fixtures, assert no email is sent (dedup kicked in).
func TestPipeline_FirstRunNotifiesOnce_SecondRunDedups(t *testing.T) {
	api := fakeUniqlo(t)
	defer api.Close()

	smtp := newFakeSMTPServer(t)
	dbPath := filepath.Join(t.TempDir(), "state.db")

	// First run — fresh state, should notify once.
	r1 := buildRunner(t, api.URL, smtp.addr, dbPath)
	if err := r1.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if smtp.BodyCount() != 1 {
		t.Fatalf("first run: expected 1 email, got %d", smtp.BodyCount())
	}
	body := smtp.LastBody()
	if !strings.Contains(body, "Cotton Socks") {
		t.Errorf("email body missing product name: %s", body)
	}
	if !strings.Contains(body, "cheap-socks") {
		t.Errorf("email body missing rule name: %s", body)
	}

	// Second run — same fixtures, same DB. Store should say IsNew=false.
	r2 := buildRunner(t, api.URL, smtp.addr, dbPath)
	if err := r2.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if smtp.BodyCount() != 1 {
		t.Errorf("second run: expected still 1 email (dedup), got %d", smtp.BodyCount())
	}
}
