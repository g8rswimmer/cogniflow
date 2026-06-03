#!/usr/bin/env bash
# =============================================================================
# Demo: M9 gRPC Plugin Protocol
#
# Demonstrates:
#   1. Plugin registration  — echo.passthrough appears in GET /node-types
#   2. Echo passthrough     — plugin node returns its upstream_data as output
#   3. Pipeline integration — HTTP Request → echo.passthrough; plugin sees n1 output
#   4. Fan-out + echo       — two parallel echo nodes feed into a Merge; shows
#                             that DirectPredecessorIDs keeps ancestors clean
#
# Prerequisites:
#   - jq installed:  brew install jq
#   - Server built:  cd backend && go build ./cmd/server
#   - Echo plugin:   cd examples/plugins/echo && go build -o echo .
#
# Quickstart (two terminals):
#
#   Terminal 1 — start the echo plugin:
#     cd examples/plugins/echo && go run .
#     (listens on :50051 by default; override with --port NNNN)
#
#   Terminal 2 — start the server with the plugin address:
#     DB_DSN="..." COGNIFLOW_ENCRYPTION_KEY="..." \
#     PLUGIN_ADDRESSES=localhost:50051 go run ./cmd/server
#
#   Terminal 3 — run this demo:
#     bash demos/m9_grpc_plugin_demo.sh
#
# Override defaults with environment variables:
#   BASE_URL     — API base (default http://localhost:8080/v1)
#   PLUGIN_PORT  — port the echo plugin is listening on (default 50051)
# =============================================================================

set -euo pipefail

BASE="${BASE_URL:-http://localhost:8080/v1}"
PLUGIN_PORT="${PLUGIN_PORT:-50051}"
PLUGIN_TYPE_ID="echo.passthrough"

# ── helpers ───────────────────────────────────────────────────────────────────

check() {
  command -v "$1" >/dev/null 2>&1 || { echo "ERROR: $1 is required. Install with: brew install $1"; exit 1; }
}

wait_for_run() {
  local run_id=$1
  local timeout=${2:-30}
  local elapsed=0
  while [ $elapsed -lt $timeout ]; do
    status=$(curl -s "$BASE/runs/$run_id" | jq -r '.status')
    case "$status" in
      succeeded|failed) echo "$status"; return ;;
    esac
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo "timeout"
}

assert_succeeded() {
  local run_id=$1
  local label=$2
  local status
  status=$(wait_for_run "$run_id" 30)
  if [ "$status" != "succeeded" ]; then
    echo "FAILED: $label (run $run_id status=$status)"
    curl -s "$BASE/runs/$run_id" | jq '.error_detail'
    exit 1
  fi
}

CREATED_WORKFLOWS=()

create_workflow() {
  local body=$1
  local id
  id=$(curl -s -X POST "$BASE/workflows" \
    -H "Content-Type: application/json" \
    -d "$body" | jq -r '.id')
  if [ -z "$id" ] || [ "$id" = "null" ]; then
    echo "ERROR: workflow creation failed. Response:"
    curl -s -X POST "$BASE/workflows" -H "Content-Type: application/json" -d "$body" | jq .
    exit 1
  fi
  CREATED_WORKFLOWS+=("$id")
  echo "$id"
}

