package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// RunHandle is returned by Run. It lets the caller observe or cancel an active run.
type RunHandle struct {
	RunID  string
	Events <-chan NodeEvent // closed when the run terminates
	Cancel context.CancelFunc
}

// WorkflowEngine orchestrates asynchronous workflow execution.
type WorkflowEngine struct {
	store    store.Store
	registry *node.NodeRegistry
	bus      *EventBus
}

// NewWorkflowEngine creates a WorkflowEngine.
func NewWorkflowEngine(st store.Store, registry *node.NodeRegistry, bus *EventBus) *WorkflowEngine {
	return &WorkflowEngine{store: st, registry: registry, bus: bus}
}

// Dispatch implements trigger.Dispatcher. It starts a run asynchronously and returns the run ID.
func (e *WorkflowEngine) Dispatch(ctx context.Context, req trigger.RunRequest) (string, error) {
	handle, err := e.Run(ctx, req)
	if err != nil {
		return "", err
	}
	return handle.RunID, nil
}

// Run fetches the workflow, creates a run record, starts execution in a background goroutine,
// and returns a RunHandle immediately.
func (e *WorkflowEngine) Run(ctx context.Context, req trigger.RunRequest) (RunHandle, error) {
	wf, err := e.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return RunHandle{}, fmt.Errorf("engine: get workflow: %w", err)
	}

	dag, err := Build(wf.Nodes, wf.Edges)
	if err != nil {
		return RunHandle{}, fmt.Errorf("engine: build dag: %w", err)
	}

	now := time.Now().UTC()
	run, err := e.store.CreateRun(ctx, store.Run{
		WorkflowID:  wf.ID,
		TriggeredBy: req.TriggeredBy,
		Status:      store.RunStatusRunning,
		StartedAt:   &now,
	})
	if err != nil {
		return RunHandle{}, fmt.Errorf("engine: create run: %w", err)
	}

	timeout := time.Duration(wf.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(context.Background(), timeout)

	events, cleanup := e.bus.Subscribe(run.ID)

	go func() {
		defer cancel()
		defer cleanup()

		finalOutput, runErr := runDAG(runCtx, run.ID, dag, req.InitialData, e.registry, e.bus)

		var statusUpdate store.RunStatus
		var outputMap map[string]any

		if runErr != nil {
			statusUpdate = store.RunStatusFailed
			outputMap = map[string]any{"error": runErr.Error()}
			e.bus.Publish(NodeEvent{RunID: run.ID, Type: EventRunFailed, Error: runErr.Error(), Timestamp: time.Now().UTC()})
		} else {
			statusUpdate = store.RunStatusSucceeded
			outputMap = flattenOutput(finalOutput)
			e.bus.Publish(NodeEvent{RunID: run.ID, Type: EventRunSucceeded, Timestamp: time.Now().UTC()})
		}

		if err := e.store.UpdateRunStatus(context.Background(), run.ID, statusUpdate, outputMap); err != nil {
			slog.Error("engine: update run status", "run_id", run.ID, "error", err)
		}
	}()

	return RunHandle{RunID: run.ID, Events: events, Cancel: cancel}, nil
}

// flattenOutput converts map[string]map[string]any to map[string]any for JSON storage.
// Direct assignment preserves original Go types (e.g. int stays int, not float64).
func flattenOutput(m map[string]map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
