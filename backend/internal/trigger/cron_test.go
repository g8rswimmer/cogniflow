package trigger

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestValidateCronExpr_Valid(t *testing.T) {
	valid := []string{
		"* * * * *",
		"0 * * * *",
		"0 9 * * 1-5",
		"30 4 1,15 * *",
	}
	for _, expr := range valid {
		if err := ValidateCronExpr(expr); err != nil {
			t.Errorf("expected valid, got error for %q: %v", expr, err)
		}
	}
}

func TestValidateCronExpr_Empty(t *testing.T) {
	if err := ValidateCronExpr(""); err == nil {
		t.Error("expected error for empty expr")
	}
}

func TestValidateCronExpr_Invalid(t *testing.T) {
	for _, expr := range []string{"not-a-cron", "* * * *", "99 * * * *"} {
		if err := ValidateCronExpr(expr); err == nil {
			t.Errorf("expected error for invalid expr %q", expr)
		}
	}
}

func TestCronScheduler_Add(t *testing.T) {
	cs := newCronScheduler()
	if err := cs.add("wf-1", "* * * * *", func() {}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if cs.entryCount() != 1 {
		t.Errorf("want 1 entry, got %d", cs.entryCount())
	}
}

func TestCronScheduler_InvalidExprReturnsError(t *testing.T) {
	cs := newCronScheduler()
	if err := cs.add("wf-1", "bad-expr", func() {}); err == nil {
		t.Error("expected error for invalid cron expression")
	}
	if cs.entryCount() != 0 {
		t.Errorf("want 0 entries after failed add, got %d", cs.entryCount())
	}
}

func TestCronScheduler_Remove(t *testing.T) {
	cs := newCronScheduler()
	if err := cs.add("wf-1", "* * * * *", func() {}); err != nil {
		t.Fatalf("add: %v", err)
	}
	cs.remove("wf-1")
	if cs.entryCount() != 0 {
		t.Errorf("want 0 entries after remove, got %d", cs.entryCount())
	}
}

func TestCronScheduler_RemoveNonexistent(t *testing.T) {
	cs := newCronScheduler()
	cs.remove("ghost") // must not panic
}

func TestCronScheduler_UpdateReplacesJob(t *testing.T) {
	cs := newCronScheduler()
	if err := cs.add("wf-1", "* * * * *", func() {}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := cs.add("wf-1", "0 * * * *", func() {}); err != nil {
		t.Fatalf("second add: %v", err)
	}
	if cs.entryCount() != 1 {
		t.Errorf("want 1 entry after update, got %d", cs.entryCount())
	}
}

func TestCronScheduler_FiresJob(t *testing.T) {
	// Use second-precision scheduler so the test completes in <3 s.
	cs := &cronScheduler{
		c:       cron.New(cron.WithSeconds()),
		entries: make(map[string]cron.EntryID),
	}
	cs.start()
	defer cs.stop()

	fired := make(chan struct{}, 1)
	if err := cs.add("wf-fire", "* * * * * *", func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	select {
	case <-fired:
	case <-time.After(3 * time.Second):
		t.Error("cron job did not fire within 3 seconds")
	}
}
