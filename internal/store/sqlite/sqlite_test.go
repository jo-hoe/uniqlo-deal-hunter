package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func testCandidate(id deal.ProductID, promo string) deal.Candidate {
	return deal.Candidate{
		ProductID:  id,
		Name:       "Socks " + string(id),
		PromoPrice: dec(promo),
		BasePrice:  dec("10.00"),
	}
}

func TestStore_IsNew_And_MarkSeen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := testCandidate("E1", "3.00")
	isNew, err := s.IsNew(ctx, c.Key())
	if err != nil || !isNew {
		t.Fatalf("expected new, got isNew=%v err=%v", isNew, err)
	}
	if err := s.MarkSeen(ctx, c, "rule-a", time.Now()); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	isNew, err = s.IsNew(ctx, c.Key())
	if err != nil || isNew {
		t.Fatalf("expected not new after MarkSeen, got isNew=%v err=%v", isNew, err)
	}
}

func TestStore_KeyingByPromoCents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c1 := testCandidate("E1", "3.00")
	c2 := testCandidate("E1", "2.50") // Same product, different promo price.
	_ = s.MarkSeen(ctx, c1, "r", time.Now())

	isNew, err := s.IsNew(ctx, c2.Key())
	if err != nil || !isNew {
		t.Fatalf("c2 should be considered new: isNew=%v err=%v", isNew, err)
	}
}

func TestStore_MarkSeen_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := testCandidate("E1", "3.00")
	for range 3 {
		if err := s.MarkSeen(ctx, c, "r", time.Now()); err != nil {
			t.Fatalf("MarkSeen repeat: %v", err)
		}
	}
	isNew, _ := s.IsNew(ctx, c.Key())
	if isNew {
		t.Fatalf("expected not new after repeated MarkSeen")
	}
}

func TestStore_Prune(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	old := time.Now().Add(-30 * 24 * time.Hour)
	fresh := time.Now()
	_ = s.MarkSeen(ctx, testCandidate("E-old", "1.00"), "r", old)
	_ = s.MarkSeen(ctx, testCandidate("E-fresh", "1.00"), "r", fresh)

	cutoff := time.Now().Add(-14 * 24 * time.Hour)
	n, err := s.Prune(ctx, cutoff)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row pruned, got %d", n)
	}

	isNewOld, _ := s.IsNew(ctx, testCandidate("E-old", "1.00").Key())
	isNewFresh, _ := s.IsNew(ctx, testCandidate("E-fresh", "1.00").Key())
	if !isNewOld {
		t.Errorf("old row should have been pruned")
	}
	if isNewFresh {
		t.Errorf("fresh row should have survived")
	}
}
