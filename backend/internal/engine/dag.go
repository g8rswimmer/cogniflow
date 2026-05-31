package engine

import (
	"errors"
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ErrCycleDetected is returned when the workflow graph contains a cycle.
var ErrCycleDetected = errors.New("cycle detected")

// DAG holds adjacency lists for efficient graph traversal during execution.
type DAG struct {
	// Nodes maps node ID → WorkflowNode.
	Nodes map[string]store.WorkflowNode
	// Successors maps node ID → immediate downstream node IDs.
	Successors map[string][]string
	// Predecessors maps node ID → immediate upstream node IDs.
	Predecessors map[string][]string
	// TopologicalOrder is a deterministic execution ordering.
	TopologicalOrder []string
}

// Build constructs a DAG from raw node and edge lists.
// Returns ErrCycleDetected if the graph is not acyclic.
func Build(nodes []store.WorkflowNode, edges []store.WorkflowEdge) (*DAG, error) {
	d := &DAG{
		Nodes:        make(map[string]store.WorkflowNode, len(nodes)),
		Successors:   make(map[string][]string, len(nodes)),
		Predecessors: make(map[string][]string, len(nodes)),
	}

	for _, n := range nodes {
		d.Nodes[n.ID] = n
		d.Successors[n.ID] = nil
		d.Predecessors[n.ID] = nil
	}

	for _, e := range edges {
		if _, ok := d.Nodes[e.SourceID]; !ok {
			return nil, fmt.Errorf("edge %q references unknown source node %q", e.ID, e.SourceID)
		}
		if _, ok := d.Nodes[e.TargetID]; !ok {
			return nil, fmt.Errorf("edge %q references unknown target node %q", e.ID, e.TargetID)
		}
		d.Successors[e.SourceID] = append(d.Successors[e.SourceID], e.TargetID)
		d.Predecessors[e.TargetID] = append(d.Predecessors[e.TargetID], e.SourceID)
	}

	order, err := topoSort(d)
	if err != nil {
		return nil, err
	}
	d.TopologicalOrder = order

	return d, nil
}

// CycleDetect returns ErrCycleDetected if the graph described by nodes and edges
// contains a cycle. It is called at workflow save time.
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

	for id := range d.Nodes {
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
