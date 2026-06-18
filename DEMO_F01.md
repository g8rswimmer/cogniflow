# F-01 Demo — Workflow Loops / Cycles

This demo exercises the loop execution feature. A `loop.controller` node drives iterative re-execution of a loop body sub-graph with a configurable `max_iterations` cap and an optional CEL `exit_condition`. All existing (non-looping) workflows are unaffected.

## Prerequisites

```bash
docker compose up --build
```

Set a convenience variable:

```bash
BASE=http://localhost:8080/v1
```

---

## Scenario 1 — Fixed N iterations (no exit condition)

Creates a workflow where an HTTP call fetches a counter value and the loop body runs exactly 3 times regardless of the response.

**Topology:**

```
[fetch_count] → [ctrl] →(loop_body)→ [increment] →(loop-back)→ [ctrl]
                  ↓(exit)
               [report]
```

```bash
WF1=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 Fixed 3 Iterations",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [
      {
        "id": "fetch_count",
        "type_id": "http.request",
        "label": "Fetch initial count",
        "position": {"x": 0, "y": 0},
        "config": {"url": "https://httpbin.org/json", "method": "GET"}
      },
      {
        "id": "ctrl",
        "type_id": "loop.controller",
        "label": "Loop 3×",
        "position": {"x": 300, "y": 0},
        "config": {"max_iterations": 3}
      },
      {
        "id": "increment",
        "type_id": "http.request",
        "label": "Body: fetch anything",
        "position": {"x": 600, "y": -80},
        "config": {"url": "https://httpbin.org/uuid", "method": "GET"}
      },
      {
        "id": "report",
        "type_id": "http.request",
        "label": "Post-loop report",
        "position": {"x": 600, "y": 80},
        "config": {"url": "https://httpbin.org/get?done=true", "method": "GET"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "fetch_count", "target_id": "ctrl",      "branch_label": null,        "is_loop_back": false},
      {"id": "e2", "source_id": "ctrl",        "target_id": "increment",  "branch_label": "loop_body", "is_loop_back": false},
      {"id": "e3", "source_id": "ctrl",        "target_id": "report",     "branch_label": "exit",      "is_loop_back": false},
      {"id": "e4", "source_id": "increment",   "target_id": "ctrl",       "branch_label": null,        "is_loop_back": true}
    ]
  }' | jq -r '.id')

echo "Workflow: $WF1"
```

Trigger a run:

```bash
RUN1=$(curl -s -X POST $BASE/workflows/$WF1/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')

echo "Run: $RUN1"
sleep 15
curl -s $BASE/runs/$RUN1 | jq '{status, node_results: (.node_results | to_entries | map({key, status: .value.status}))}'
```

**Expected output:**

```json
{
  "status": "succeeded",
  "node_results": [
    {"key": "ctrl",        "status": "succeeded"},
    {"key": "fetch_count", "status": "succeeded"},
    {"key": "increment",   "status": "succeeded"},
    {"key": "report",      "status": "succeeded"}
  ]
}
```

`node_results` for `increment` shows its last-iteration output. The controller ran 4 times internally (iterations 0–2 → loop_body; iteration 3 → exit), but only the final result is persisted.

---

## Scenario 2 — CEL exit condition (loop exits early)

The loop body calls httpbin and extracts the `uuid` field. The controller's `exit_condition` checks whether iteration ≥ 2 — causing the loop to exit after 2 body executions rather than the full `max_iterations` cap of 10.

```bash
WF2=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 CEL Exit Condition",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [
      {
        "id": "ctrl",
        "type_id": "loop.controller",
        "label": "Loop until iteration 2",
        "position": {"x": 0, "y": 0},
        "config": {
          "max_iterations": 10,
          "exit_condition": "has(ctx[\"_loop_state\"]) && ctx[\"_loop_state\"][\"iteration\"] >= 2"
        }
      },
      {
        "id": "body",
        "type_id": "http.request",
        "label": "Body work",
        "position": {"x": 300, "y": -80},
        "config": {"url": "https://httpbin.org/uuid", "method": "GET"}
      },
      {
        "id": "done",
        "type_id": "http.request",
        "label": "Post-loop",
        "position": {"x": 300, "y": 80},
        "config": {"url": "https://httpbin.org/get?early=exit", "method": "GET"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "ctrl", "target_id": "body", "branch_label": "loop_body", "is_loop_back": false},
      {"id": "e2", "source_id": "ctrl", "target_id": "done", "branch_label": "exit",      "is_loop_back": false},
      {"id": "e3", "source_id": "body", "target_id": "ctrl", "branch_label": null,         "is_loop_back": true}
    ]
  }' | jq -r '.id')

RUN2=$(curl -s -X POST $BASE/workflows/$WF2/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')

echo "Run: $RUN2"
sleep 10
curl -s $BASE/runs/$RUN2 | jq '{status, final_output}'
```

