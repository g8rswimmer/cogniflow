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
	nodeID        string
	output        map[string]any // post-parser output; stored in ExecutionContext for downstream nodes
	routingOutput map[string]any // pre-parser output; used by branchAllows for conditional routing
	err           error
}

// runDAG executes a workflow DAG, returning the outputs of all sink nodes and
// per-node results on success. Per-node results are populated for every dispatched
// node (succeeded or failed); skipped/pruned nodes are absent from the map.
//
// Concurrency model:
//   - Root nodes (in-degree 0) are dispatched immediately as goroutines.
//   - When a node succeeds, its successors whose pending count reaches 0 are dispatched.
//   - On the first failure, the run context is cancelled and no new goroutines are started.
//   - The supervisor loop exits once every dispatched goroutine has returned a result.
//
// Loop support:
//   - A loop.controller node drives iteration of a loop body sub-graph.
//   - When the controller returns action="loop_body", the supervisor resets pending
//     counts for all body nodes and re-dispatches them, then resets the controller's
//     pending to the number of loop-back edges so it will be re-dispatched automatically
//     when all body sinks complete.
//   - When the controller returns action="exit" (or max_iterations is reached), normal
//     OutEdge routing fires the "exit" branch and execution continues as a regular DAG.
func runDAG(
	ctx context.Context,
	runID string,
	dag *DAG,
	initData map[string]any,
	registry *node.NodeRegistry,
	bus *EventBus,
	nodeMocks map[string]map[string]any,
) (map[string]map[string]any, map[string]store.NodeResult, error) {
	if len(dag.Nodes) == 0 {
		return map[string]map[string]any{}, map[string]store.NodeResult{}, nil
	}

	execCtx := newExecutionContext()
	execCtx.set("_initial", initData)
	// Pre-inject _loop_state with iteration 0 so that CEL exit_conditions that
	// reference ctx["_loop_state"]["iteration"] without an existence guard do not
	// crash on the very first controller dispatch (before any loop body has run).
	execCtx.set("_loop_state", map[string]any{"iteration": 0})

	// nodeResults collects the persisted outcome for every dispatched node.
	// Skipped/pruned nodes are never dispatched and will be absent from the map.
	nodeResults := make(map[string]store.NodeResult, len(dag.Nodes))

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

	// loopIterations tracks how many times a loop.controller has routed to its
	// loop body. Used to enforce max_iterations limits.
	loopIterations := make(map[string]int)

	// resultCh is sized to handle re-dispatches from loop iterations.
	// For looping workflows the channel buffer needs to accommodate body node
	// re-dispatches across all iterations; a generous upper bound prevents
	// goroutines from blocking on send under high parallelism.
	maxDispatches := computeMaxDispatches(dag)
	resultCh := make(chan nodeResult, maxDispatches)

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
		go executeNode(innerCtx, nodeID, dag, execCtx, registry, bus, runID, resultCh, nodeMocks)
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
			nodeResults[result.nodeID] = store.NodeResult{Status: "failed", Error: result.err.Error()}
			if firstErr == nil {
				firstErr = result.err
				cancel()
			}
			// Do not dispatch successors; wait for remaining goroutines to drain.
			continue
		}

		// For loop controllers: update node results with the latest iteration's output.
		// The last call (action=exit) is what gets persisted to the run record.
		nodeResults[result.nodeID] = store.NodeResult{Status: "succeeded", Output: result.output}
		execCtx.set(result.nodeID, result.output)

		if firstErr != nil {
			continue // failure already recorded; skip scheduling successors
		}

		// Loop controller special path: when the controller chooses to iterate,
		// reset body node pending counts and re-dispatch body roots. Skip normal
		// OutEdge routing so the "loop_body" forward edge does not double-dispatch.
		if dag.Nodes[result.nodeID].TypeID == loopControllerTypeID {
			if handleLoopIteration(result, dag, execCtx, pending, skipped, hasLive, loopIterations, dispatch) {
				continue
			}
		}

		for _, outEdge := range dag.OutEdges[result.nodeID] {
			if !branchAllows(outEdge, result.routingOutput, dag.Nodes[result.nodeID].TypeID) {
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

		// Fire loop-back edges originating from this node.
		// When a body sink completes, decrement the controller's pending so it is
		// re-dispatched once all body sinks have reported.
		for _, lbe := range dag.LoopBackEdges {
			if lbe.SourceID == result.nodeID {
				pending[lbe.TargetID]--
				if pending[lbe.TargetID] == 0 {
					dispatch(lbe.TargetID)
				}
			}
		}
	}

	if firstErr != nil {
		return nil, nodeResults, firstErr
	}
	finalOutput := execCtx.sinkOutputs(dag)
	// If conditional routing suppressed every path to every sink, the run
	// appears to succeed but produces no output, which is indistinguishable
	// from a legitimately empty-output workflow. Surface this as an error so
	// callers and the run store can distinguish the two cases.
	if len(finalOutput) == 0 && len(skipped) > 0 {
		for id := range skipped {
			if len(dag.Successors[id]) == 0 {
				return nil, nodeResults, fmt.Errorf("all sink branches were suppressed by conditional routing")
			}
		}
	}
	return finalOutput, nodeResults, nil
}

