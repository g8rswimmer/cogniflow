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

// ---- loop support helpers --------------------------------------------------

func loopCtrl(id string) store.WorkflowNode {
	return store.WorkflowNode{ID: id, TypeID: "loop.controller", Config: map[string]any{"max_iterations": float64(3)}}
}

func loopBackEdge(id, src, dst string) store.WorkflowEdge {
	return store.WorkflowEdge{ID: id, SourceID: src, TargetID: dst, IsLoopBack: true}
}

func labelEdge(id, src, dst, label string) store.WorkflowEdge {
	return store.WorkflowEdge{ID: id, SourceID: src, TargetID: dst, BranchLabel: &label}
}

// ---- loop DAG tests --------------------------------------------------------

// TestBuild_LoopBack_ExcludedFromTopoSort verifies that a loop-back edge is
// excluded from cycle detection and from the adjacency structure used by the engine.
// Topology: upstream → ctrl →(loop_body)→ body →(loop_back)→ ctrl, ctrl →(exit)→ downstream
func TestBuild_LoopBack_ExcludedFromTopoSort(t *testing.T) {
	nodes := []store.WorkflowNode{
		wn("upstream"), loopCtrl("ctrl"), wn("body"), wn("downstream"),
	}
	edges := []store.WorkflowEdge{
		edge("e1", "upstream", "ctrl"),
		labelEdge("e2", "ctrl", "body", "loop_body"),
		labelEdge("e3", "ctrl", "downstream", "exit"),
		loopBackEdge("e4", "body", "ctrl"),
	}

	d, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error building loop DAG: %v", err)
	}

	// Loop-back edge must be stored separately.
	if len(d.LoopBackEdges) != 1 || d.LoopBackEdges[0].ID != "e4" {
		t.Errorf("expected 1 loop-back edge e4, got %v", d.LoopBackEdges)
	}

	// Loop-back edge must NOT appear in Predecessors or Successors of any node.
	if sliceContains(d.Predecessors["ctrl"], "body") {
		t.Error("body must NOT be a regular predecessor of ctrl (only loop-back)")
	}
	if sliceContains(d.Successors["body"], "ctrl") {
		t.Error("ctrl must NOT be a regular successor of body (only loop-back)")
	}

	// Topological order must include all nodes and be cycle-free.
	if len(d.TopologicalOrder) != 4 {
		t.Errorf("expected 4 nodes in topo order, got %v", d.TopologicalOrder)
	}
}

// TestBuild_LoopBodyNodes_Populated verifies that BFS from ctrl's "loop_body" edge
// correctly identifies body as a loop body node (not ctrl or downstream).
func TestBuild_LoopBodyNodes_Populated(t *testing.T) {
	nodes := []store.WorkflowNode{wn("upstream"), loopCtrl("ctrl"), wn("body"), wn("downstream")}
	edges := []store.WorkflowEdge{
		edge("e1", "upstream", "ctrl"),
		labelEdge("e2", "ctrl", "body", "loop_body"),
		labelEdge("e3", "ctrl", "downstream", "exit"),
		loopBackEdge("e4", "body", "ctrl"),
	}

	d, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodySet, ok := d.LoopBodyNodes["ctrl"]
	if !ok {
		t.Fatal("expected LoopBodyNodes entry for ctrl")
	}
	if !bodySet["body"] {
		t.Error("body should be in loop body set")
	}
	if bodySet["ctrl"] {
		t.Error("ctrl must NOT be in its own loop body set")
	}
	if bodySet["downstream"] {
		t.Error("downstream must NOT be in loop body set")
	}
}

// TestBuild_LoopBack_MustTargetController rejects a loop-back edge that targets
// a non-loop.controller node.
func TestBuild_LoopBack_MustTargetController(t *testing.T) {
	nodes := []store.WorkflowNode{wn("a"), wn("b")}
	edges := []store.WorkflowEdge{
		loopBackEdge("e1", "b", "a"), // a is not a loop.controller
	}
	_, err := Build(nodes, edges)
	if err == nil {
		t.Fatal("expected error for loop-back edge targeting non-controller")
	}
}

