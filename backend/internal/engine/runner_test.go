package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- test NodeHandler implementations ------------------------------------

// fixedHandler returns a predetermined output map.
type fixedHandler struct {
	meta   node.NodeMeta
	output map[string]any
}

func (h *fixedHandler) Meta() node.NodeMeta { return h.meta }
func (h *fixedHandler) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{Data: h.output}, nil
}

// failHandler always returns an error.
type failHandler struct{ meta node.NodeMeta }

func (h *failHandler) Meta() node.NodeMeta { return h.meta }
func (h *failHandler) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{}, errors.New("intentional failure")
}

// countingHandler records how many times Execute is called.
type countingHandler struct {
	meta  node.NodeMeta
	calls atomic.Int32
}

func (h *countingHandler) Meta() node.NodeMeta { return h.meta }
func (h *countingHandler) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	h.calls.Add(1)
	return node.NodeOutput{Data: map[string]any{"called": true}}, nil
}

// funcHandler delegates to an arbitrary function; useful for closure-based tests.
type funcHandler struct {
	meta   node.NodeMeta
	execFn func(context.Context, node.NodeInput) (node.NodeOutput, error)
}

func (h *funcHandler) Meta() node.NodeMeta { return h.meta }
func (h *funcHandler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	return h.execFn(ctx, input)
}

// ---- helpers -------------------------------------------------------------

func newTestRegistry(handlers ...node.NodeHandler) *node.NodeRegistry {
	r := node.NewRegistry()
	for _, h := range handlers {
		r.Register(h)
	}
	return r
}

func newMeta(typeID string) node.NodeMeta {
	return node.NodeMeta{
		TypeID: typeID, DisplayName: typeID, Category: "test",
		Description: "test", InputSchema: []byte("{}"), OutputSchema: []byte("{}"),
	}
}

func makeNode(id, typeID string) store.WorkflowNode {
	return store.WorkflowNode{ID: id, TypeID: typeID}
}

func makeEdge(id, src, tgt string) store.WorkflowEdge {
	return store.WorkflowEdge{ID: id, SourceID: src, TargetID: tgt}
}

func makeBranchEdge(id, src, tgt, label string) store.WorkflowEdge {
	return store.WorkflowEdge{ID: id, SourceID: src, TargetID: tgt, BranchLabel: &label}
}

// ---- tests ---------------------------------------------------------------

func TestRunDAG_SingleNode(t *testing.T) {
	registry := newTestRegistry(&fixedHandler{
		meta:   newMeta("fixed"),
		output: map[string]any{"result": "done"},
	})

	dag, _ := Build([]store.WorkflowNode{makeNode("n1", "fixed")}, nil)

	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["n1"]["result"] != "done" {
		t.Errorf("expected result=done, got %v", out["n1"])
	}
}

func TestRunDAG_Sequential(t *testing.T) {
	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("a"), output: map[string]any{"step": "a"}},
		&fixedHandler{meta: newMeta("b"), output: map[string]any{"step": "b"}},
	)

	dag, _ := Build(
		[]store.WorkflowNode{makeNode("n1", "a"), makeNode("n2", "b")},
		[]store.WorkflowEdge{makeEdge("e1", "n1", "n2")},
	)

	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Sink is n2.
	if out["n2"]["step"] != "b" {
		t.Errorf("expected n2.step=b, got %v", out["n2"])
	}
	if _, hasN1 := out["n1"]; hasN1 {
		t.Error("n1 is not a sink; should not appear in final output")
	}
}

func TestRunDAG_Parallel(t *testing.T) {
	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("root"), output: map[string]any{"root": true}},
		&fixedHandler{meta: newMeta("branch"), output: map[string]any{"branch": true}},
	)

	// n1 fans out to n2 and n3 (both sinks).
	dag, _ := Build(
		[]store.WorkflowNode{makeNode("n1", "root"), makeNode("n2", "branch"), makeNode("n3", "branch")},
		[]store.WorkflowEdge{makeEdge("e1", "n1", "n2"), makeEdge("e2", "n1", "n3")},
	)

	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := out["n2"]; !ok {
		t.Error("expected n2 in output")
	}
	if _, ok := out["n3"]; !ok {
		t.Error("expected n3 in output")
	}
}