cleanup() {
  if [ ${#CREATED_WORKFLOWS[@]} -eq 0 ]; then return; fi
  echo ""
  echo "── Cleanup ──────────────────────────────────────────────────────"
  for wf_id in "${CREATED_WORKFLOWS[@]}"; do
    curl -s -X DELETE "$BASE/workflows/$wf_id" >/dev/null
    echo "  deleted workflow $wf_id"
  done
}
trap cleanup EXIT

# ── preflight ─────────────────────────────────────────────────────────────────

check jq
check curl

echo ""
echo "══════════════════════════════════════════════════════════════════"
echo " cogniflow M9 — gRPC Plugin Protocol demo"
echo "══════════════════════════════════════════════════════════════════"

# Check server health
health=$(curl -sf "http://localhost:8080/health" 2>/dev/null | jq -r '.status' 2>/dev/null || echo "")
if [ "$health" != "ok" ]; then
  echo ""
  echo "ERROR: Server is not reachable at http://localhost:8080"
  echo ""
  echo "Start the server with the echo plugin address:"
  echo "  Terminal 1:  cd examples/plugins/echo && go run ."
  echo "  Terminal 2:  DB_DSN=\"...\" COGNIFLOW_ENCRYPTION_KEY=\"...\" \\"
  echo "               PLUGIN_ADDRESSES=localhost:$PLUGIN_PORT go run ./cmd/server"
  exit 1
fi

# Check that the echo plugin is registered
plugin_registered=$(curl -s "$BASE/node-types" | jq -r \
  "[.node_types[] | select(.type_id == \"$PLUGIN_TYPE_ID\")] | length")

if [ "$plugin_registered" = "0" ]; then
  echo ""
  echo "ERROR: Plugin '$PLUGIN_TYPE_ID' is not registered."
  echo ""
  echo "The server must be started with PLUGIN_ADDRESSES pointing to the echo plugin."
  echo "Steps:"
  echo ""
  echo "  1. Start the echo plugin (in a separate terminal):"
  echo "       cd examples/plugins/echo && go run . --port $PLUGIN_PORT"
  echo ""
  echo "  2. Restart the server with PLUGIN_ADDRESSES:"
  echo "       PLUGIN_ADDRESSES=localhost:$PLUGIN_PORT \\"
  echo "       DB_DSN=\"...\" COGNIFLOW_ENCRYPTION_KEY=\"...\" \\"
  echo "       go run ./cmd/server"
  echo ""
  echo "  3. Re-run this script."
  exit 1
fi

echo ""
echo "✓ Server is healthy"
echo "✓ Plugin '$PLUGIN_TYPE_ID' is registered"

# ─────────────────────────────────────────────────────────────────────────────
# Demo 1 — Plugin appears in the node-types registry
#
# The registrar called Meta() on the echo plugin at startup and stored its
# NodeMeta in the registry. GET /node-types should return it with category=plugin.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 1: Plugin registration — node-types catalogue ==="
echo ""
echo "GET /v1/node-types | select echo.passthrough"
echo ""

curl -s "$BASE/node-types" | jq '
  .node_types[]
  | select(.type_id == "echo.passthrough")
  | {type_id, display_name, category, description}
'

# ─────────────────────────────────────────────────────────────────────────────
# Demo 2 — Single echo node with initial_data
#
# Workflow:  _initial → echo.passthrough
#
# The echo plugin receives upstream_data (which contains _initial) and returns
# it unchanged. The run output should contain the initial_data fields.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 2: Single echo node — initial_data passthrough ==="
echo ""
echo "Workflow:  initial_data → echo.passthrough (returns upstream_data as-is)"
echo ""

ECHO_WF=$(create_workflow '{
  "name": "Demo M9 - Echo passthrough",
  "trigger": {"kind": "manual"},
  "timeout_seconds": 15,
  "nodes": [{
    "id": "n1",
    "type_id": "echo.passthrough",
    "label": "Echo node",
    "position": {"x": 0, "y": 0},
    "config": {}
  }],
  "edges": []
}')

echo "Workflow: $ECHO_WF"

RUN2=$(curl -s -X POST "$BASE/workflows/$ECHO_WF/runs" \
  -H "Content-Type: application/json" \
  -d '{
    "initial_data": {
      "order_id": "ORD-9981",
      "customer": "Alice",
      "amount":   149.99
    }
  }' | jq -r '.run_id')

echo "Run: $RUN2 — waiting..."
assert_succeeded "$RUN2" "echo passthrough"

echo ""
echo "Plugin output (upstream_data echoed back — should include _initial fields):"
curl -s "$BASE/runs/$RUN2" | jq '{
  echoed_order_id:  .final_output.n1."_initial".order_id,
  echoed_customer:  .final_output.n1."_initial".customer,
  echoed_amount:    .final_output.n1."_initial".amount
}'

# ─────────────────────────────────────────────────────────────────────────────
# Demo 3 — HTTP Request → echo.passthrough
#
# Workflow:
#   n1 (http.request) — GET https://httpbin.org/get?demo=m9
#   n2 (echo.passthrough) — receives n1.status_code, n1.body, and _initial
#
# The plugin node is indistinguishable from a built-in to the engine.
# The echo plugin's output shows that it received the full upstream context
# (both _initial and n1's output), proving that UpstreamData is correctly
# serialised and sent over the gRPC ExecuteRequest.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 3: HTTP Request → echo.passthrough (plugin in a pipeline) ==="
echo ""
echo "Workflow:  http.request → echo.passthrough"
echo "           Verifies plugin receives upstream node output over gRPC"
echo ""

PIPELINE_WF=$(create_workflow '{
  "name": "Demo M9 - HTTP into Echo",
  "trigger": {"kind": "manual"},
  "timeout_seconds": 30,
  "nodes": [
    {
      "id": "n1",
      "type_id": "http.request",
      "label": "Fetch anything",
      "position": {"x": 0, "y": 0},
      "config": {
        "url":    "https://httpbin.org/get?demo=m9&source={{._initial.source}}",
        "method": "GET"
      }
    },
    {
      "id": "n2",
      "type_id": "echo.passthrough",
      "label": "Echo upstream",
      "position": {"x": 400, "y": 0},
      "config": {}
    }
  ],
  "edges": [
    {"id": "e1", "source_id": "n1", "target_id": "n2", "branch_label": null}
  ]
}')

echo "Workflow: $PIPELINE_WF"

RUN3=$(curl -s -X POST "$BASE/workflows/$PIPELINE_WF/runs" \
  -H "Content-Type: application/json" \
  -d '{"initial_data": {"source": "cogniflow-demo"}}' | jq -r '.run_id')

echo "Run: $RUN3 — waiting for HTTP call + plugin execution..."
assert_succeeded "$RUN3" "pipeline with plugin"

echo ""
echo "HTTP node output (n1):"
curl -s "$BASE/runs/$RUN3" | jq '{
  n1_status_code: .final_output.n1.status_code,
  n1_has_body:    (.final_output.n1.body | type == "string")
}'