**Expected:**

```json
{
  "status": "succeeded",
  "final_output": {
    "done": { "status_code": 200, "body": "...", "headers": {} }
  }
}
```

The `done` node is the sink (reached via the `exit` edge). The loop ran 2 body iterations before the exit condition became true.

---

## Scenario 3 — Max iterations enforced (graceful exit)

When `max_iterations` is reached without the exit condition firing, the controller exits gracefully via the `exit` edge — it does not fail the run.

```bash
WF3=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 Max Iterations Cap",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [
      {
        "id": "ctrl",
        "type_id": "loop.controller",
        "label": "Loop (cap at 2)",
        "position": {"x": 0, "y": 0},
        "config": {"max_iterations": 2}
      },
      {
        "id": "body",
        "type_id": "http.request",
        "label": "Body",
        "position": {"x": 300, "y": -80},
        "config": {"url": "https://httpbin.org/uuid", "method": "GET"}
      },
      {
        "id": "sink",
        "type_id": "http.request",
        "label": "Sink",
        "position": {"x": 300, "y": 80},
        "config": {"url": "https://httpbin.org/get?sink=true", "method": "GET"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "ctrl", "target_id": "body", "branch_label": "loop_body", "is_loop_back": false},
      {"id": "e2", "source_id": "ctrl", "target_id": "sink", "branch_label": "exit",      "is_loop_back": false},
      {"id": "e3", "source_id": "body", "target_id": "ctrl", "branch_label": null,         "is_loop_back": true}
    ]
  }' | jq -r '.id')

RUN3=$(curl -s -X POST $BASE/workflows/$WF3/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')

sleep 10
curl -s $BASE/runs/$RUN3 | jq '{status, ctrl_output: .node_results.ctrl.output}'
```

**Expected:**

```json
{
  "status": "succeeded",
  "ctrl_output": {
    "action": "exit",
    "iteration": 2,
    "exit_reason": "max_iterations"
  }
}
```

The run succeeds. `exit_reason: "max_iterations"` distinguishes a cap-triggered exit from a condition-triggered exit.

---

## Scenario 4 — Upstream data flows into the loop body

The loop body can reference upstream (pre-loop) node outputs via template syntax. This verifies that the controller and body nodes see the correct upstream context.

```bash
WF4=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 Upstream Context",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "initial_data_schema": {
      "type": "object",
      "properties": {"topic": {"type": "string"}}
    },
    "nodes": [
      {
        "id": "setup",
        "type_id": "http.request",
        "label": "Pre-loop setup",
        "position": {"x": 0, "y": 0},
        "config": {"url": "https://httpbin.org/get?setup=true", "method": "GET"}
      },
      {
        "id": "ctrl",
        "type_id": "loop.controller",
        "label": "Loop 2×",
        "position": {"x": 300, "y": 0},
        "config": {"max_iterations": 2}
      },
      {
        "id": "body",
        "type_id": "http.request",
        "label": "Body: echo topic",
        "position": {"x": 600, "y": -80},
        "config": {
          "url": "https://httpbin.org/get?topic={{._initial.topic}}",
          "method": "GET"
        }
      },
      {
        "id": "sink",
        "type_id": "http.request",
        "label": "Sink",
        "position": {"x": 600, "y": 80},
        "config": {"url": "https://httpbin.org/get?finished=true", "method": "GET"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "setup", "target_id": "ctrl", "branch_label": null,        "is_loop_back": false},
      {"id": "e2", "source_id": "ctrl",  "target_id": "body", "branch_label": "loop_body", "is_loop_back": false},
      {"id": "e3", "source_id": "ctrl",  "target_id": "sink", "branch_label": "exit",      "is_loop_back": false},
      {"id": "e4", "source_id": "body",  "target_id": "ctrl", "branch_label": null,         "is_loop_back": true}
    ]
  }' | jq -r '.id')

RUN4=$(curl -s -X POST $BASE/workflows/$WF4/runs \
  -H 'Content-Type: application/json' \
  -d '{"topic": "cogniflow-loops"}' | jq -r '.run_id')

sleep 10
curl -s $BASE/runs/$RUN4 | jq '{status, body_url: .node_results.body.output.url}'
```

**Expected:**

```json
{
  "status": "succeeded",
  "body_url": "https://httpbin.org/get?topic=cogniflow-loops"
}
```

The template `{{._initial.topic}}` resolved correctly inside the loop body.

---

## Scenario 5 — Save-time validation errors

### 5a — Unmarked back edge (no `is_loop_back` flag)