func TestRunDAG_FailurePropagates(t *testing.T) {
	downstream := &countingHandler{meta: newMeta("after")}
	registry := newTestRegistry(
		&failHandler{meta: newMeta("fail")},
		downstream,
	)

	dag, _ := Build(
		[]store.WorkflowNode{makeNode("n1", "fail"), makeNode("n2", "after")},
		[]store.WorkflowEdge{makeEdge("e1", "n1", "n2")},
	)

	bus := NewEventBus()
	_, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err == nil {
		t.Fatal("expected error from failed node")
	}
	if downstream.calls.Load() != 0 {
		t.Errorf("downstream node should not have executed, got %d calls", downstream.calls.Load())
	}
}

func TestRunDAG_EmptyWorkflow(t *testing.T) {
	dag := &DAG{
		Nodes:        map[string]store.WorkflowNode{},
		Successors:   map[string][]string{},
		Predecessors: map[string][]string{},
	}
	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, node.NewRegistry(), bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %v", out)
	}
}

func TestRunDAG_InitialDataAvailable(t *testing.T) {
	var capturedUpstream map[string]any

	registry := newTestRegistry(&funcHandler{
		meta: newMeta("capture"),
		execFn: func(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
			capturedUpstream = input.UpstreamData
			return node.NodeOutput{Data: map[string]any{}}, nil
		},
	})

	dag, _ := Build([]store.WorkflowNode{makeNode("n1", "capture")}, nil)
	bus := NewEventBus()
	_, _, err := runDAG(context.Background(), "run-1", dag, map[string]any{"msg": "hello"}, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	init, ok := capturedUpstream["_initial"].(map[string]any)
	if !ok {
		t.Fatalf("expected _initial in upstream, got %T", capturedUpstream["_initial"])
	}
	if init["msg"] != "hello" {
		t.Errorf("expected msg=hello, got %v", init["msg"])
	}
}

func TestRunDAG_ContextCancellation(t *testing.T) {
	registry := newTestRegistry(&funcHandler{
		meta: newMeta("slow"),
		execFn: func(ctx context.Context, _ node.NodeInput) (node.NodeOutput, error) {
			select {
			case <-ctx.Done():
				return node.NodeOutput{}, ctx.Err()
			case <-time.After(5 * time.Second):
				return node.NodeOutput{Data: map[string]any{}}, nil
			}
		},
	})

	dag, _ := Build([]store.WorkflowNode{makeNode("n1", "slow")}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	bus := NewEventBus()
	_, _, err := runDAG(ctx, "run-1", dag, nil, registry, bus, nil)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestRetryPolicy(t *testing.T) {
	calls := 0
	handler := &funcHandler{
		meta: newMeta("flaky"),
		execFn: func(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
			calls++
			if calls < 3 {
				return node.NodeOutput{}, errors.New("transient")
			}
			return node.NodeOutput{Data: map[string]any{"ok": true}}, nil
		},
	}
	registry := newTestRegistry(handler)

	n := store.WorkflowNode{
		ID: "n1", TypeID: "flaky",
		RetryPolicy: &store.RetryPolicy{MaxRetries: 3, BackoffMs: 1},
	}
	dag, _ := Build([]store.WorkflowNode{n}, nil)

	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["n1"]["ok"] != true {
		t.Errorf("expected ok=true, got %v", out["n1"])
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRunDAG_NodePanic_ReturnsError(t *testing.T) {
	// A NodeHandler that panics must not hang the supervisor loop.
	registry := newTestRegistry(&funcHandler{
		meta: newMeta("panicky"),
		execFn: func(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
			panic("something went very wrong")
		},
	})

	dag, _ := Build([]store.WorkflowNode{makeNode("n1", "panicky")}, nil)

	bus := NewEventBus()
	_, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err == nil {
		t.Fatal("expected error from panicking node")
	}
}

// TestRunDAG_GrandparentOutputVisible verifies that a node can reference the
// output of a non-immediate ancestor (transitive predecessor) via UpstreamData.
// With the Ancestors map, n3 should see both n1's and n2's outputs even though
// n1 → n2 → n3 and n1 is two hops away.
func TestRunDAG_GrandparentOutputVisible(t *testing.T) {
	var capturedUpstream map[string]any

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("root"), output: map[string]any{"root_key": "root_val"}},
		&fixedHandler{meta: newMeta("mid"), output: map[string]any{"mid_key": "mid_val"}},
		&funcHandler{
			meta: newMeta("leaf"),
			execFn: func(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
				capturedUpstream = input.UpstreamData
				return node.NodeOutput{Data: map[string]any{}}, nil
			},
		},
	)

	dag, _ := Build(
		[]store.WorkflowNode{makeNode("n1", "root"), makeNode("n2", "mid"), makeNode("n3", "leaf")},
		[]store.WorkflowEdge{makeEdge("e1", "n1", "n2"), makeEdge("e2", "n2", "n3")},
	)

	bus := NewEventBus()
	_, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedUpstream["n1"] == nil {
		t.Error("n3 should see n1 (grandparent) in UpstreamData, got nil")
	}
	if capturedUpstream["n2"] == nil {
		t.Error("n3 should see n2 (direct predecessor) in UpstreamData, got nil")
	}
	n1out, _ := capturedUpstream["n1"].(map[string]any)
	if n1out["root_key"] != "root_val" {
		t.Errorf("n1.root_key: want root_val, got %v", n1out["root_key"])
	}
}

// ---- conditional routing tests ----------------------------------------------

// TestRunDAG_ConditionalTrue verifies that the "true" branch fires and the
// "false" branch is suppressed when the conditional node returns result=true.
func TestRunDAG_ConditionalTrue(t *testing.T) {
	trueBranch := &countingHandler{meta: newMeta("trueBranch")}
	falseBranch := &countingHandler{meta: newMeta("falseBranch")}

	// conditional node always returns result=true
	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("cond"), output: map[string]any{"result": true}},
		trueBranch,
		falseBranch,
	)

	nodes := []store.WorkflowNode{
		makeNode("n1", "cond"),
		makeNode("n2", "trueBranch"),
		makeNode("n3", "falseBranch"),
	}
	edges := []store.WorkflowEdge{
		makeBranchEdge("e1", "n1", "n2", "true"),
		makeBranchEdge("e2", "n1", "n3", "false"),
	}
	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if trueBranch.calls.Load() != 1 {
		t.Errorf("true branch: want 1 call, got %d", trueBranch.calls.Load())
	}
	if falseBranch.calls.Load() != 0 {
		t.Errorf("false branch: want 0 calls, got %d", falseBranch.calls.Load())
	}
	if _, ok := out["n2"]; !ok {
		t.Error("n2 (true branch) should appear in output")
	}
	if _, ok := out["n3"]; ok {
		t.Error("n3 (false branch, skipped) should not appear in output")
	}
}

// TestRunDAG_ConditionalFalse verifies the "false" branch fires when result=false.
func TestRunDAG_ConditionalFalse(t *testing.T) {
	trueBranch := &countingHandler{meta: newMeta("trueBranch")}
	falseBranch := &countingHandler{meta: newMeta("falseBranch")}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("cond"), output: map[string]any{"result": false}},
		trueBranch,
		falseBranch,
	)

	nodes := []store.WorkflowNode{
		makeNode("n1", "cond"),
		makeNode("n2", "trueBranch"),
		makeNode("n3", "falseBranch"),
	}
	edges := []store.WorkflowEdge{
		makeBranchEdge("e1", "n1", "n2", "true"),
		makeBranchEdge("e2", "n1", "n3", "false"),
	}
	dag, _ := Build(nodes, edges)

	bus := NewEventBus()
	_, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if trueBranch.calls.Load() != 0 {
		t.Errorf("true branch: want 0 calls, got %d", trueBranch.calls.Load())
	}
	if falseBranch.calls.Load() != 1 {
		t.Errorf("false branch: want 1 call, got %d", falseBranch.calls.Load())
	}
}

