// Package app wires the domain packages together into a one-pass Runner.
// Everything expensive (HTTP, SMTP, disk) is behind an interface so the
// runner is trivially testable with fakes.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/deal"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/filter"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/notifier"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/source"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/store"
)

// Runner performs exactly one full scrape → filter → notify → persist pass.
type Runner struct {
	cfg       *config.Config
	src       source.Source
	eval      filter.Evaluator
	notifier  notifier.Notifier
	store     store.Store
	logger    *slog.Logger
	now       func() time.Time
}

// NewRunner constructs a Runner with the given collaborators.
func NewRunner(
	cfg *config.Config,
	src source.Source,
	eval filter.Evaluator,
	notif notifier.Notifier,
	st store.Store,
	logger *slog.Logger,
) *Runner {
	return &Runner{
		cfg:      cfg,
		src:      src,
		eval:     eval,
		notifier: notif,
		store:    st,
		logger:   logger,
		now:      time.Now,
	}
}

// Run executes the full pipeline. Returns non-nil only for errors that
// should exit the CronJob non-zero (config-level or source-level failures).
// Per-item errors during enrichment / persistence are logged and skipped.
func (r *Runner) Run(ctx context.Context) error {
	candidates, err := r.src.FetchDeals(ctx)
	if err != nil {
		return fmt.Errorf("fetch deals: %w", err)
	}
	r.logger.Info("fetched candidates", "count", len(candidates))

	matches := r.filterCandidates(candidates)
	r.logger.Info("candidates matched by rules", "count", len(matches))

	matches = r.enrichAndReconfirm(ctx, matches)
	r.logger.Info("candidates surviving size check", "count", len(matches))

	fresh := r.filterNew(ctx, matches)
	r.logger.Info("fresh (not previously notified) deals", "count", len(fresh))

	if err := r.notifyAndPersist(ctx, fresh); err != nil {
		return err
	}

	r.prune(ctx)
	return nil
}

// filterCandidates evaluates every candidate against the rule set and pairs
// matches with the rule that admitted them.
func (r *Runner) filterCandidates(cands []deal.Candidate) []notifier.MatchedDeal {
	out := make([]notifier.MatchedDeal, 0, len(cands))
	for _, c := range cands {
		if rule := r.eval.Match(c); rule != nil {
			out = append(out, notifier.MatchedDeal{Deal: c, RuleName: rule.Name})
		}
	}
	return out
}

// enrichAndReconfirm fetches authoritative sizes for each match, replaces
// the candidate's Sizes, and re-checks the rule with the fresh data.
// This handles the "listing lies about sizes" case: a candidate whose
// listing sizes said "M in stock" may lose the match if the L2 endpoint
// disagrees.
func (r *Runner) enrichAndReconfirm(ctx context.Context, matches []notifier.MatchedDeal) []notifier.MatchedDeal {
	out := make([]notifier.MatchedDeal, 0, len(matches))
	for _, m := range matches {
		enriched, ok := r.enrichOne(ctx, m)
		if ok {
			out = append(out, enriched)
		}
	}
	return out
}

// enrichOne resolves authoritative sizes for a single match, merges labels,
// drops it if nothing is in stock, and re-evaluates it against the rule set.
// Returns the updated match and true when it should proceed, false to drop.
func (r *Runner) enrichOne(ctx context.Context, m notifier.MatchedDeal) (notifier.MatchedDeal, bool) {
	sizes, err := r.src.ResolveSizes(ctx, m.Deal)
	if err != nil {
		r.logger.Warn("resolve sizes failed, skipping",
			"productId", m.Deal.ProductID, "err", err)
		return m, false
	}
	m.Deal.Sizes = mergeLabels(m.Deal.Sizes, sizes)
	if len(m.Deal.InStockSizeLabels()) == 0 {
		r.logger.Debug("dropped: no sizes in stock", "productId", m.Deal.ProductID)
		return m, false
	}
	matchedRule := r.eval.Match(m.Deal)
	if matchedRule == nil {
		r.logger.Debug("dropped after size refresh", "productId", m.Deal.ProductID)
		return m, false
	}
	m.RuleName = matchedRule.Name
	return m, true
}

// mergeLabels re-applies human-readable labels from the listing sizes onto the
// authoritative l2s sizes, which carry codes but not display names.
func mergeLabels(listing, authoritative []deal.Size) []deal.Size {
	labelByCode := make(map[string]string, len(listing))
	for _, s := range listing {
		if s.Label != "" {
			labelByCode[s.Code] = s.Label
		}
	}
	for i := range authoritative {
		if label, ok := labelByCode[authoritative[i].Code]; ok {
			authoritative[i].Label = label
		}
	}
	return authoritative
}

// filterNew drops matches the store has already seen.
func (r *Runner) filterNew(ctx context.Context, matches []notifier.MatchedDeal) []notifier.MatchedDeal {
	out := make([]notifier.MatchedDeal, 0, len(matches))
	for _, m := range matches {
		isNew, err := r.store.IsNew(ctx, m.Deal.Key())
		if err != nil {
			r.logger.Warn("dedup lookup failed, skipping",
				"productId", m.Deal.ProductID, "err", err)
			continue
		}
		if isNew {
			out = append(out, m)
		}
	}
	return out
}

// notifyAndPersist sends the digest and, on success, records each deal.
// Notification failure is fatal to the run: those deals must re-appear next
// time.
func (r *Runner) notifyAndPersist(ctx context.Context, fresh []notifier.MatchedDeal) error {
	if len(fresh) == 0 {
		r.logger.Info("no new deals to notify")
		return nil
	}
	if err := r.notifier.Notify(ctx, fresh); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	now := r.now().UTC()
	for _, m := range fresh {
		if err := r.store.MarkSeen(ctx, m.Deal, m.RuleName, now); err != nil {
			// Rare: log and continue. Worst case we re-notify next run.
			r.logger.Warn("mark seen failed",
				"productId", m.Deal.ProductID, "err", err)
		}
	}
	return nil
}

// prune deletes state older than the configured retention. Failures are
// logged but do not fail the run — pruning is a housekeeping best-effort.
func (r *Runner) prune(ctx context.Context) {
	if r.cfg.Store.RetentionDays <= 0 {
		return
	}
	cutoff := r.now().Add(-time.Duration(r.cfg.Store.RetentionDays) * 24 * time.Hour)
	n, err := r.store.Prune(ctx, cutoff)
	if err != nil {
		r.logger.Warn("prune failed", "err", err)
		return
	}
	if n > 0 {
		r.logger.Info("pruned old state rows", "count", n)
	}
}

// ErrRunFailed is returned by Run when the pipeline could not complete.
// Kept as a sentinel so main.go can log-and-exit uniformly.
var ErrRunFailed = errors.New("run failed")