echo ""
echo "Echo plugin output (n2) — should contain n1 AND _initial in the echoed map:"
curl -s "$BASE/runs/$RUN3" | jq '{
  plugin_saw_n1_status: .final_output.n2.n1.status_code,
  plugin_saw_source:    .final_output.n2."_initial".source,
  all_keys_in_echo:     (.final_output.n2 | keys)
}'

# ─────────────────────────────────────────────────────────────────────────────
# Demo 4 — Fan-out → two echo nodes → Merge
#
# Workflow:
#   n1 (http.request) → httpbin.org/uuid  (extracts uuid via output_parser)
#   ├─ n2 (echo.passthrough) — echoes {_initial, n1} back as "left_echo"
#   └─ n3 (echo.passthrough) — echoes {_initial, n1} back as "right_echo"
#   n4 (merge) — waits for both, merges outputs deterministically
#
# Demonstrates:
#   - Two plugin nodes running concurrently (engine fan-out)
#   - Merge receives both plugin outputs and flattens them
#   - Each echo receives the full ancestor chain (n1's output + _initial)
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 4: Fan-out into two plugin nodes → Merge ==="
echo ""
echo "Workflow:  http.request → [echo, echo] → merge"
echo "           Two plugin nodes run concurrently; Merge waits for both"
echo ""

FANOUT_WF=$(create_workflow '{
  "name": "Demo M9 - Fan-out echo + Merge",
  "trigger": {"kind": "manual"},
  "timeout_seconds": 30,
  "nodes": [
    {
      "id": "n1",
      "type_id": "http.request",
      "label": "Fetch UUID",
      "position": {"x": 0, "y": 0},
      "config": {
        "url":    "https://httpbin.org/uuid",
        "method": "GET"
      },
      "output_parsers": {
        "uuid": {
          "kind":    "json_path",
          "source":  "body",
          "pattern": "uuid"
        }
      }
    },
    {
      "id": "n2",
      "type_id": "echo.passthrough",
      "label": "Left echo",
      "position": {"x": 400, "y": -100},
      "config": {}
    },
    {
      "id": "n3",
      "type_id": "echo.passthrough",
      "label": "Right echo",
      "position": {"x": 400, "y": 100},
      "config": {}
    },
    {
      "id": "n4",
      "type_id": "merge",
      "label": "Merge outputs",
      "position": {"x": 800, "y": 0}
    }
  ],
  "edges": [
    {"id": "e1", "source_id": "n1", "target_id": "n2", "branch_label": null},
    {"id": "e2", "source_id": "n1", "target_id": "n3", "branch_label": null},
    {"id": "e3", "source_id": "n2", "target_id": "n4", "branch_label": null},
    {"id": "e4", "source_id": "n3", "target_id": "n4", "branch_label": null}
  ]
}')

echo "Workflow: $FANOUT_WF"

RUN4=$(curl -s -X POST "$BASE/workflows/$FANOUT_WF/runs" \
  -H "Content-Type: application/json" \
  -d '{"initial_data": {"demo": "m9-fanout"}}' | jq -r '.run_id')

echo "Run: $RUN4 — waiting for concurrent plugin nodes..."
assert_succeeded "$RUN4" "fan-out echo + merge"

echo ""
echo "UUID extracted by n1 (output_parser):"
curl -s "$BASE/runs/$RUN4" | jq '.final_output.n1.uuid'

echo ""
echo "n2 and n3 each ran the echo plugin — both should have seen n1.uuid:"
curl -s "$BASE/runs/$RUN4" | jq '{
  n2_saw_n1_uuid:  .final_output.n2.n1.uuid,
  n3_saw_n1_uuid:  .final_output.n3.n1.uuid
}'

echo ""
echo "Merge output (n4) — flattened from n2 and n3 (last alphabetically wins on key collision):"
curl -s "$BASE/runs/$RUN4" | jq '{
  merged_keys:  (.final_output.n4 | keys),
  n1_uuid_via_merge: .final_output.n4.n1.uuid
}'

echo ""
echo "══════════════════════════════════════════════════════════════════"
echo " All demos complete."
echo ""
echo " Summary:"
echo "   Demo 1 — echo.passthrough is visible in GET /v1/node-types"
echo "   Demo 2 — plugin node receives and returns initial_data"
echo "   Demo 3 — plugin node in a pipeline sees upstream HTTP output"
echo "   Demo 4 — two plugin nodes run concurrently; Merge waits for both"
echo "══════════════════════════════════════════════════════════════════"