// TestRunDAG_ConditionalMerge verifies that a merge node after a conditional
// fork executes exactly once, with data from the live branch only.
func TestRunDAG_ConditionalMerge(t *testing.T) {
	mergeHandler := &countingHandler{meta: newMeta("merge")}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("cond"), output: map[string]any{"result": true}},
		&fixedHandler{meta: newMeta("trueBranch"), output: map[string]any{"from": "true"}},
		&fixedHandler{meta: newMeta("falseBranch"), output: map[string]any{"from": "false"}},
		mergeHandler,
	)

	// n1 (cond) → n2 (true) and n3 (false); both → n4 (merge)
	nodes := []store.WorkflowNode{
		makeNode("n1", "cond"),
		makeNode("n2", "trueBranch"),
		makeNode("n3", "falseBranch"),
		makeNode("n4", "merge"),
	}
	edges := []store.WorkflowEdge{
		makeBranchEdge("e1", "n1", "n2", "true"),
		makeBranchEdge("e2", "n1", "n3", "false"),
		makeEdge("e3", "n2", "n4"),
		makeEdge("e4", "n3", "n4"),
	}
	dag, _ := Build(nodes, edges)

	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// merge fires exactly once
	if mergeHandler.calls.Load() != 1 {
		t.Errorf("merge: want 1 call, got %d", mergeHandler.calls.Load())
	}
	// n4 is the only sink
	if _, ok := out["n4"]; !ok {
		t.Error("n4 should be in the final output")
	}
}

