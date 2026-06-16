package eval

import (
	"context"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/robfig/cron/v3"
)

// stubRunnerForSched is a minimal evalRunnerI for scheduler tests.
type stubRunnerForSched struct {
	fired chan string // receives suiteID on each Execute call
	err   error
}

func (r *stubRunnerForSched) Execute(_ context.Context, suiteID string, _ string) (string, error) {
	if r.fired != nil {
		select {
		case r.fired <- suiteID:
		default:
		}
	}
	return "run-id", r.err
}

// newTestScheduler creates a standard five-field cron scheduler for unit tests
// that only check entry counts, not actual firing.
func newTestScheduler(ctx context.Context, runner evalRunnerI) *EvalScheduler {
	return newEvalSchedulerWith(ctx, runner, cron.New())
}

// newTestSchedulerSeconds creates a second-precision scheduler so firing tests
// can complete in < 3 s instead of waiting up to 60 s.
func newTestSchedulerSeconds(ctx context.Context, runner evalRunnerI) *EvalScheduler {
	return newEvalSchedulerWith(ctx, runner, cron.New(cron.WithSeconds()))
}

func TestEvalScheduler_Arm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newTestScheduler(ctx, &stubRunnerForSched{})

	if err := s.Arm("suite-1", "* * * * *"); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if s.entryCount() != 1 {
		t.Errorf("want 1 entry, got %d", s.entryCount())
	}
}

func TestEvalScheduler_ArmInvalidExpr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newTestScheduler(ctx, &stubRunnerForSched{})

	if err := s.Arm("suite-1", "not-a-cron"); err == nil {
		t.Error("expected error for invalid expression")
	}
	if s.entryCount() != 0 {
		t.Errorf("want 0 entries after failed arm, got %d", s.entryCount())
	}
}

func TestEvalScheduler_ArmReplaces(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newTestScheduler(ctx, &stubRunnerForSched{})

	if err := s.Arm("suite-1", "* * * * *"); err != nil {
		t.Fatalf("first Arm: %v", err)
	}
	if err := s.Arm("suite-1", "0 * * * *"); err != nil {
		t.Fatalf("second Arm: %v", err)
	}
	if s.entryCount() != 1 {
		t.Errorf("want 1 entry after replace, got %d", s.entryCount())
	}
}

func TestEvalScheduler_Disarm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newTestScheduler(ctx, &stubRunnerForSched{})

	if err := s.Arm("suite-1", "* * * * *"); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	s.Disarm("suite-1")
	if s.entryCount() != 0 {
		t.Errorf("want 0 entries after Disarm, got %d", s.entryCount())
	}
}

func TestEvalScheduler_DisarmNonexistent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newTestScheduler(ctx, &stubRunnerForSched{})
	s.Disarm("ghost") // must not panic
}

func TestEvalScheduler_LoadAll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newTestScheduler(ctx, &stubRunnerForSched{})

	suites := []store.EvalSuite{
		{ID: "s1", TriggerKind: "cron", CronExpr: "* * * * *"},
		{ID: "s2", TriggerKind: "cron", CronExpr: "0 * * * *"},
	}
	s.LoadAll(suites)
	if s.entryCount() != 2 {
		t.Errorf("want 2 entries after LoadAll, got %d", s.entryCount())
	}
}

func TestEvalScheduler_LoadAllSkipsBadExpr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newTestScheduler(ctx, &stubRunnerForSched{})

	suites := []store.EvalSuite{
		{ID: "s1", TriggerKind: "cron", CronExpr: "* * * * *"},
		{ID: "s2", TriggerKind: "cron", CronExpr: "bad-expr"},
	}
	s.LoadAll(suites)
	if s.entryCount() != 1 {
		t.Errorf("want 1 entry (bad expr skipped), got %d", s.entryCount())
	}
}

func TestEvalScheduler_NilSafe(t *testing.T) {
	var s *EvalScheduler
	if err := s.Arm("x", "* * * * *"); err != nil {
		t.Errorf("nil Arm returned error: %v", err)
	}
	s.Disarm("x")
	s.LoadAll(nil)
	s.Start()
	if s.entryCount() != 0 {
		t.Errorf("nil entryCount: want 0, got %d", s.entryCount())
	}
}

func TestEvalScheduler_Fires(t *testing.T) {
	// Second-precision cron "* * * * * *" fires every second.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fired := make(chan string, 1)
	runner := &stubRunnerForSched{fired: fired}
	s := newTestSchedulerSeconds(ctx, runner)
	s.Start()

	if err := s.Arm("suite-fire", "* * * * * *"); err != nil {
		t.Fatalf("Arm: %v", err)
	}

	select {
	case id := <-fired:
		if id != "suite-fire" {
			t.Errorf("want suite-fire, got %q", id)
		}
	case <-time.After(3 * time.Second):
		t.Error("cron did not fire within 3 seconds")
	}
}

func TestEvalScheduler_DisarmStopsFiring(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fired := make(chan string, 10)
	runner := &stubRunnerForSched{fired: fired}
	s := newTestSchedulerSeconds(ctx, runner)
	s.Start()

	if err := s.Arm("suite-stop", "* * * * * *"); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	// Wait for at least one fire, then disarm.
	select {
	case <-fired:
	case <-time.After(3 * time.Second):
		t.Fatal("cron did not fire before disarm")
	}

	s.Disarm("suite-stop")
	// Drain any fires that landed before Disarm took effect.
	time.Sleep(50 * time.Millisecond)
	for len(fired) > 0 {
		<-fired
	}

	// No further fires should arrive within the next second.
	select {
	case id := <-fired:
		t.Errorf("unexpected fire after Disarm: suite_id=%q", id)
	case <-time.After(1500 * time.Millisecond):
		// Expected: no fire.
	}
}
