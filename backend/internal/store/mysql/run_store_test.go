package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- CreateRun --------------------------------------------------------------

func TestRunStore_Create_GeneratesID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	got, err := s.CreateRun(ctx, store.Run{
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if got.ID == "" {
		t.Error("expected a generated run ID")
	}
	if got.WorkflowID != "wf-1" {
		t.Errorf("workflow_id: want wf-1, got %q", got.WorkflowID)
	}
	if got.Status != store.RunStatusRunning {
		t.Errorf("status: want running, got %q", got.Status)
	}
}

func TestRunStore_Create_PreservesProvidedID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	got, err := s.CreateRun(ctx, store.Run{
		ID:          "run-abc",
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if got.ID != "run-abc" {
		t.Errorf("id: want run-abc, got %q", got.ID)
	}
}

func TestRunStore_Create_WorkflowNotFound_ReturnsError(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreateRun(context.Background(), store.Run{
		WorkflowID:  "no-such-workflow",
		TriggeredBy: "manual",
		Status:      store.RunStatusRunning,
	})
	if err == nil {
		t.Fatal("expected error for non-existent workflow")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---- GetRun -----------------------------------------------------------------

func TestRunStore_Get_Found(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	created, err := s.CreateRun(ctx, store.Run{
		WorkflowID:  "wf-1",
		TriggeredBy: "webhook",
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := s.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id: want %q, got %q", created.ID, got.ID)
	}
	if got.TriggeredBy != "webhook" {
		t.Errorf("triggered_by: want webhook, got %q", got.TriggeredBy)
	}
	if got.Status != store.RunStatusRunning {
		t.Errorf("status: want running, got %q", got.Status)
	}
	if got.StartedAt == nil {
		t.Error("started_at should not be nil")
	} else {
		withinSecond(t, "started_at", now, *got.StartedAt)
	}
}

func TestRunStore_Get_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetRun(context.Background(), "no-such-run")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---- UpdateRunStatus --------------------------------------------------------

func TestRunStore_UpdateStatus_Succeeded(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	created, err := s.CreateRun(ctx, store.Run{
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	output := map[string]any{"n1": map[string]any{"status_code": float64(200)}}
	if err := s.UpdateRunStatus(ctx, created.ID, store.RunStatusSucceeded, output); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}

	got, err := s.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != store.RunStatusSucceeded {
		t.Errorf("status: want succeeded, got %q", got.Status)
	}
	if got.FinishedAt == nil {
		t.Error("finished_at should be set after completion")
	}
	if got.FinalOutput == nil {
		t.Fatal("final_output should not be nil")
	}
	n1, ok := got.FinalOutput["n1"].(map[string]any)
	if !ok {
		t.Fatalf("final_output.n1: want map, got %T", got.FinalOutput["n1"])
	}
	if n1["status_code"] != float64(200) {
		t.Errorf("status_code: want 200, got %v", n1["status_code"])
	}
}

func TestRunStore_UpdateStatus_Failed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	created, err := s.CreateRun(ctx, store.Run{
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	errDetail := map[string]any{"error": "node timed out"}
	if err := s.UpdateRunStatus(ctx, created.ID, store.RunStatusFailed, errDetail); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}

	got, err := s.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != store.RunStatusFailed {
		t.Errorf("status: want failed, got %q", got.Status)
	}
	if got.ErrorDetail == nil {
		t.Fatal("error_detail should not be nil")
	}
	if got.ErrorDetail["error"] != "node timed out" {
		t.Errorf("error: want %q, got %v", "node timed out", got.ErrorDetail["error"])
	}
	if got.FinalOutput != nil {
		t.Errorf("final_output should be nil for failed run, got %v", got.FinalOutput)
	}
}

func TestRunStore_UpdateStatus_NilOutput(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	created, err := s.CreateRun(ctx, store.Run{
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Nil output should not error.
	if err := s.UpdateRunStatus(ctx, created.ID, store.RunStatusSucceeded, nil); err != nil {
		t.Fatalf("UpdateRunStatus with nil output: %v", err)
	}

	got, err := s.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != store.RunStatusSucceeded {
		t.Errorf("status: want succeeded, got %q", got.Status)
	}
}

// ---- ListRuns ---------------------------------------------------------------

func TestRunStore_ListRuns_Empty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.ListRuns(context.Background(), store.RunFilter{WorkflowID: "wf-none"})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty list, got %d", len(got))
	}
}

func TestRunStore_ListRuns_ByWorkflow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-a")
	insertTestWorkflow(t, s, "wf-b")

	now := time.Now().UTC()
	for _, wf := range []string{"wf-a", "wf-a", "wf-b"} {
		if _, err := s.CreateRun(ctx, store.Run{
			WorkflowID:  wf,
			TriggeredBy: "manual",
			Status:      store.RunStatusRunning,
			StartedAt:   &now,
		}); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	got, err := s.ListRuns(ctx, store.RunFilter{WorkflowID: "wf-a"})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 runs for wf-a, got %d", len(got))
	}
	for _, r := range got {
		if r.WorkflowID != "wf-a" {
			t.Errorf("unexpected workflow_id %q in result", r.WorkflowID)
		}
	}
}

func TestRunStore_ListRuns_ByStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	statuses := []store.RunStatus{
		store.RunStatusRunning,
		store.RunStatusSucceeded,
		store.RunStatusSucceeded,
	}
	for _, st := range statuses {
		if _, err := s.CreateRun(ctx, store.Run{
			WorkflowID:  "wf-1",
			TriggeredBy: "manual",
			Status:      st,
			StartedAt:   &now,
		}); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	got, err := s.ListRuns(ctx, store.RunFilter{
		WorkflowID: "wf-1",
		Status:     store.RunStatusSucceeded,
	})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 succeeded runs, got %d", len(got))
	}
}

func TestRunStore_ListRuns_Limit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if _, err := s.CreateRun(ctx, store.Run{
			WorkflowID:  "wf-1",
			TriggeredBy: "manual",
			Status:      store.RunStatusSucceeded,
			StartedAt:   &now,
		}); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	got, err := s.ListRuns(ctx, store.RunFilter{WorkflowID: "wf-1", Limit: 3})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 runs (limit), got %d", len(got))
	}
}

func TestRunStore_ListRuns_SinceUntilFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	base := time.Now().UTC().Truncate(time.Second)
	old := base.Add(-2 * time.Hour)
	recent := base.Add(-30 * time.Minute)
	future := base.Add(2 * time.Hour)

	for _, ts := range []*time.Time{&old, &recent, &future} {
		if _, err := s.CreateRun(ctx, store.Run{
			WorkflowID:  "wf-1",
			TriggeredBy: "manual",
			Status:      store.RunStatusSucceeded,
			StartedAt:   ts,
		}); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	got, err := s.ListRuns(ctx, store.RunFilter{
		WorkflowID: "wf-1",
		Since:      base.Add(-time.Hour),
		Until:      base.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 run in window, got %d", len(got))
	}
	if got[0].StartedAt == nil || !got[0].StartedAt.Truncate(time.Second).Equal(recent) {
		t.Errorf("expected recent run, got started_at=%v", got[0].StartedAt)
	}
}

// ---- SaveRunNodeResults -----------------------------------------------------

func TestRunStore_SaveRunNodeResults_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	created, err := s.CreateRun(ctx, store.Run{
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	results := map[string]store.NodeResult{
		"n1": {Status: "succeeded", Output: map[string]any{"status_code": float64(200)}},
		"n2": {Status: "failed", Error: "connection refused"},
	}
	if err := s.SaveRunNodeResults(ctx, created.ID, results); err != nil {
		t.Fatalf("SaveRunNodeResults: %v", err)
	}

	got, err := s.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if len(got.NodeResults) != 2 {
		t.Fatalf("node_results: want 2 entries, got %d", len(got.NodeResults))
	}

	n1 := got.NodeResults["n1"]
	if n1.Status != "succeeded" {
		t.Errorf("n1.status: want succeeded, got %q", n1.Status)
	}
	if n1.Output["status_code"] != float64(200) {
		t.Errorf("n1.output.status_code: want 200, got %v", n1.Output["status_code"])
	}

	n2 := got.NodeResults["n2"]
	if n2.Status != "failed" {
		t.Errorf("n2.status: want failed, got %q", n2.Status)
	}
	if n2.Error != "connection refused" {
		t.Errorf("n2.error: want %q, got %q", "connection refused", n2.Error)
	}
}

func TestRunStore_SaveRunNodeResults_NotInListRuns(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	now := time.Now().UTC()
	created, err := s.CreateRun(ctx, store.Run{
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusSucceeded,
		StartedAt:   &now,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := s.SaveRunNodeResults(ctx, created.ID, map[string]store.NodeResult{
		"n1": {Status: "succeeded"},
	}); err != nil {
		t.Fatalf("SaveRunNodeResults: %v", err)
	}

	// ListRuns omits node_results for efficiency; NodeResults should be nil.
	list, err := s.ListRuns(ctx, store.RunFilter{WorkflowID: "wf-1"})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 run, got %d", len(list))
	}
	if list[0].NodeResults != nil {
		t.Errorf("ListRuns should not populate NodeResults, got %v", list[0].NodeResults)
	}
}

func TestRunStore_ListRuns_OrderedByStartedAtDesc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertTestWorkflow(t, s, "wf-1")

	base := time.Now().UTC().Truncate(time.Second)
	times := []time.Time{
		base.Add(-3 * time.Hour),
		base.Add(-1 * time.Hour),
		base.Add(-2 * time.Hour),
	}
	for _, ts := range times {
		ts := ts
		if _, err := s.CreateRun(ctx, store.Run{
			WorkflowID:  "wf-1",
			TriggeredBy: "manual",
			Status:      store.RunStatusSucceeded,
			StartedAt:   &ts,
		}); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	got, err := s.ListRuns(ctx, store.RunFilter{WorkflowID: "wf-1"})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 runs, got %d", len(got))
	}
	// Verify descending order: each started_at should be >= the next one.
	for i := 1; i < len(got); i++ {
		if got[i-1].StartedAt == nil || got[i].StartedAt == nil {
			t.Fatal("started_at should not be nil")
		}
		if got[i-1].StartedAt.Before(*got[i].StartedAt) {
			t.Errorf("run[%d].started_at (%v) is before run[%d].started_at (%v); want DESC order",
				i-1, got[i-1].StartedAt, i, got[i].StartedAt)
		}
	}
}