// TestRunDAG_ConditionalMerge_MultiSource verifies that a merge node with
// predecessors from INDEPENDENT parents (not both children of the same
// conditional) is still dispatched when one predecessor is on a suppressed
// branch and the other is live.
//
// Topology:
//   n1 (regular root) ─────────────────────→ n3 (merge)
//   n2 (conditional, result=false) ─[true]─→ n3 (merge)  ← suppressed
func TestRunDAG_ConditionalMerge_MultiSource(t *testing.T) {
	mergeHandler := &countingHandler{meta: newMeta("merge")}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("regular"), output: map[string]any{"live": true}},
		&fixedHandler{meta: newMeta("cond"), output: map[string]any{"result": false}},
		mergeHandler,
	)

	nodes := []store.WorkflowNode{
		makeNode("n1", "regular"),                   // root: live predecessor of n3
		makeNode("n2", "cond"),                      // root: conditional, result=false
		makeNode("n3", "merge"),                     // merge: pending=2
	}
	edges := []store.WorkflowEdge{
		makeEdge("e1", "n1", "n3"),                  // unconditional: always fires
		makeBranchEdge("e2", "n2", "n3", "true"),   // labelled "true" but result=false → suppressed
	}
	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	bus := NewEventBus()
	out, _, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mergeHandler.calls.Load() != 1 {
		t.Errorf("merge: want 1 call, got %d — node was wrongly skipped", mergeHandler.calls.Load())
	}
	if _, ok := out["n3"]; !ok {
		t.Error("n3 (merge) should appear in final output")
	}
}

// TestRunDAG_AllSinksSkipped verifies that runDAG returns an error when the only
// sink node is on the suppressed branch of a conditional.
func TestRunDAG_AllSinksSkipped(t *testing.T) {
	sink := &countingHandler{meta: newMeta("sink")}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("cond"), output: map[string]any{"result": false}},
		sink,
	)

	// n1(cond, result=false) →[true]→ n2(sink) — the true edge is suppressed.
	nodes := []store.WorkflowNode{makeNode("n1", "cond"), makeNode("n2", "sink")}
	edges := []store.WorkflowEdge{makeBranchEdge("e1", "n1", "n2", "true")}
	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	bus := NewEventBus()
	_, _, err = runDAG(context.Background(), "run-1", dag, nil, registry, bus, nil)
	if err == nil {
		t.Fatal("expected error when all sink branches are suppressed, got nil")
	}
	if sink.calls.Load() != 0 {
		t.Errorf("sink should not have executed, got %d calls", sink.calls.Load())
	}
}