// TestBuild_MultipleControllersForbidden rejects workflows with more than one
// loop.controller node.
func TestBuild_MultipleControllersForbidden(t *testing.T) {
	nodes := []store.WorkflowNode{loopCtrl("ctrl1"), loopCtrl("ctrl2")}
	_, err := Build(nodes, nil)
	if err == nil {
		t.Fatal("expected error for multiple loop.controller nodes")
	}
}

// TestBuild_ControllerMissingLoopBodyEdge rejects a controller with no "loop_body" outgoing edge.
func TestBuild_ControllerMissingLoopBodyEdge(t *testing.T) {
	nodes := []store.WorkflowNode{loopCtrl("ctrl"), wn("downstream")}
	edges := []store.WorkflowEdge{
		labelEdge("e1", "ctrl", "downstream", "exit"),
	}
	_, err := Build(nodes, edges)
	if err == nil {
		t.Fatal("expected error: controller missing loop_body edge")
	}
}

// TestBuild_ControllerMissingExitEdge rejects a controller with no "exit" outgoing edge.
func TestBuild_ControllerMissingExitEdge(t *testing.T) {
	nodes := []store.WorkflowNode{loopCtrl("ctrl"), wn("body")}
	edges := []store.WorkflowEdge{
		labelEdge("e1", "ctrl", "body", "loop_body"),
		loopBackEdge("e2", "body", "ctrl"),
	}
	_, err := Build(nodes, edges)
	if err == nil {
		t.Fatal("expected error: controller missing exit edge")
	}
}

// TestBuild_ForwardEdgeToControllerValid confirms that a regular forward edge
// targeting a loop.controller (from a pre-loop upstream node) is valid.
func TestBuild_ForwardEdgeToControllerValid(t *testing.T) {
	nodes := []store.WorkflowNode{wn("upstream"), loopCtrl("ctrl"), wn("body"), wn("downstream")}
	edges := []store.WorkflowEdge{
		edge("e1", "upstream", "ctrl"),
		labelEdge("e2", "ctrl", "body", "loop_body"),
		labelEdge("e3", "ctrl", "downstream", "exit"),
		loopBackEdge("e4", "body", "ctrl"),
	}
	_, err := Build(nodes, edges)
	if err != nil {
		t.Fatalf("forward edge to loop.controller should be valid, got: %v", err)
	}
}

// TestBuild_LoopBackWithBranchLabel rejects a loop-back edge that carries a branch_label.
func TestBuild_LoopBackWithBranchLabel(t *testing.T) {
	label := "some-label"
	nodes := []store.WorkflowNode{wn("body"), loopCtrl("ctrl")}
	edges := []store.WorkflowEdge{
		{ID: "e1", SourceID: "body", TargetID: "ctrl", IsLoopBack: true, BranchLabel: &label},
	}
	_, err := Build(nodes, edges)
	if err == nil {
		t.Fatal("expected error: loop-back edge with branch_label")
	}
}

// TestBuild_UnmarkedCycleStillDetected verifies that a true cycle (IsLoopBack=false)
// is still rejected even when a loop.controller exists.
func TestBuild_UnmarkedCycleStillDetected(t *testing.T) {
	nodes := []store.WorkflowNode{loopCtrl("ctrl"), wn("body"), wn("downstream")}
	edges := []store.WorkflowEdge{
		labelEdge("e1", "ctrl", "body", "loop_body"),
		labelEdge("e2", "ctrl", "downstream", "exit"),
		// cycle without is_loop_back flag — must be rejected
		edge("e3", "body", "ctrl"),
	}
	_, err := Build(nodes, edges)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected for unmarked back edge, got: %v", err)
	}
}
