package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

type nodeResult struct {
	nodeID string
	output map[string]any
	err    error
}

// runDAG executes a workflow DAG, returning the outputs of all sink nodes on success.
//
// Concurrency model:
//   - Root nodes (in-degree 0) are dispatched immediately as goroutines.
//   - When a node succeeds, its successors whose pending count reaches 0 are dispatched.
//   - On the first failure, the run context is cancelled and no new goroutines are started.
//   - The supervisor loop exits once every dispatched goroutine has returned a result.
func runDAG(
	ctx context.Context,
	runID string,
	dag *DAG,
	initData map[string]any,
	registry *node.NodeRegistry,
	bus *EventBus,
) (map[string]map[string]any, error) {
	if len(dag.Nodes) == 0 {
		return map[string]map[string]any{}, nil
	}

	execCtx := newExecutionContext()
	execCtx.set("_initial", initData)

	// pending[id] counts how many predecessors have not yet completed.
	pending := make(map[string]int, len(dag.Nodes))
	for id := range dag.Nodes {
		pending[id] = len(dag.Predecessors[id])
	}

	// resultCh is sized to total nodes — goroutines never block on send.
	resultCh := make(chan nodeResult, len(dag.Nodes))

	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	dispatched := 0
	received := 0
	var firstErr error

	// Dispatch all root nodes.
	for id, count := range pending {
		if count == 0 {
			dispatched++
			bus.Publish(NodeEvent{RunID: runID, NodeID: id, Type: EventNodePending, Timestamp: time.Now().UTC()})
			go executeNode(innerCtx, id, dag, execCtx, registry, bus, runID, resultCh)
		}
	}

	for received < dispatched {
		result := <-resultCh
		received++

		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
				cancel()
			}
			// Do not dispatch successors; wait for remaining goroutines to drain.
			continue
		}

		execCtx.set(result.nodeID, result.output)

		if firstErr != nil {
			continue // failure already recorded; skip scheduling successors
		}

		for _, succ := range dag.Successors[result.nodeID] {
			pending[succ]--
			if pending[succ] == 0 {
				dispatched++
				bus.Publish(NodeEvent{RunID: runID, NodeID: succ, Type: EventNodePending, Timestamp: time.Now().UTC()})
				go executeNode(innerCtx, succ, dag, execCtx, registry, bus, runID, resultCh)
			}
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return execCtx.sinkOutputs(dag), nil
}

// executeNode runs a single node inside a goroutine, respecting retry policy.
func executeNode(
	ctx context.Context,
	nodeID string,
	dag *DAG,
	execCtx *ExecutionContext,
	registry *node.NodeRegistry,
	bus *EventBus,
	runID string,
	resultCh chan<- nodeResult,
) {
	// Recover from panics in NodeHandler.Execute so the supervisor loop is
	// always unblocked and the run terminates with a failure instead of hanging.
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("panic: %v", r)
			bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeFailed, Error: msg, Timestamp: time.Now().UTC()})
			resultCh <- nodeResult{nodeID: nodeID, err: fmt.Errorf("node %s panicked: %v", nodeID, r)}
		}
	}()

	bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeRunning, Timestamp: time.Now().UTC()})

	n := dag.Nodes[nodeID]

	handler, err := registry.Lookup(n.TypeID)
	if err != nil {
		bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeFailed, Error: err.Error(), Timestamp: time.Now().UTC()})
		resultCh <- nodeResult{nodeID: nodeID, err: err}
		return
	}

	input := node.NodeInput{
		Config:       n.Config,
		UpstreamData: execCtx.mergeUpstream(dag.Predecessors[nodeID]),
	}

	out, execErr := executeWithRetry(ctx, n, input, handler)
	if execErr != nil {
		bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeFailed, Error: execErr.Error(), Timestamp: time.Now().UTC()})
		resultCh <- nodeResult{nodeID: nodeID, err: execErr}
		return
	}

	bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeSucceeded, Output: out.Data, Timestamp: time.Now().UTC()})
	resultCh <- nodeResult{nodeID: nodeID, output: out.Data}
}

// executeWithRetry calls handler.Execute, retrying up to MaxRetries times with linear backoff.
func executeWithRetry(ctx context.Context, n store.WorkflowNode, input node.NodeInput, handler node.NodeHandler) (node.NodeOutput, error) {
	maxRetries := 0
	backoffMs := 1000
	if n.RetryPolicy != nil {
		maxRetries = n.RetryPolicy.MaxRetries
		backoffMs = n.RetryPolicy.BackoffMs
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(backoffMs*attempt) * time.Millisecond
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return node.NodeOutput{}, ctx.Err()
			case <-t.C:
			}
		}
		out, err := handler.Execute(ctx, input)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return node.NodeOutput{}, lastErr
}