// TestBranchAllows covers the branchAllows helper directly.
func TestBranchAllows(t *testing.T) {
	trueLabel := "true"
	falseLabel := "false"

	tests := []struct {
		name    string
		edge    store.WorkflowEdge
		output  map[string]any
		allowed bool
	}{
		{
			name:    "no label always allows",
			edge:    store.WorkflowEdge{},
			output:  map[string]any{},
			allowed: true,
		},
		{
			name:    "true label matches true result",
			edge:    store.WorkflowEdge{BranchLabel: &trueLabel},
			output:  map[string]any{"result": true},
			allowed: true,
		},
		{
			name:    "true label blocked by false result",
			edge:    store.WorkflowEdge{BranchLabel: &trueLabel},
			output:  map[string]any{"result": false},
			allowed: false,
		},
		{
			name:    "false label matches false result",
			edge:    store.WorkflowEdge{BranchLabel: &falseLabel},
			output:  map[string]any{"result": false},
			allowed: true,
		},
		{
			name:    "false label blocked by true result",
			edge:    store.WorkflowEdge{BranchLabel: &falseLabel},
			output:  map[string]any{"result": true},
			allowed: false,
		},
		{
			name:    "labelled edge blocked when result missing",
			edge:    store.WorkflowEdge{BranchLabel: &trueLabel},
			output:  map[string]any{},
			allowed: false,
		},
		{
			name:    "labelled edge blocked when result is non-bool",
			edge:    store.WorkflowEdge{BranchLabel: &trueLabel},
			output:  map[string]any{"result": "yes"},
			allowed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := branchAllows(tc.edge, tc.output)
			if got != tc.allowed {
				t.Errorf("want %v, got %v", tc.allowed, got)
			}
		})
	}
}

// ---- loop execution tests --------------------------------------------------

// makeLoopCtrlNode creates a loop.controller WorkflowNode with the given config.
func makeLoopCtrlNode(id string, maxIter int) store.WorkflowNode {
	return store.WorkflowNode{
		ID:     id,
		TypeID: "loop.controller",
		Config: map[string]any{"max_iterations": float64(maxIter)},
	}
}

// loopBackEdgeR creates an IsLoopBack edge for runner tests.
func loopBackEdgeR(id, src, tgt string) store.WorkflowEdge {
	return store.WorkflowEdge{ID: id, SourceID: src, TargetID: tgt, IsLoopBack: true}
}

// labelEdgeR creates a branch-labelled edge for runner tests.
func labelEdgeR(id, src, tgt, label string) store.WorkflowEdge {
	return store.WorkflowEdge{ID: id, SourceID: src, TargetID: tgt, BranchLabel: &label}
}

// buildLoopDAG builds a simple loop DAG:
//
//	upstream → ctrl →(loop_body)→ body →(loop_back)→ ctrl
//	ctrl →(exit)→ downstream
func buildLoopDAG(t *testing.T, maxIter int) *DAG {
	t.Helper()
	nodes := []store.WorkflowNode{
		makeNode("upstream", "fixed"),
		makeLoopCtrlNode("ctrl", maxIter),
		makeNode("body", "body_type"),
		makeNode("downstream", "fixed"),
	}
	edges := []store.WorkflowEdge{
		makeEdge("e1", "upstream", "ctrl"),
		labelEdgeR("e2", "ctrl", "body", "loop_body"),
		labelEdgeR("e3", "ctrl", "downstream", "exit"),
		loopBackEdgeR("e4", "body", "ctrl"),
	}
	d, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("buildLoopDAG: %v", err)
	}
	return d
}

// TestRunDAG_LoopBodyRunsNTimes verifies that with max_iterations=3 and no exit
// condition, the body node executes exactly 3 times before the downstream fires.
func TestRunDAG_LoopBodyRunsNTimes(t *testing.T) {
	const maxIter = 3

	bodyCounter := &countingHandler{meta: newMeta("body_type")}

	// loop.controller handler: returns loop_body until iteration 3, then exit.
	ctrlHandler := &funcHandler{
		meta: newMeta("loop.controller"),
		execFn: func(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
			iter := 0
			if state, ok := input.UpstreamData["_loop_state"].(map[string]any); ok {
				if n, ok := state["iteration"].(int); ok {
					iter = n
				}
			}
			if iter >= maxIter {
				return node.NodeOutput{Data: map[string]any{"action": "exit", "iteration": iter}}, nil
			}
			return node.NodeOutput{Data: map[string]any{"action": "loop_body", "iteration": iter}}, nil
		},
	}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("fixed"), output: map[string]any{"ok": true}},
		ctrlHandler,
		bodyCounter,
	)

	d := buildLoopDAG(t, maxIter)
	bus := NewEventBus()
	_, nodeResults, err := runDAG(context.Background(), "run-loop", d, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("runDAG failed: %v", err)
	}

	if int(bodyCounter.calls.Load()) != maxIter {
		t.Errorf("body should execute %d times, executed %d", maxIter, bodyCounter.calls.Load())
	}
	if nodeResults["downstream"].Status != "succeeded" {
		t.Errorf("downstream should have succeeded, got %v", nodeResults["downstream"])
	}
}

