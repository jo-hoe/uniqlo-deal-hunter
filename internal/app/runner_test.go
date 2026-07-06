package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/filter"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/notifier"
)

// --- Fakes ---

type fakeSource struct {
	candidates []deal.Candidate
	sizesByID  map[deal.ProductID][]deal.Size
	fetchErr   error
	sizesErr   error
}

func (f *fakeSource) FetchDeals(_ context.Context) ([]deal.Candidate, error) {
	return f.candidates, f.fetchErr
}
func (f *fakeSource) ResolveSizes(_ context.Context, id deal.ProductID) ([]deal.Size, error) {
	if f.sizesErr != nil {
		return nil, f.sizesErr
	}
	return f.sizesByID[id], nil
}

type recordingNotifier struct {
	sent [][]notifier.MatchedDeal
	err  error
}

func (r *recordingNotifier) Notify(_ context.Context, deals []notifier.MatchedDeal) error {
	if r.err != nil {
		return r.err
	}
	// Copy to defend against later mutation.
	cp := make([]notifier.MatchedDeal, len(deals))
	copy(cp, deals)
	r.sent = append(r.sent, cp)
	return nil
}

type memoryStore struct {
	seen map[deal.Key]time.Time
}

func newMemoryStore() *memoryStore { return &memoryStore{seen: make(map[deal.Key]time.Time)} }
func (m *memoryStore) IsNew(_ context.Context, k deal.Key) (bool, error) {
	_, ok := m.seen[k]
	return !ok, nil
}
func (m *memoryStore) MarkSeen(_ context.Context, d deal.Candidate, _ string, at time.Time) error {
	m.seen[d.Key()] = at
	return nil
}
func (m *memoryStore) Prune(_ context.Context, cutoff time.Time) (int64, error) {
	var n int64
	for k, v := range m.seen {
		if v.Before(cutoff) {
			delete(m.seen, k)
			n++
		}
	}
	return n, nil
}
func (m *memoryStore) Close() error { return nil }

// --- Helpers ---

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func newTestConfig() *config.Config {
	return &config.Config{
		Rules: []config.Rule{{Name: "any", MaxPriceEUR: dec("100")}},
		Store: config.Store{RetentionDays: 30},
	}
}

func newTestRunner(t *testing.T, src *fakeSource, notif *recordingNotifier, st *memoryStore) *Runner {
	t.Helper()
	cfg := newTestConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewRunner(cfg, src, filter.New(cfg.Rules), notif, st, logger)
}

// --- Tests ---

func TestRunner_HappyPath_NotifiesOnce(t *testing.T) {
	c := deal.Candidate{ProductID: "E1", Name: "Socks", URL: "u", PromoPrice: dec("3"), BasePrice: dec("10")}
	src := &fakeSource{
		candidates: []deal.Candidate{c},
		sizesByID:  map[deal.ProductID][]deal.Size{"E1": {{Code: "M", Label: "M", InStock: true}}},
	}
	notif := &recordingNotifier{}
	st := newMemoryStore()

	r := newTestRunner(t, src, notif, st)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(notif.sent) != 1 || len(notif.sent[0]) != 1 {
		t.Fatalf("expected 1 email with 1 deal, got %+v", notif.sent)
	}
	if _, ok := st.seen[c.Key()]; !ok {
		t.Errorf("expected key in store")
	}
}

func TestRunner_Dedup_SkipsAlreadySeen(t *testing.T) {
	c := deal.Candidate{ProductID: "E1", Name: "Socks", URL: "u", PromoPrice: dec("3"), BasePrice: dec("10")}
	src := &fakeSource{
		candidates: []deal.Candidate{c},
		sizesByID:  map[deal.ProductID][]deal.Size{"E1": {{Code: "M", Label: "M", InStock: true}}},
	}
	notif := &recordingNotifier{}
	st := newMemoryStore()
	// Prime the store as if we already notified.
	_ = st.MarkSeen(context.Background(), c, "any", time.Now())

	r := newTestRunner(t, src, notif, st)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(notif.sent) != 0 {
		t.Errorf("expected no email, got %+v", notif.sent)
	}
}

func TestRunner_NotifyFailure_DoesNotMarkSeen(t *testing.T) {
	c := deal.Candidate{ProductID: "E1", Name: "Socks", URL: "u", PromoPrice: dec("3"), BasePrice: dec("10")}
	src := &fakeSource{
		candidates: []deal.Candidate{c},
		sizesByID:  map[deal.ProductID][]deal.Size{"E1": {{Code: "M", Label: "M", InStock: true}}},
	}
	notif := &recordingNotifier{err: errors.New("smtp down")}
	st := newMemoryStore()

	r := newTestRunner(t, src, notif, st)
	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if len(st.seen) != 0 {
		t.Errorf("store should be empty after notify failure, got %v", st.seen)
	}
}

func TestRunner_SkipsOnResolveSizesError(t *testing.T) {
	c := deal.Candidate{ProductID: "E1", Name: "Socks", URL: "u", PromoPrice: dec("3"), BasePrice: dec("10")}
	src := &fakeSource{
		candidates: []deal.Candidate{c},
		sizesErr:   errors.New("l2s 500"),
	}
	notif := &recordingNotifier{}
	st := newMemoryStore()

	r := newTestRunner(t, src, notif, st)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run should tolerate item-level errors: %v", err)
	}
	if len(notif.sent) != 0 {
		t.Errorf("expected no notification when sizes unresolvable, got %+v", notif.sent)
	}
}

func TestRunner_FetchFailure_ReturnsError(t *testing.T) {
	src := &fakeSource{fetchErr: errors.New("api dead")}
	notif := &recordingNotifier{}
	st := newMemoryStore()

	r := newTestRunner(t, src, notif, st)
	if err := r.Run(context.Background()); err == nil {
		t.Fatal("expected fetch error to propagate")
	}
}
