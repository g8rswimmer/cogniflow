package engine

import (
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func wn(id string) store.WorkflowNode { return store.WorkflowNode{ID: id} }
func edge(id, src, dst string) store.WorkflowEdge {
	return store.WorkflowEdge{ID: id, SourceID: src, TargetID: dst}
}

// TestBuild_LinearChain verifies topological order for a -> b -> c.
func TestBuild_LinearChain(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a"), wn("b"), wn("c")}
	edges := []store.WorkflowEdge{edge("e1", "a", "b"), edge("e2", "b", "c")}

	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertOrder(t, dag.TopologicalOrder, "a", "b", "c")
}

// TestBuild_SingleNode ensures a solo node builds without error.
func TestBuild_SingleNode(t *testing.T) {
	dag, err := Build([]store.WorkflowNode{wn("solo")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dag.TopologicalOrder) != 1 || dag.TopologicalOrder[0] != "solo" {
		t.Fatalf("expected [solo], got %v", dag.TopologicalOrder)
	}
}

// TestBuild_FanOut verifies a -> b, a -> c (parallel branches).
func TestBuild_FanOut(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a"), wn("b"), wn("c")}
	edges := []store.WorkflowEdge{edge("e1", "a", "b"), edge("e2", "a", "c")}

	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dag.TopologicalOrder[0] != "a" {
		t.Fatalf("a must be first, got %v", dag.TopologicalOrder)
	}
	if len(dag.Successors["a"]) != 2 {
		t.Fatalf("a should have 2 successors")
	}
}

// TestBuild_FanIn verifies b -> d, c -> d (merge).
func TestBuild_FanIn(t *testing.T) {
	nodes := []store.WorkflowNode{wn("b"), wn("c"), wn("d")}
	edges := []store.WorkflowEdge{edge("e1", "b", "d"), edge("e2", "c", "d")}

	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dag.TopologicalOrder[len(dag.TopologicalOrder)-1] != "d" {
		t.Fatalf("d must be last, got %v", dag.TopologicalOrder)
	}
	if len(dag.Predecessors["d"]) != 2 {
		t.Fatalf("d should have 2 predecessors")
	}
}

// TestBuild_DisconnectedNodes verifies nodes with no edges are all present.
func TestBuild_DisconnectedNodes(t *testing.T) {
	nodes := []store.WorkflowNode{wn("x"), wn("y"), wn("z")}
	dag, err := Build(nodes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dag.TopologicalOrder) != 3 {
		t.Fatalf("expected 3 nodes, got %v", dag.TopologicalOrder)
	}
}

// TestBuild_Ancestors_Linear verifies a → b → c: c's ancestors = {a, b}.
func TestBuild_Ancestors_Linear(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a"), wn("b"), wn("c")}
	edges := []store.WorkflowEdge{edge("e1", "a", "b"), edge("e2", "b", "c")}

	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dag.Ancestors["a"]) != 0 {
		t.Errorf("a (root) should have 0 ancestors, got %v", dag.Ancestors["a"])
	}
	if len(dag.Ancestors["b"]) != 1 || dag.Ancestors["b"][0] != "a" {
		t.Errorf("b should have ancestor [a], got %v", dag.Ancestors["b"])
	}
	if len(dag.Ancestors["c"]) != 2 {
		t.Errorf("c should have 2 ancestors (a, b), got %v", dag.Ancestors["c"])
	}
	ancestorSet := map[string]bool{}
	for _, id := range dag.Ancestors["c"] {
		ancestorSet[id] = true
	}
	if !ancestorSet["a"] || !ancestorSet["b"] {
		t.Errorf("c's ancestor set should contain a and b, got %v", dag.Ancestors["c"])
	}
}

// TestBuild_Ancestors_Diamond verifies fan-out/fan-in: root → b, root → c, b → sink, c → sink.
// sink's ancestors should be root, b, and c.
func TestBuild_Ancestors_Diamond(t *testing.T) {
	nodes := []store.WorkflowNode{wn("root"), wn("b"), wn("c"), wn("sink")}
	edges := []store.WorkflowEdge{
		edge("e1", "root", "b"), edge("e2", "root", "c"),
		edge("e3", "b", "sink"), edge("e4", "c", "sink"),
	}

	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dag.Ancestors["sink"]) != 3 {
		t.Errorf("sink should have 3 ancestors (root, b, c), got %v", dag.Ancestors["sink"])
	}
}