// TestRunDAG_LoopExitsImmediately verifies that when the controller returns
// action=exit on the first call, the body node never executes.
func TestRunDAG_LoopExitsImmediately(t *testing.T) {
	bodyCounter := &countingHandler{meta: newMeta("body_type")}
	ctrlHandler := &fixedHandler{
		meta:   newMeta("loop.controller"),
		output: map[string]any{"action": "exit", "iteration": 0},
	}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("fixed"), output: map[string]any{"ok": true}},
		ctrlHandler,
		bodyCounter,
	)

	d := buildLoopDAG(t, 10)
	bus := NewEventBus()
	_, _, err := runDAG(context.Background(), "run-noop", d, nil, registry, bus, nil)
	if err != nil {
		t.Fatalf("runDAG failed: %v", err)
	}

	if bodyCounter.calls.Load() != 0 {
		t.Errorf("body should not execute when ctrl exits immediately, got %d calls", bodyCounter.calls.Load())
	}
}

// TestRunDAG_LoopBodyOutputVisibleToController verifies that after each body
// iteration, the controller can read the body's output from UpstreamData.
func TestRunDAG_LoopBodyOutputVisibleToController(t *testing.T) {
	const maxIter = 2
	var lastSeenBodyOutput map[string]any

	ctrlHandler := &funcHandler{
		meta: newMeta("loop.controller"),
		execFn: func(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
			iter := 0
			if state, ok := input.UpstreamData["_loop_state"].(map[string]any); ok {
				if n, ok := state["iteration"].(int); ok {
					iter = n
				}
			}
			// Save what the controller sees from the body.
			if bodyOut, ok := input.UpstreamData["body"].(map[string]any); ok {
				lastSeenBodyOutput = bodyOut
			}
			if iter >= maxIter {
				return node.NodeOutput{Data: map[string]any{"action": "exit", "iteration": iter}}, nil
			}
			return node.NodeOutput{Data: map[string]any{"action": "loop_body", "iteration": iter}}, nil
		},
	}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("fixed"), output: map[string]any{"ok": true}},
		ctrlHandler,
		&fixedHandler{meta: newMeta("body_type"), output: map[string]any{"body_result": "hello"}},
	)

	d := buildLoopDAG(t, maxIter)
	bus := NewEventBus()
	if _, _, err := runDAG(context.Background(), "run-vis", d, nil, registry, bus, nil); err != nil {
		t.Fatalf("runDAG failed: %v", err)
	}

	if lastSeenBodyOutput == nil || lastSeenBodyOutput["body_result"] != "hello" {
		t.Errorf("controller should see body output; got %v", lastSeenBodyOutput)
	}
}

// TestRunDAG_LoopBodyFailurePropagates verifies that a failing body node causes
// the overall run to fail.
func TestRunDAG_LoopBodyFailurePropagates(t *testing.T) {
	ctrlHandler := &fixedHandler{
		meta:   newMeta("loop.controller"),
		output: map[string]any{"action": "loop_body", "iteration": 0},
	}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("fixed"), output: map[string]any{"ok": true}},
		ctrlHandler,
		&failHandler{meta: newMeta("body_type")},
	)

	d := buildLoopDAG(t, 3)
	bus := NewEventBus()
	_, _, err := runDAG(context.Background(), "run-fail", d, nil, registry, bus, nil)
	if err == nil {
		t.Fatal("expected error from failing body node")
	}
}