A regular edge forming a cycle is still rejected:

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 Bad Cycle",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {"id": "a", "type_id": "http.request", "label": "A", "position": {"x": 0,   "y": 0}, "config": {"url": "https://httpbin.org/get", "method": "GET"}},
      {"id": "b", "type_id": "http.request", "label": "B", "position": {"x": 300, "y": 0}, "config": {"url": "https://httpbin.org/get", "method": "GET"}}
    ],
    "edges": [
      {"id": "e1", "source_id": "a", "target_id": "b", "branch_label": null, "is_loop_back": false},
      {"id": "e2", "source_id": "b", "target_id": "a", "branch_label": null, "is_loop_back": false}
    ]
  }' | jq '{error}'
```

**Expected:**

```json
{"error": {"code": "CYCLE_DETECTED", "message": "cycle detected: ..."}}
```

### 5b — Loop-back edge targeting a non-controller node

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 Bad Loop-Back Target",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {"id": "a", "type_id": "http.request", "label": "A", "position": {"x": 0,   "y": 0}, "config": {"url": "https://httpbin.org/get", "method": "GET"}},
      {"id": "b", "type_id": "http.request", "label": "B", "position": {"x": 300, "y": 0}, "config": {"url": "https://httpbin.org/get", "method": "GET"}}
    ],
    "edges": [
      {"id": "e1", "source_id": "a", "target_id": "b", "branch_label": null, "is_loop_back": false},
      {"id": "e2", "source_id": "b", "target_id": "a", "branch_label": null, "is_loop_back": true}
    ]
  }' | jq '{error}'
```

**Expected:**

```json
{"error": {"code": "VALIDATION_FAILED", "message": "loop-back edge must target a loop.controller node ..."}}
```

### 5c — Two loop.controller nodes in one workflow

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 Double Controller",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {"id": "c1", "type_id": "loop.controller", "label": "Ctrl 1", "position": {"x": 0,   "y": 0}, "config": {"max_iterations": 3}},
      {"id": "c2", "type_id": "loop.controller", "label": "Ctrl 2", "position": {"x": 300, "y": 0}, "config": {"max_iterations": 3}}
    ],
    "edges": []
  }' | jq '{error}'
```

**Expected:**

```json
{"error": {"code": "VALIDATION_FAILED", "message": "at most one loop.controller node is permitted per workflow; found 2"}}
```

### 5d — Invalid CEL exit condition

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F01 Bad CEL",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {"id": "ctrl", "type_id": "loop.controller", "label": "Ctrl", "position": {"x": 0, "y": 0},
       "config": {"max_iterations": 5, "exit_condition": "!!!not valid CEL!!!"}},
      {"id": "body", "type_id": "http.request", "label": "Body", "position": {"x": 300, "y": -80},
       "config": {"url": "https://httpbin.org/uuid", "method": "GET"}},
      {"id": "sink", "type_id": "http.request", "label": "Sink", "position": {"x": 300, "y": 80},
       "config": {"url": "https://httpbin.org/get", "method": "GET"}}
    ],
    "edges": [
      {"id": "e1", "source_id": "ctrl", "target_id": "body", "branch_label": "loop_body", "is_loop_back": false},
      {"id": "e2", "source_id": "ctrl", "target_id": "sink", "branch_label": "exit",      "is_loop_back": false},
      {"id": "e3", "source_id": "body", "target_id": "ctrl", "branch_label": null,         "is_loop_back": true}
    ]
  }' | jq '.validation_errors'
```

**Expected:** an array containing a field-level error on node `ctrl`, field `exit_condition`.

---

## Scenario 6 — Verify in the UI

1. Open `http://localhost:3000` and create a new workflow.
2. Drag **Loop Controller** from the node palette onto the canvas.
3. Observe the **Loop Configuration** panel in the right sidebar with **Max Iterations** and **Exit Condition (CEL)** fields, plus wiring instructions.
4. Drag an **HTTP Request** node and connect `ctrl → body` with a `loop_body` edge label.
5. Connect `ctrl → sink` with an `exit` edge label.
6. Draw an edge **from body back to ctrl** — it auto-detects as a loop-back and renders as a **dashed amber** "↩ loop back" edge.
7. Click **Save** — the workflow persists without a cycle error.
8. Click **Run** → observe the run status panel. The body node's status updates on each iteration; the final `node_results` shows the last iteration's output.
9. After the run, open **Run History** → click the run → verify `ctrl`, `body`, and `sink` all show green **succeeded** badges.

---

## Cleanup

```bash
curl -s -X DELETE $BASE/workflows/$WF1
curl -s -X DELETE $BASE/workflows/$WF2
curl -s -X DELETE $BASE/workflows/$WF3
curl -s -X DELETE $BASE/workflows/$WF4
echo "Cleaned up"
```
