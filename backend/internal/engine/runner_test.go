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