// TestRunDAG_LoopPrePostNodesRunOnce verifies that nodes before and after the
// loop execute exactly once regardless of iteration count.
func TestRunDAG_LoopPrePostNodesRunOnce(t *testing.T) {
	const maxIter = 3

	upstreamCounter := &countingHandler{meta: newMeta("upstream_type")}
	downstreamCounter := &countingHandler{meta: newMeta("downstream_type")}
	bodyCounter := &countingHandler{meta: newMeta("body_type")}

	ctrlHandler := &funcHandler{
		meta: newMeta("loop.controller"),
		execFn: func(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
			iter := 0
			if state, ok := input.UpstreamData["_loop_state"].(map[string]any); ok {
				if n, ok := state["iteration"].(int); ok {
					iter = n
				}
			}
			if iter >= maxIter {
				return node.NodeOutput{Data: map[string]any{"action": "exit", "iteration": iter}}, nil
			}
			return node.NodeOutput{Data: map[string]any{"action": "loop_body", "iteration": iter}}, nil
		},
	}

	// Use distinct type IDs for each node so they get distinct handlers.
	nodes := []store.WorkflowNode{
		{ID: "upstream", TypeID: "upstream_type"},
		makeLoopCtrlNode("ctrl", maxIter),
		{ID: "body", TypeID: "body_type"},
		{ID: "downstream", TypeID: "downstream_type"},
	}
	edges := []store.WorkflowEdge{
		makeEdge("e1", "upstream", "ctrl"),
		labelEdgeR("e2", "ctrl", "body", "loop_body"),
		labelEdgeR("e3", "ctrl", "downstream", "exit"),
		loopBackEdgeR("e4", "body", "ctrl"),
	}
	d, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	registry := newTestRegistry(upstreamCounter, ctrlHandler, bodyCounter, downstreamCounter)
	bus := NewEventBus()
	if _, _, err := runDAG(context.Background(), "run-counts", d, nil, registry, bus, nil); err != nil {
		t.Fatalf("runDAG: %v", err)
	}

	if upstreamCounter.calls.Load() != 1 {
		t.Errorf("upstream should run exactly once, got %d", upstreamCounter.calls.Load())
	}
	if downstreamCounter.calls.Load() != 1 {
		t.Errorf("downstream should run exactly once, got %d", downstreamCounter.calls.Load())
	}
	if int(bodyCounter.calls.Load()) != maxIter {
		t.Errorf("body should run %d times, got %d", maxIter, bodyCounter.calls.Load())
	}
}

// TestRunDAG_LoopContextCancellation verifies that context cancellation during
// a loop body causes the run to terminate cleanly.
func TestRunDAG_LoopContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Body handler blocks until context is cancelled.
	ctrlHandler := &fixedHandler{
		meta:   newMeta("loop.controller"),
		output: map[string]any{"action": "loop_body", "iteration": 0},
	}
	blockingBody := &funcHandler{
		meta: newMeta("body_type"),
		execFn: func(ctx context.Context, _ node.NodeInput) (node.NodeOutput, error) {
			<-ctx.Done()
			return node.NodeOutput{}, ctx.Err()
		},
	}

	registry := newTestRegistry(
		&fixedHandler{meta: newMeta("fixed"), output: map[string]any{"ok": true}},
		ctrlHandler,
		blockingBody,
	)

	d := buildLoopDAG(t, 10)
	bus := NewEventBus()
	_, _, err := runDAG(ctx, "run-cancel", d, nil, registry, bus, nil)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

// TestBranchAllows_ActionKey verifies that "action" values in node output route
// edges correctly for loop.controller-style labelled edges.
func TestBranchAllows_ActionKey(t *testing.T) {
	loopBodyLabel := "loop_body"
	exitLabel := "exit"

	tests := []struct {
		name    string
		edge    store.WorkflowEdge
		output  map[string]any
		allowed bool
	}{
		{
			name:    "loop_body label matches action=loop_body",
			edge:    store.WorkflowEdge{BranchLabel: &loopBodyLabel},
			output:  map[string]any{"action": "loop_body"},
			allowed: true,
		},
		{
			name:    "exit label matches action=exit",
			edge:    store.WorkflowEdge{BranchLabel: &exitLabel},
			output:  map[string]any{"action": "exit"},
			allowed: true,
		},
		{
			name:    "loop_body label blocked when action=exit",
			edge:    store.WorkflowEdge{BranchLabel: &loopBodyLabel},
			output:  map[string]any{"action": "exit"},
			allowed: false,
		},
		{
			name:    "exit label blocked when action=loop_body",
			edge:    store.WorkflowEdge{BranchLabel: &exitLabel},
			output:  map[string]any{"action": "loop_body"},
			allowed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := branchAllows(tc.edge, tc.output)
			if got != tc.allowed {
				t.Errorf("branchAllows: want %v, got %v", tc.allowed, got)
			}
		})
	}
}
