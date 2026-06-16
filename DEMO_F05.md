# F-05 Demo — Persistent Per-Node Execution Data

This demo exercises the full-run-replay feature. After each run, `GET /v1/runs/{run_id}` now returns a `node_results` map with per-node status, output, and error — not just the sink-node `final_output`. The `RunDetailPage` uses this to show accurate `succeeded` / `failed` / `skipped` badges for every node.

## Prerequisites

```bash
docker compose up --build
```

Set a convenience variable:

```bash
BASE=http://localhost:8080/v1
```

---

## Scenario 1 — Sequential success (all nodes show "succeeded")

Create a two-node sequential workflow:

```bash
WF1=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F05 Sequential",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "n1",
        "type_id": "http.request",
        "label": "Fetch anything",
        "position": {"x": 0, "y": 0},
        "config": {"url": "https://httpbin.org/get", "method": "GET"}
      },
      {
        "id": "n2",
        "type_id": "http.request",
        "label": "Fetch status",
        "position": {"x": 300, "y": 0},
        "config": {"url": "https://httpbin.org/status/200", "method": "GET"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "n1", "target_id": "n2", "branch_label": null}
    ]
  }' | jq -r '.id')

echo "Workflow: $WF1"
```

Trigger a run and wait for it to finish:

```bash
RUN1=$(curl -s -X POST $BASE/workflows/$WF1/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')

sleep 5

curl -s $BASE/runs/$RUN1 | jq '{status, node_results}'
```

**Expected `node_results`:**

```json
{
  "status": "succeeded",
  "node_results": {
    "n1": { "status": "succeeded", "output": { "status_code": 200, "body": "...", "headers": {} } },
    "n2": { "status": "succeeded", "output": { "status_code": 200, "body": "", "headers": {} } }
  }
}
```

Both nodes appear with `"status": "succeeded"`. The old `final_output` only carried `n2` (the sink); `node_results` carries both.

---

## Scenario 2 — Node failure (failed node + downstream nodes are absent)

Create a workflow where `n1` points to a non-existent host and `n2` would run downstream:

```bash
WF2=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F05 Failure",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 15,
    "nodes": [
      {
        "id": "n1",
        "type_id": "http.request",
        "label": "Will fail",
        "position": {"x": 0, "y": 0},
        "config": {"url": "http://localhost:9999/no-such-host", "method": "GET"}
      },
      {
        "id": "n2",
        "type_id": "http.request",
        "label": "Never runs",
        "position": {"x": 300, "y": 0},
        "config": {"url": "https://httpbin.org/get", "method": "GET"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "n1", "target_id": "n2", "branch_label": null}
    ]
  }' | jq -r '.id')

RUN2=$(curl -s -X POST $BASE/workflows/$WF2/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')

sleep 5

curl -s $BASE/runs/$RUN2 | jq '{status, node_results}'
```

**Expected `node_results`:**

```json
{
  "status": "failed",
  "node_results": {
    "n1": { "status": "failed", "error": "http.request: ... connection refused" }
  }
}
```

`n1` shows `"status": "failed"` with an error message. `n2` is **absent** from `node_results` because the engine never dispatched it after `n1` failed. In the `RunDetailPage`, `n2` renders with a gray **"skipped"** badge.

---

## Scenario 3 — Conditional branch (pruned nodes are absent)

Create a workflow where `n1` fetches a 200 response and a conditional routes to either `n3` (success branch) or `n4` (error branch). The untaken branch is absent from `node_results`.

```bash
WF3=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F05 Conditional",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "n1",
        "type_id": "http.request",
        "label": "Fetch",
        "position": {"x": 0, "y": 0},
        "config": {"url": "https://httpbin.org/status/200", "method": "GET"}
      },
      {
        "id": "n2",
        "type_id": "conditional.branch",
        "label": "Check status",
        "position": {"x": 300, "y": 0},
        "config": {
          "rules": [
            {
              "label": "ok",
              "logic": "AND",
              "conditions": [
                {"node_id": "n1", "field": "status_code", "operator": "==", "value": "200", "value_type": "number"}
              ]
            },
            {
              "label": "fail",
              "logic": "AND",
              "conditions": [
                {"node_id": "n1", "field": "status_code", "operator": ">=", "value": "400", "value_type": "number"}
              ]
            }
          ]
        }
      },
      {
        "id": "n3",
        "type_id": "http.request",
        "label": "Success path",
        "position": {"x": 600, "y": -100},
        "config": {"url": "https://httpbin.org/get?path=ok", "method": "GET"}
      },
      {
        "id": "n4",
        "type_id": "http.request",
        "label": "Error path",
        "position": {"x": 600, "y": 100},
        "config": {"url": "https://httpbin.org/get?path=error", "method": "GET"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "n1", "target_id": "n2",  "branch_label": null},
      {"id": "e2", "source_id": "n2", "target_id": "n3",  "branch_label": "ok"},
      {"id": "e3", "source_id": "n2", "target_id": "n4",  "branch_label": "fail"}
    ]
  }' | jq -r '.id')

RUN3=$(curl -s -X POST $BASE/workflows/$WF3/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')

sleep 8

curl -s $BASE/runs/$RUN3 | jq '{status, node_results: (.node_results | keys)}'
```

**Expected keys in `node_results`:**

```json
{
  "status": "succeeded",
  "node_results": ["n1", "n2", "n3"]
}
```

`n4` (the error branch) is **absent** — it was pruned by the conditional and never dispatched. The `RunDetailPage` shows `n4` with a gray **"skipped"** badge while `n1`, `n2`, and `n3` show green **"succeeded"** badges.

---

## Verify in the UI

1. Open `http://localhost:3000` and navigate to the workflow.
2. Click **Run History** → click any of the runs above.
3. Observe in the `RunDetailPage`:
   - Scenario 1: all nodes green.
   - Scenario 2: `n1` red (failed, expandable error); `n2` gray (skipped).
   - Scenario 3: `n1`, `n2`, `n3` green; `n4` gray (skipped).
4. Expand any **succeeded** node row to see its output JSON.
5. Expand any **failed** node row to see the error message.

---

## Backward compatibility check

Runs recorded before migration 0014 have `node_results: null` in the DB. The `RunDetailPage` falls back to the old heuristic (nodes present in `final_output` → succeeded, everything else → unknown gray) automatically — no action required.
