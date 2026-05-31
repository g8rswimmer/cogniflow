package engine

import (
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func TestExecutionContext_SetAndMerge(t *testing.T) {
	dag := &DAG{
		Nodes:      map[string]store.WorkflowNode{"n1": {}, "n2": {}},
		Successors: map[string][]string{"n1": {"n2"}, "n2": nil},
		Predecessors: map[string][]string{
			"n1": nil,
			"n2": {"n1"},
		},
	}

	ec := newExecutionContext()
	ec.set("_initial", map[string]any{"msg": "hello"})
	ec.set("n1", map[string]any{"status_code": 200, "body": "ok"})

	upstream := ec.mergeUpstream(dag.Predecessors["n2"])

	if upstream["_initial"] == nil {
		t.Error("expected _initial in upstream")
	}
	n1Out, ok := upstream["n1"].(map[string]any)
	if !ok {
		t.Fatalf("expected n1 in upstream, got %T", upstream["n1"])
	}
	if n1Out["status_code"] != 200 {
		t.Errorf("expected status_code=200, got %v", n1Out["status_code"])
	}
}

func TestExecutionContext_MergeUpstream_RootNode(t *testing.T) {
	ec := newExecutionContext()
	ec.set("_initial", map[string]any{"key": "val"})

	upstream := ec.mergeUpstream(nil) // root node has no predecessors
	init, ok := upstream["_initial"].(map[string]any)
	if !ok {
		t.Fatalf("expected _initial in root node upstream, got %T", upstream["_initial"])
	}
	if init["key"] != "val" {
		t.Errorf("expected key=val, got %v", init["key"])
	}
}

func TestExecutionContext_SinkOutputs(t *testing.T) {
	dag := &DAG{
		Nodes: map[string]store.WorkflowNode{
			"n1": {}, "n2": {}, "n3": {},
		},
		Successors: map[string][]string{
			"n1": {"n2"},
			"n2": {"n3"},
			"n3": nil,
		},
		Predecessors: map[string][]string{
			"n1": nil, "n2": {"n1"}, "n3": {"n2"},
		},
	}

	ec := newExecutionContext()
	ec.set("n1", map[string]any{"v": 1})
	ec.set("n2", map[string]any{"v": 2})
	ec.set("n3", map[string]any{"v": 3})

	sinks := ec.sinkOutputs(dag)
	if len(sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinks))
	}
	if sinks["n3"]["v"] != 3 {
		t.Errorf("expected n3.v=3, got %v", sinks["n3"]["v"])
	}
}

func TestExecutionContext_SinkOutputs_MultipleSinks(t *testing.T) {
	dag := &DAG{
		Nodes:      map[string]store.WorkflowNode{"n1": {}, "n2": {}, "n3": {}},
		Successors: map[string][]string{"n1": {"n2", "n3"}, "n2": nil, "n3": nil},
		Predecessors: map[string][]string{
			"n1": nil, "n2": {"n1"}, "n3": {"n1"},
		},
	}

	ec := newExecutionContext()
	ec.set("n2", map[string]any{"branch": "a"})
	ec.set("n3", map[string]any{"branch": "b"})

	sinks := ec.sinkOutputs(dag)
	if len(sinks) != 2 {
		t.Fatalf("expected 2 sinks, got %d", len(sinks))
	}
}
