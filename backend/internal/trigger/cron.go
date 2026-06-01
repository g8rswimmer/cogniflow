package trigger

import (
	"context"
	"fmt"
	"sync"

	"github.com/robfig/cron/v3"
)

// ValidateCronExpr parses expr using the standard five-field cron format and
// returns an error if the expression is empty or syntactically invalid.
// Call this in the API layer before saving a workflow to give callers a clear
// 400 response rather than a runtime failure when the scheduler tries to arm.
func ValidateCronExpr(expr string) error {
	if expr == "" {
		return fmt.Errorf("cron_expr is required when trigger.kind is \"cron\"")
	}
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return nil
}

// cronScheduler wraps a robfig/cron.Cron instance and tracks the entry ID
// per workflow so individual jobs can be updated or removed by workflow ID.
type cronScheduler struct {
	c       *cron.Cron
	entries map[string]cron.EntryID
	mu      sync.Mutex
}

func newCronScheduler() *cronScheduler {
	return &cronScheduler{
		c:       cron.New(),
		entries: make(map[string]cron.EntryID),
	}
}

// add schedules fn to run on expr for workflowID. If a job already exists for
// that workflow it is replaced atomically.
func (cs *cronScheduler) add(workflowID, expr string, fn func()) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if old, ok := cs.entries[workflowID]; ok {
		cs.c.Remove(old)
	}
	entryID, err := cs.c.AddFunc(expr, fn)
	if err != nil {
		// Remove the stale map entry so future Upserts don't see a ghost job.
		delete(cs.entries, workflowID)
		return fmt.Errorf("cron: schedule %q: %w", expr, err)
	}
	cs.entries[workflowID] = entryID
	return nil
}

// remove cancels the scheduled job for workflowID. It is a no-op if no job
// exists for that workflow.
func (cs *cronScheduler) remove(workflowID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if entryID, ok := cs.entries[workflowID]; ok {
		cs.c.Remove(entryID)
		delete(cs.entries, workflowID)
	}
}

// entryCount returns the number of currently scheduled jobs. Used in tests.
func (cs *cronScheduler) entryCount() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.entries)
}

func (cs *cronScheduler) start() { cs.c.Start() }

// stop halts the scheduler and returns a drain context that is cancelled once
// all in-flight job goroutines have returned. Callers should wait on it for a
// clean shutdown.
func (cs *cronScheduler) stop() context.Context { return cs.c.Stop() }