// handleLoopIteration processes a loop.controller result. If the controller
// chose to iterate (action="loop_body" and max_iterations not reached), it resets
// body node state, re-dispatches body roots, and returns true so the supervisor
// skips normal OutEdge routing. If the loop is exiting it pre-marks all body
// nodes as skipped and returns false so normal routing fires the "exit" branch.
func handleLoopIteration(
	result nodeResult,
	dag *DAG,
	execCtx *ExecutionContext,
	pending map[string]int,
	skipped map[string]bool,
	hasLive map[string]bool,
	loopIterations map[string]int,
	dispatch func(string),
) bool {
	action, _ := result.routingOutput["action"].(string)
	maxIter := maxIterFromConfig(dag.Nodes[result.nodeID].Config)

	if action == "loop_body" && loopIterations[result.nodeID] < maxIter {
		loopIterations[result.nodeID]++

		// Inject iteration state so the controller reads the correct iteration number
		// on its next Execute() call.
		execCtx.set("_loop_state", map[string]any{"iteration": loopIterations[result.nodeID]})

		// Ensure "_loop_state" is visible in the controller's mergeUpstream.
		if !sliceContains(dag.Ancestors[result.nodeID], "_loop_state") {
			dag.Ancestors[result.nodeID] = append(dag.Ancestors[result.nodeID], "_loop_state")
		}

		// Add body nodes to the controller's Ancestors so subsequent Execute() calls
		// receive the body nodes' latest outputs. Ancestors is only read by goroutines
		// dispatched after this point, so mutating it here (single-threaded supervisor)
		// is race-free.
		bodyNodes := dag.LoopBodyNodes[result.nodeID]
		for bodyID := range bodyNodes {
			if !sliceContains(dag.Ancestors[result.nodeID], bodyID) {
				dag.Ancestors[result.nodeID] = append(dag.Ancestors[result.nodeID], bodyID)
			}
		}

		// Reset pending counts for all body nodes.
		for bodyID := range bodyNodes {
			pending[bodyID] = countBodyInternalPredecessors(dag, bodyID, bodyNodes)
			delete(skipped, bodyID)
			delete(hasLive, bodyID)
		}

		// Reset pending for the controller to the number of loop-back edges targeting
		// it; when all body sinks complete those edges will re-dispatch the controller.
		loopBackCount := 0
		for _, lbe := range dag.LoopBackEdges {
			if lbe.TargetID == result.nodeID {
				loopBackCount++
			}
		}
		pending[result.nodeID] = loopBackCount

		// Dispatch root body nodes (those with no in-body predecessors).
		for bodyID := range bodyNodes {
			if pending[bodyID] == 0 {
				hasLive[bodyID] = true
				dispatch(bodyID)
			}
		}
		return true // skip normal OutEdge routing for this iteration
	}

	// Loop is exiting (action="exit" or max_iterations reached).
	// Pre-mark all body nodes as skipped so propagateSkip for the suppressed
	// "loop_body" outgoing edge does not re-dispatch them.
	for bodyID := range dag.LoopBodyNodes[result.nodeID] {
		skipped[bodyID] = true
	}
	return false
}

