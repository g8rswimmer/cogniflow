package engine

import (
	"errors"
	"fmt"
	"sort"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

const loopControllerTypeID = "loop.controller"

// ErrCycleDetected is returned when the workflow graph contains a cycle.
var ErrCycleDetected = errors.New("cycle detected")

// DAG holds adjacency lists for efficient graph traversal during execution.
type DAG struct {
	// Nodes maps node ID → WorkflowNode.
	Nodes map[string]store.WorkflowNode
	// Successors maps node ID → immediate downstream node IDs (forward edges only).
	Successors map[string][]string
	// Predecessors maps node ID → immediate upstream node IDs (forward edges only).
	Predecessors map[string][]string
	// TopologicalOrder is a deterministic execution ordering (forward edges only).
	TopologicalOrder []string
	// OutEdges maps node ID → outgoing forward edges, preserving branch labels for
	// conditional routing.
	OutEdges map[string][]store.WorkflowEdge

	// Ancestors maps node ID → all transitively reachable ancestor node IDs
	// (every node that has a forward edge path leading to this node).
	Ancestors map[string][]string

	// LoopBackEdges holds every edge whose IsLoopBack flag is true.
	// These are excluded from Successors/Predecessors/TopologicalOrder.
	// The runner uses them to fire re-entry signals for the loop.controller node.
	LoopBackEdges []store.WorkflowEdge

	// LoopBodyNodes maps loop controller node ID → set of body node IDs.
	// Body nodes are those reachable from the controller's "loop_body"-labeled successor
	// via forward edges, excluding the controller itself.
	LoopBodyNodes map[string]map[string]bool
}

// Build constructs a DAG from raw node and edge lists.
// Returns ErrCycleDetected if the forward-edge graph is not acyclic.
// Loop-back edges (IsLoopBack == true) are excluded from cycle detection.
func Build(nodes []store.WorkflowNode, edges []store.WorkflowEdge) (*DAG, error) {
	// Partition edges before building adjacency.
	var forwardEdges, loopBackEdges []store.WorkflowEdge
	for _, e := range edges {
		if e.IsLoopBack {
			loopBackEdges = append(loopBackEdges, e)
		} else {
			forwardEdges = append(forwardEdges, e)
		}
	}

	if err := validateLoopConstraints(nodes, forwardEdges, loopBackEdges); err != nil {
		return nil, err
	}

	d := &DAG{
		Nodes:         make(map[string]store.WorkflowNode, len(nodes)),
		Successors:    make(map[string][]string, len(nodes)),
		Predecessors:  make(map[string][]string, len(nodes)),
		OutEdges:      make(map[string][]store.WorkflowEdge, len(nodes)),
		Ancestors:     make(map[string][]string, len(nodes)),
		LoopBackEdges: loopBackEdges,
		LoopBodyNodes: make(map[string]map[string]bool),
	}

	for _, n := range nodes {
		d.Nodes[n.ID] = n
		d.Successors[n.ID] = nil
		d.Predecessors[n.ID] = nil
		d.OutEdges[n.ID] = nil
	}

	for _, e := range forwardEdges {
		if _, ok := d.Nodes[e.SourceID]; !ok {
			return nil, fmt.Errorf("edge %q references unknown source node %q", e.ID, e.SourceID)
		}
		if _, ok := d.Nodes[e.TargetID]; !ok {
			return nil, fmt.Errorf("edge %q references unknown target node %q", e.ID, e.TargetID)
		}
		d.Successors[e.SourceID] = append(d.Successors[e.SourceID], e.TargetID)
		d.Predecessors[e.TargetID] = append(d.Predecessors[e.TargetID], e.SourceID)
		d.OutEdges[e.SourceID] = append(d.OutEdges[e.SourceID], e)
	}

	// Validate loop-back edge endpoints after adjacency is built so we can look up TypeIDs.
	for _, e := range loopBackEdges {
		if _, ok := d.Nodes[e.SourceID]; !ok {
			return nil, fmt.Errorf("loop-back edge %q references unknown source node %q", e.ID, e.SourceID)
		}
		if _, ok := d.Nodes[e.TargetID]; !ok {
			return nil, fmt.Errorf("loop-back edge %q references unknown target node %q", e.ID, e.TargetID)
		}
	}

	order, err := topoSort(d)
	if err != nil {
		return nil, err
	}
	d.TopologicalOrder = order

	for id := range d.Nodes {
		d.Ancestors[id] = collectAncestors(id, d.Predecessors)
	}

	// Compute loop body node sets for each loop controller.
	for _, n := range nodes {
		if n.TypeID == loopControllerTypeID {
			d.LoopBodyNodes[n.ID] = computeLoopBodyNodes(n.ID, d)
		}
	}

	// Validate that every controller has the required outgoing edge labels.
	for _, n := range nodes {
		if n.TypeID != loopControllerTypeID {
			continue
		}
		var hasLoopBody, hasExit bool
		for _, e := range d.OutEdges[n.ID] {
			if e.BranchLabel != nil && *e.BranchLabel == "loop_body" {
				hasLoopBody = true
			}
			if e.BranchLabel != nil && *e.BranchLabel == "exit" {
				hasExit = true
			}
		}
		if !hasLoopBody {
			return nil, fmt.Errorf("loop.controller node %q must have an outgoing edge labelled %q", n.ID, "loop_body")
		}
		if !hasExit {
			return nil, fmt.Errorf("loop.controller node %q must have an outgoing edge labelled %q", n.ID, "exit")
		}
	}

	return d, nil
}

// validateLoopConstraints checks structural rules for loop-back edges before adjacency is built.
func validateLoopConstraints(nodes []store.WorkflowNode, _ []store.WorkflowEdge, loopBackEdges []store.WorkflowEdge) error {
	// Build a type lookup for quick access.
	nodeType := make(map[string]string, len(nodes))
	controllerCount := 0
	for _, n := range nodes {
		nodeType[n.ID] = n.TypeID
		if n.TypeID == loopControllerTypeID {
			controllerCount++
		}
	}

	if controllerCount > 1 {
		return fmt.Errorf("at most one loop.controller node is permitted per workflow; found %d", controllerCount)
	}

	for _, e := range loopBackEdges {
		if e.BranchLabel != nil {
			return fmt.Errorf("loop-back edge %q must not have a branch_label", e.ID)
		}
		if nodeType[e.TargetID] != loopControllerTypeID {
			return fmt.Errorf("loop-back edge %q must target a loop.controller node (targets %q which has type %q)", e.ID, e.TargetID, nodeType[e.TargetID])
		}
	}

	return nil
}

// computeLoopBodyNodes returns the set of node IDs reachable from the controller's
// "loop_body"-labeled successor via forward edges, excluding the controller itself
// and any nodes that are direct targets of non-loop_body edges from the controller
// (i.e., exit-path nodes). This prevents post-loop nodes from being incorrectly
// included in the body set when a body node happens to share a forward edge with
// a node also targeted by the controller's exit edge.
func computeLoopBodyNodes(controllerID string, d *DAG) map[string]bool {
	// Collect immediate exit targets so the BFS does not cross the loop boundary.
	exitTargets := make(map[string]bool)
	for _, e := range d.OutEdges[controllerID] {
		if e.BranchLabel == nil || *e.BranchLabel != "loop_body" {
			exitTargets[e.TargetID] = true
		}
	}

	body := make(map[string]bool)
	queue := make([]string, 0)

	for _, e := range d.OutEdges[controllerID] {
		if e.BranchLabel != nil && *e.BranchLabel == "loop_body" {
			queue = append(queue, e.TargetID)
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if body[curr] || curr == controllerID || exitTargets[curr] {
			continue
		}
		body[curr] = true
		for _, succ := range d.Successors[curr] {
			if !body[succ] && succ != controllerID && !exitTargets[succ] {
				queue = append(queue, succ)
			}
		}
	}
	return body
}

// collectAncestors returns all transitively reachable ancestor node IDs for
// the given node by walking up the Predecessors map. The returned slice is
// sorted for deterministic ordering.
func collectAncestors(nodeID string, predecessors map[string][]string) []string {
	visited := make(map[string]bool)
	var walk func(id string)
	walk = func(id string) {
		for _, pred := range predecessors[id] {
			if !visited[pred] {
				visited[pred] = true
				walk(pred)
			}
		}
	}
	walk(nodeID)
	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// CycleDetect returns ErrCycleDetected if the graph described by nodes and edges
// contains a cycle in its forward edges. Loop-back edges (IsLoopBack == true) are
// excluded from cycle detection. It is called at workflow save time.
func CycleDetect(nodes []store.WorkflowNode, edges []store.WorkflowEdge) error {
	_, err := Build(nodes, edges)
	return err
}

// topoSort returns a topological ordering of node IDs using DFS with
// three-colour (white/grey/black) marking. A grey→grey back edge signals a cycle.
func topoSort(d *DAG) ([]string, error) {
	const (
		white = 0
		grey  = 1
		black = 2
	)

	color := make(map[string]int, len(d.Nodes))
	order := make([]string, 0, len(d.Nodes))

	var visit func(id string) error
	visit = func(id string) error {
		switch color[id] {
		case grey:
			return fmt.Errorf("%w: node %q is part of a cycle", ErrCycleDetected, id)
		case black:
			return nil
		}
		color[id] = grey
		for _, succ := range d.Successors[id] {
			if err := visit(succ); err != nil {
				return err
			}
		}
		color[id] = black
		order = append(order, id)
		return nil
	}

	// Sort IDs for a deterministic traversal order.
	ids := make([]string, 0, len(d.Nodes))
	for id := range d.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		if color[id] == white {
			if err := visit(id); err != nil {
				return nil, err
			}
		}
	}

	// Reverse to get source-first order.
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order, nil
}