// TestBuild_Ancestors_Parallel verifies that parallel sibling nodes are NOT
// in each other's ancestor sets. For root → b and root → c (no edge b↔c),
// b should not be an ancestor of c and vice versa.
func TestBuild_Ancestors_Parallel(t *testing.T) {
	nodes := []store.WorkflowNode{wn("root"), wn("b"), wn("c")}
	edges := []store.WorkflowEdge{edge("e1", "root", "b"), edge("e2", "root", "c")}

	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, anc := range dag.Ancestors["b"] {
		if anc == "c" {
			t.Error("c should NOT be an ancestor of b (they are siblings)")
		}
	}
	for _, anc := range dag.Ancestors["c"] {
		if anc == "b" {
			t.Error("b should NOT be an ancestor of c (they are siblings)")
		}
	}
}

// TestBuild_SimpleCycle detects a -> b -> a.
func TestBuild_SimpleCycle(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a"), wn("b")}
	edges := []store.WorkflowEdge{edge("e1", "a", "b"), edge("e2", "b", "a")}

	_, err := Build(nodes, edges)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestBuild_SelfLoop detects a -> a.
func TestBuild_SelfLoop(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a")}
	edges := []store.WorkflowEdge{edge("e1", "a", "a")}

	_, err := Build(nodes, edges)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestBuild_ThreeNodeCycle detects a -> b -> c -> a.
func TestBuild_ThreeNodeCycle(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a"), wn("b"), wn("c")}
	edges := []store.WorkflowEdge{
		edge("e1", "a", "b"),
		edge("e2", "b", "c"),
		edge("e3", "c", "a"),
	}

	_, err := Build(nodes, edges)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestBuild_UnknownSourceNode rejects edges referencing non-existent nodes.
func TestBuild_UnknownSourceNode(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a")}
	edges := []store.WorkflowEdge{edge("e1", "ghost", "a")}

	_, err := Build(nodes, edges)
	if err == nil {
		t.Fatal("expected error for unknown source node")
	}
}

// TestBuild_UnknownTargetNode rejects edges to non-existent target nodes.
func TestBuild_UnknownTargetNode(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a")}
	edges := []store.WorkflowEdge{edge("e1", "a", "ghost")}

	_, err := Build(nodes, edges)
	if err == nil {
		t.Fatal("expected error for unknown target node")
	}
}

// TestCycleDetect_Valid passes for an acyclic graph.
func TestCycleDetect_Valid(t *testing.T) {
	nodes := []store.WorkflowNode{wn("n1"), wn("n2")}
	edges := []store.WorkflowEdge{edge("e1", "n1", "n2")}
	if err := CycleDetect(nodes, edges); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestCycleDetect_Invalid fails for a cyclic graph.
func TestCycleDetect_Invalid(t *testing.T) {
	nodes := []store.WorkflowNode{wn("n1"), wn("n2")}
	edges := []store.WorkflowEdge{
		edge("e1", "n1", "n2"),
		edge("e2", "n2", "n1"),
	}
	if err := CycleDetect(nodes, edges); !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestBuild_DiamondDAG verifies a -> b, a -> c, b -> d, c -> d (diamond).
func TestBuild_DiamondDAG(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a"), wn("b"), wn("c"), wn("d")}
	edges := []store.WorkflowEdge{
		edge("e1", "a", "b"),
		edge("e2", "a", "c"),
		edge("e3", "b", "d"),
		edge("e4", "c", "d"),
	}

	dag, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	order := dag.TopologicalOrder
	if order[0] != "a" {
		t.Fatalf("a must be first, got %v", order)
	}
	if order[len(order)-1] != "d" {
		t.Fatalf("d must be last, got %v", order)
	}
}

// assertOrder checks that got starts with expected in the given sequence.
func assertOrder(t *testing.T, got []string, expected ...string) {
	t.Helper()
	if len(got) != len(expected) {
		t.Fatalf("order length: expected %d, got %d (%v)", len(expected), len(got), got)
	}
	for i, want := range expected {
		if got[i] != want {
			t.Fatalf("position %d: expected %q, got %q (full: %v)", i, want, got[i], got)
		}
	}
}
