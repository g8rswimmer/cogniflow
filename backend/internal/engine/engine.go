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
	// Drain the events channel so its buffer never fills and blocks Publish for other subscribers.
	go func() { for range handle.Events {} }()
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

	slog.Info("run started",
		"run_id", run.ID,
		"workflow_id", wf.ID,
		"workflow_name", wf.Name,
		"triggered_by", req.TriggeredBy,
		"node_count", len(dag.Nodes),
		"timeout_s", int(timeout.Seconds()),
	)

	go func() {
		defer cancel()
		defer cleanup()

		// Subscribe internally before runDAG so no node events are missed.
		// The drain goroutine collects succeeded/failed outcomes for persistence.
		internalCh, internalCleanup := e.bus.Subscribe(run.ID)
		nodeResults := make(map[string]store.NodeResult)
		drainDone := make(chan struct{})
		go func() {
			defer close(drainDone)
			for event := range internalCh {
				switch event.Type {
				case EventNodeSucceeded:
					nodeResults[event.NodeID] = store.NodeResult{Status: "succeeded", Output: event.Output}
				case EventNodeFailed:
					nodeResults[event.NodeID] = store.NodeResult{Status: "failed", Error: event.Error}
				}
			}
		}()

		start := time.Now()
		finalOutput, runErr := runDAG(runCtx, run.ID, dag, req.InitialData, e.registry, e.bus, req.NodeMocks)
		durationMs := time.Since(start).Milliseconds()

		var statusUpdate store.RunStatus
		var outputMap map[string]any

		if runErr != nil {
			statusUpdate = store.RunStatusFailed
			outputMap = map[string]any{"error": runErr.Error()}
			slog.Error("run failed",
				"run_id", run.ID,
				"workflow_id", wf.ID,
				"workflow_name", wf.Name,
				"duration_ms", durationMs,
				"error", runErr,
			)
			e.bus.Publish(NodeEvent{RunID: run.ID, Type: EventRunFailed, Error: runErr.Error(), Timestamp: time.Now().UTC()})
		} else {
			statusUpdate = store.RunStatusSucceeded
			outputMap = flattenOutput(finalOutput)
			slog.Info("run succeeded",
				"run_id", run.ID,
				"workflow_id", wf.ID,
				"workflow_name", wf.Name,
				"duration_ms", durationMs,
				"sink_nodes", len(finalOutput),
			)
			e.bus.Publish(NodeEvent{RunID: run.ID, Type: EventRunSucceeded, Timestamp: time.Now().UTC()})
		}

		if err := e.store.UpdateRunStatus(context.Background(), run.ID, statusUpdate, outputMap); err != nil {
			slog.Error("engine: update run status", "run_id", run.ID, "error", err)
		}

		// Close internal subscription and wait for the drain goroutine to finish
		// processing any remaining buffered events before persisting node results.
		internalCleanup()
		<-drainDone
		if err := e.store.SaveRunNodeResults(context.Background(), run.ID, nodeResults); err != nil {
			slog.Error("engine: save node results", "run_id", run.ID, "error", err)
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
