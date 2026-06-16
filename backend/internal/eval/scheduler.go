package eval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/robfig/cron/v3"
)

// EvalScheduler manages cron-triggered eval suite runs.
// A nil *EvalScheduler is valid; all methods are no-ops on nil.
type EvalScheduler struct {
	c       *cron.Cron
	entries map[string]cron.EntryID // suiteID → cron entry
	mu      sync.Mutex
	runner  evalRunnerI
	ctx     context.Context // server-lifetime context
}

// NewEvalScheduler creates an EvalScheduler. When ctx is cancelled
// (server shutdown) the scheduler stops automatically.
func NewEvalScheduler(ctx context.Context, runner evalRunnerI) *EvalScheduler {
	return newEvalSchedulerWith(ctx, runner, cron.New())
}

// newEvalSchedulerWith is the internal constructor used by NewEvalScheduler
// and tests (which inject a second-precision cron.Cron).
func newEvalSchedulerWith(ctx context.Context, runner evalRunnerI, c *cron.Cron) *EvalScheduler {
	s := &EvalScheduler{
		c:       c,
		entries: make(map[string]cron.EntryID),
		runner:  runner,
		ctx:     ctx,
	}
	go func() {
		<-ctx.Done()
		<-s.c.Stop().Done()
	}()
	return s
}

// LoadAll arms every suite in the slice. Call once at startup with the
// results of store.ListEvalSuitesByCronTrigger.
func (s *EvalScheduler) LoadAll(suites []store.EvalSuite) {
	if s == nil {
		return
	}
	for _, suite := range suites {
		if err := s.Arm(suite.ID, suite.CronExpr); err != nil {
			slog.Warn("eval scheduler: could not arm suite at startup",
				"suite_id", suite.ID,
				"cron_expr", suite.CronExpr,
				"error", err,
			)
		}
	}
}

// Arm schedules suiteID to fire on cronExpr. Replaces any existing schedule
// for the same suite atomically. Returns an error if the expression is invalid.
func (s *EvalScheduler) Arm(suiteID, cronExpr string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.entries[suiteID]; ok {
		s.c.Remove(old)
		delete(s.entries, suiteID)
	}

	id, err := s.c.AddFunc(cronExpr, func() {
		if _, err := s.runner.Execute(s.ctx, suiteID, "cron"); err != nil {
			slog.Error("eval scheduler: cron fire failed",
				"suite_id", suiteID, "error", err)
			if errors.Is(err, ErrWorkflowDeleted) {
				slog.Info("eval scheduler: disarming suite with deleted workflow", "suite_id", suiteID)
				s.Disarm(suiteID)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("eval scheduler: arm %q: %w", cronExpr, err)
	}
	s.entries[suiteID] = id
	return nil
}

// Disarm removes the scheduled job for suiteID. No-op if not scheduled.
func (s *EvalScheduler) Disarm(suiteID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entries[suiteID]; ok {
		s.c.Remove(id)
		delete(s.entries, suiteID)
	}
}

// Start begins the cron scheduler. Call after LoadAll.
func (s *EvalScheduler) Start() {
	if s == nil {
		return
	}
	s.c.Start()
}

// entryCount returns the number of currently armed suites. Used in tests.
func (s *EvalScheduler) entryCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}
