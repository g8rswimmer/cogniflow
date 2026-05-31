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

// ---- tests ---------------------------------------------------------------

func TestRunDAG_SingleNode(t *testing.T) {
	registry := newTestRegistry(&fixedHandler{
		meta:   newMeta("fixed"),
		output: map[string]any{"result": "done"},
	})

	dag, _ := Build([]store.WorkflowNode{makeNode("n1", "fixed")}, nil)

	bus := NewEventBus()
	out, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus)
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
	out, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus)
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
	out, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus)
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
	_, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus)
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
	out, err := runDAG(context.Background(), "run-1", dag, nil, node.NewRegistry(), bus)
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
	_, err := runDAG(context.Background(), "run-1", dag, map[string]any{"msg": "hello"}, registry, bus)
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
	_, err := runDAG(ctx, "run-1", dag, nil, registry, bus)
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
	out, err := runDAG(context.Background(), "run-1", dag, nil, registry, bus)
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