// branchAllows reports whether the given edge should fire given the completed
// node's output. Edges without a branch_label always fire.
//
// The routing key is selected based on sourceTypeID:
//   - loop.controller: matches the "action" field ("loop_body" or "exit").
//   - conditional.branch (new format): matches the "matched_rule" string field.
//   - conditional.branch (legacy format): matches "result" bool against "true"/"false".
//
// Gating the "action" check on sourceTypeID prevents nodes that legitimately
// emit an "action" field in their output from being misrouted by the loop path.
func branchAllows(edge store.WorkflowEdge, output map[string]any, sourceTypeID string) bool {
	if edge.BranchLabel == nil {
		return true
	}
	label := *edge.BranchLabel

	// loop.controller routing: match against "action" field only for controllers.
	if sourceTypeID == loopControllerTypeID {
		action, _ := output["action"].(string)
		return action == label
	}
	// New conditional format: string match against matched_rule.
	if mr, ok := output["matched_rule"].(string); ok {
		return mr == label
	}
	// Legacy conditional format: bool match against "true"/"false" label.
	if res, ok := output["result"].(bool); ok {
		return (label == "true") == res
	}
	return false
}

// computeMaxDispatches returns a safe upper bound for the resultCh buffer.
// For workflows with a loop controller, body nodes and the controller itself
// are dispatched once per iteration, so the total exceeds len(dag.Nodes).
func computeMaxDispatches(dag *DAG) int {
	base := len(dag.Nodes)
	for ctrlID, bodyNodes := range dag.LoopBodyNodes {
		maxIter := maxIterFromConfig(dag.Nodes[ctrlID].Config)
		// Each iteration dispatches all body nodes + 1 controller re-dispatch.
		base += maxIter * (len(bodyNodes) + 1)
	}
	if base < 1 {
		base = 1
	}
	return base
}

// maxIterFromConfig reads max_iterations from a loop.controller config, defaulting to 10.
func maxIterFromConfig(config map[string]any) int {
	if v, ok := config["max_iterations"]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 10
}

// countBodyInternalPredecessors counts predecessors of bodyID that are also
// inside the loop body (i.e., bodyNodes). Used to reset pending counts when
// re-dispatching the loop body for a new iteration.
func countBodyInternalPredecessors(dag *DAG, bodyID string, bodyNodes map[string]bool) int {
	count := 0
	for _, pred := range dag.Predecessors[bodyID] {
		if bodyNodes[pred] {
			count++
		}
	}
	return count
}

// sliceContains reports whether s is present in the slice.
func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// executeNode runs a single node inside a goroutine, respecting retry policy.
// If nodeMocks[nodeID] exists the real Execute() is bypassed and the mock output is used instead.
func executeNode(
	ctx context.Context,
	nodeID string,
	dag *DAG,
	execCtx *ExecutionContext,
	registry *node.NodeRegistry,
	bus *EventBus,
	runID string,
	resultCh chan<- nodeResult,
	nodeMocks map[string]map[string]any,
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

	// Mock interception: if a canned output was provided for this node, skip Execute()
	// and emit the mock output. Output parsers are intentionally NOT applied to mock outputs
	// (MK-03: mocks are used exactly as supplied for deterministic eval behaviour).
	if mockOut, ok := nodeMocks[nodeID]; ok {
		slog.Debug("node mocked", "run_id", runID, "node_id", nodeID)
		bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeRunning, Timestamp: time.Now().UTC()})
		out := make(map[string]any, len(mockOut)+1)
		for k, v := range mockOut {
			out[k] = v
		}
		out["mocked"] = true
		bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeSucceeded, Output: out, Timestamp: time.Now().UTC()})
		resultCh <- nodeResult{nodeID: nodeID, output: out, routingOutput: out}
		return
	}

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
	// Keep the pre-parser output separately so conditional routing (branchAllows)
	// is not affected by a parser whose name happens to collide with "matched_rule"
	// or "result" — the keys the conditional handler uses for routing.
	routingOutput := out.Data
	outData := outputparser.Apply(out.Data, n.OutputParsers)

	slog.Debug("node succeeded",
		"run_id", runID,
		"node_id", nodeID,
		"type_id", n.TypeID,
		"duration_ms", time.Since(nodeStart).Milliseconds(),
		"output_keys", mapKeys(outData),
	)
	bus.Publish(NodeEvent{RunID: runID, NodeID: nodeID, Type: EventNodeSucceeded, Output: outData, Timestamp: time.Now().UTC()})
	resultCh <- nodeResult{nodeID: nodeID, output: outData, routingOutput: routingOutput}
}

// newTimer creates a timer that fires after d. Overridable in tests to avoid real sleeps.
var newTimer = time.NewTimer

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
			t := newTimer(delay)
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
