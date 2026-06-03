package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/outputparser"
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

	// skipped tracks nodes whose pending count reached zero entirely through
	// suppressed (branch-labelled) edges. Skipped nodes never execute.
	skipped := make(map[string]bool)

	// hasLive[id] is true once at least one live (non-suppressed) predecessor
	// has decremented pending[id]. It prevents propagateSkip from marking a
	// node skipped when pending reaches 0 via the last suppressed edge but a
	// live predecessor had already contributed to that count.
	hasLive := make(map[string]bool)

	// resultCh is sized to total nodes — goroutines never block on send.
	resultCh := make(chan nodeResult, len(dag.Nodes))

	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	dispatched := 0
	received := 0
	var firstErr error

	slog.Debug("dag starting", "run_id", runID, "node_count", len(dag.Nodes))

	// dispatch fires nodeID as a goroutine and increments the dispatched counter.
	dispatch := func(nodeID string) {
		dispatched++
		slog.Debug("node dispatched", "run_id", runID, "node_id", nodeID, "type_id", dag.Nodes[nodeID].TypeID)
		bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodePending, Timestamp: time.Now().UTC()})
		go executeNode(innerCtx, nodeID, dag, execCtx, registry, bus, runID, resultCh)
	}

	// propagateSkip decrements pending for nodeID without dispatching it.
	// When pending reaches zero through suppressed edges only (hasLive is false),
	// the node is marked skipped and propagation continues to its successors.
	// When pending reaches zero but a live predecessor already fired (hasLive is
	// true), the node is dispatched so it can receive that live predecessor's output.
	// The supervisor loop is the sole caller, so all map accesses are single-threaded.
	var propagateSkip func(nodeID string)
	propagateSkip = func(nodeID string) {
		pending[nodeID]--
		if pending[nodeID] > 0 || skipped[nodeID] {
			return // still waiting on other predecessors, or already handled
		}
		if hasLive[nodeID] {
			// At least one live predecessor completed before all suppressed ones;
			// this node should run with whatever live data it received.
			dispatch(nodeID)
			return
		}
		skipped[nodeID] = true
		for _, edge := range dag.OutEdges[nodeID] {
			propagateSkip(edge.TargetID)
		}
	}

	// Dispatch all root nodes.
	for id, count := range pending {
		if count == 0 {
			dispatch(id)
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

		for _, outEdge := range dag.OutEdges[result.nodeID] {
			if !branchAllows(outEdge, result.output) {
				// Suppressed edge: account for this predecessor without dispatching.
				propagateSkip(outEdge.TargetID)
				continue
			}
			succ := outEdge.TargetID
			hasLive[succ] = true // record that a live predecessor contributed
			pending[succ]--
			if pending[succ] == 0 {
				dispatch(succ)
			}
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	finalOutput := execCtx.sinkOutputs(dag)
	// If conditional routing suppressed every path to every sink, the run
	// appears to succeed but produces no output, which is indistinguishable
	// from a legitimately empty-output workflow. Surface this as an error so
	// callers and the run store can distinguish the two cases.
	if len(finalOutput) == 0 && len(skipped) > 0 {
		for id := range skipped {
			if len(dag.Successors[id]) == 0 {
				return nil, fmt.Errorf("all sink branches were suppressed by conditional routing")
			}
		}
	}
	return finalOutput, nil
}

// branchAllows reports whether the given edge should fire given the completed
// node's output. Edges without a branch_label always fire. Edges labelled
// "true" or "false" fire only when the output "result" bool matches.
func branchAllows(edge store.WorkflowEdge, output map[string]any) bool {
	if edge.BranchLabel == nil {
		return true
	}
	res, ok := output["result"].(bool)
	if !ok {
		return false
	}
	return (*edge.BranchLabel == "true") == res
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
			slog.Error("node panicked", "run_id", runID, "node_id", nodeID, "panic", r)
			bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeFailed, Error: msg, Timestamp: time.Now().UTC()})
			resultCh <- nodeResult{nodeID: nodeID, err: fmt.Errorf("node %s panicked: %v", nodeID, r)}
		}
	}()

	n := dag.Nodes[nodeID]

	slog.Debug("node executing", "run_id", runID, "node_id", nodeID, "type_id", n.TypeID)
	bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeRunning, Timestamp: time.Now().UTC()})

	handler, err := registry.Lookup(n.TypeID)
	if err != nil {
		slog.Error("node handler not found", "run_id", runID, "node_id", nodeID, "type_id", n.TypeID, "error", err)
		bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeFailed, Error: err.Error(), Timestamp: time.Now().UTC()})
		resultCh <- nodeResult{nodeID: nodeID, err: err}
		return
	}

	input := node.NodeInput{
		Config:               n.Config,
		UpstreamData:         execCtx.mergeUpstream(dag.Ancestors[nodeID]),
		DirectPredecessorIDs: dag.Predecessors[nodeID],
	}

	nodeStart := time.Now()
	out, execErr := executeWithRetry(ctx, runID, nodeID, n, input, handler)
	if execErr != nil {
		slog.Debug("node failed",
			"run_id", runID,
			"node_id", nodeID,
			"type_id", n.TypeID,
			"duration_ms", time.Since(nodeStart).Milliseconds(),
			"error", execErr,
		)
		bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeFailed, Error: execErr.Error(), Timestamp: time.Now().UTC()})
		resultCh <- nodeResult{nodeID: nodeID, err: execErr}
		return
	}

	// Apply output parsers defined on the node to extract named fields from
	// the raw output (e.g. regex or JSON path over an LLM completion).
	outData := outputparser.Apply(out.Data, n.OutputParsers)

	slog.Debug("node succeeded",
		"run_id", runID,
		"node_id", nodeID,
		"type_id", n.TypeID,
		"duration_ms", time.Since(nodeStart).Milliseconds(),
		"output_keys", mapKeys(outData),
	)
	bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeSucceeded, Output: outData, Timestamp: time.Now().UTC()})
	resultCh <- nodeResult{nodeID: nodeID, output: outData}
}

// executeWithRetry calls handler.Execute, retrying up to MaxRetries times with linear backoff.
func executeWithRetry(ctx context.Context, runID, nodeID string, n store.WorkflowNode, input node.NodeInput, handler node.NodeHandler) (node.NodeOutput, error) {
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
			slog.Warn("node retrying",
				"run_id", runID,
				"node_id", nodeID,
				"type_id", n.TypeID,
				"attempt", attempt,
				"max_retries", maxRetries,
				"backoff_ms", delay.Milliseconds(),
				"last_error", lastErr,
			)
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

// mapKeys returns a sorted slice of keys from a map — used for structured logging.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
