package engine

import "sync"

// ExecutionContext is the run-scoped, thread-safe store of each node's output.
// Keys are node IDs (plus the reserved "_initial" key for the run's initial data).
type ExecutionContext struct {
	mu      sync.RWMutex
	outputs map[string]map[string]any
}

func newExecutionContext() *ExecutionContext {
	return &ExecutionContext{outputs: make(map[string]map[string]any)}
}

// set stores the output for a node. Called by the supervisor goroutine after a node succeeds.
func (ec *ExecutionContext) set(nodeID string, data map[string]any) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.outputs[nodeID] = data
}

// mergeUpstream returns a map of nodeID → output for all given predecessor IDs.
// The "_initial" key (run initial data) is always included if present.
// This map becomes NodeInput.UpstreamData for the next node.
func (ec *ExecutionContext) mergeUpstream(predecessorIDs []string) map[string]any {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	merged := make(map[string]any, len(predecessorIDs)+1)
	if init, ok := ec.outputs["_initial"]; ok {
		merged["_initial"] = init
	}
	for _, id := range predecessorIDs {
		if out, ok := ec.outputs[id]; ok {
			merged[id] = out
		}
	}
	return merged
}

// sinkOutputs returns the outputs of every node that has no successors in dag.
func (ec *ExecutionContext) sinkOutputs(dag *DAG) map[string]map[string]any {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	result := make(map[string]map[string]any)
	for id := range dag.Nodes {
		if len(dag.Successors[id]) == 0 {
			if out, ok := ec.outputs[id]; ok {
				result[id] = out
			}
		}
	}
	return result
}
