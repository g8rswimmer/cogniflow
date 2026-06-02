# cogniflow — Project Plan

> **Status:** Draft v0.2
> **Last Updated:** 2026-06-01
> **References:** REQUIREMENTS.md · ARCHITECTURE.md

Each milestone leaves the system in a **runnable, verifiable state**. Later milestones build on earlier ones. No milestone is purely internal — every one can be exercised by starting the system and observing real behaviour.

---

## Milestone Overview

| # | Milestone | Key Capability Unlocked |
|---|-----------|------------------------|
| M1 | [Infrastructure & Scaffold](#m1-infrastructure--scaffold) | `docker-compose up` starts cleanly; health endpoint responds |
| M2 | [Workflow CRUD & Node Registry](#m2-workflow-crud--node-registry) | Workflows can be created, saved, and retrieved via API |
| M3 | [Execution Engine & Manual Trigger](#m3-execution-engine--manual-trigger) | Workflows with HTTP Request nodes run end-to-end; results in DB |
| M4 | [Real-Time Run Events](#m4-real-time-run-events) | Per-node status streams live over WebSocket; run history queryable |
| M5 | [Trigger System](#m5-trigger-system) | Workflows fire automatically via webhook and cron |
| M6 | [AI Nodes](#m6-ai-nodes) | LLM Call and Embedding nodes work in live workflows |
| M7 | [RAG Nodes](#m7-rag-nodes) | Documents ingestable; vector similarity search returns relevant chunks |
| M8 | [Advanced Deterministic Nodes](#m8-advanced-deterministic-nodes) | Conditional branching, data transforms, database read/write, parallel merge |
| M9 | [gRPC Plugin Protocol](#m9-grpc-plugin-protocol) | External plugin processes register and execute as first-class nodes |
| M10 | [Frontend — Canvas & CRUD](#m10-frontend--canvas--crud) | Browser UI: create workflows, add/connect nodes, save |
| M11 | [Frontend — Run & Observe](#m11-frontend--run--observe) | Browser UI: trigger runs, watch live status, browse history |
| M12 | [Production Build & Hardening](#m12-production-build--hardening) | Single `docker-compose up` from a clean clone; full system E2E |

---

## M1: Infrastructure & Scaffold

**Goal:** Establish the monorepo layout, database connection, schema migrations, and a live health endpoint so that every subsequent milestone has a stable foundation to build on.

### Deliverables

- Monorepo directory structure (`backend/`, `frontend/`, `docker-compose.yml`, `.env.example`, `.gitignore`)
- `backend/go.mod` initialised (`module github.com/g8rswimmer/cogniflow`)
- `docker-compose.yml` with `mysql` and `backend` services; MySQL 9.0 healthcheck
- `backend/internal/store/mysql/db.go` — `*sqlx.DB` init, ping, and `golang-migrate` bootstrap
- Migration `0001_create_workflows.up.sql` — `workflows` table only (minimal schema to prove migrations run)
- `backend/internal/api/health_handler.go` — `GET /health` returns `{"status":"ok","db":"ok"}`
- `backend/cmd/server/main.go` — wires DB + router, starts on `PORT` env var
- `backend/Dockerfile` — multi-stage Go build

### Testable Criteria

```bash
# Start services
docker-compose up --build

# Health check responds
curl http://localhost:8080/health
# → {"status":"ok","db":"ok","uptime":12}

# MySQL is reachable and migrations ran
docker-compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SHOW TABLES;"
# → workflows
```

### Dependencies
None — this is the starting point.

---

## M2: Workflow CRUD & Node Registry

**Goal:** The core data model is fully persisted and retrievable. Developers can define node types, register them, and have them appear in the API. Workflow graph validation (cycle detection) is enforced at save time.

### Deliverables

- Remaining schema migrations:
  - `0002_create_workflow_nodes_edges.up.sql` — `workflow_nodes`, `workflow_edges`, `node_configs`
  - `0003_create_runs.up.sql` — `runs` table (status/output columns only; used in M3)
- `backend/internal/node/handler.go` — `NodeHandler` interface, `NodeMeta` struct (with `json.RawMessage` schemas)
- `backend/internal/node/registry.go` — `NodeRegistry` implementation (thread-safe map)
- `backend/internal/crypto/encrypt.go` + `config_vault.go` — AES-256-GCM encrypt/decrypt; reads `COGNIFLOW_ENCRYPTION_KEY`
- `backend/internal/store/mysql/workflow_store.go` — full workflow CRUD SQL (nodes + edges + configs in one transaction)
- `backend/internal/engine/dag.go` — `CycleDetect()` used at save time; `Build()` ready for M3
- `backend/internal/api/workflow_handler.go` — `GET/POST/PUT/DELETE /workflows`
- `backend/internal/api/nodetype_handler.go` — `GET /node-types`
- First built-in node registered: **HTTP Request** (`http.request`) — `Meta()` only, `Execute()` stubbed to return an empty output (full implementation in M3)
- `"x-template": true` added to `url`, `body`, and header value properties in the `http.request` input schema (see ARCHITECTURE.md §11)

### Testable Criteria

```bash
# Register an HTTP Request node type
curl http://localhost:8080/node-types | jq '.node_types[].type_id'
# → "http.request"

# Create a workflow with two nodes and one edge
curl -s -X POST http://localhost:8080/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Test Flow",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [
      {"id":"n1","type_id":"http.request","label":"Step 1","position":{"x":0,"y":0},"config":{"url":"https://httpbin.org/get","method":"GET"}},
      {"id":"n2","type_id":"http.request","label":"Step 2","position":{"x":200,"y":0},"config":{"url":"https://httpbin.org/get","method":"GET"}}
    ],
    "edges": [{"id":"e1","source_id":"n1","target_id":"n2","branch_label":null}]
  }' | jq '.id'
# → "550e8400-..."

# Retrieve it
curl http://localhost:8080/workflows/550e8400-... | jq '.name'
# → "Test Flow"

# Cycle detection rejects a cyclic graph
curl -s -X POST http://localhost:8080/workflows \
  -H 'Content-Type: application/json' \
  -d '{...edges: n1→n2 AND n2→n1...}' | jq '.error.code'
# → "CYCLE_DETECTED"

# Sensitive config is masked in responses
# api_key stored encrypted; returned as "***"
```

### Dependencies
M1

---

## M3: Execution Engine & Manual Trigger

**Goal:** A workflow containing HTTP Request nodes can be triggered manually, execute all nodes in DAG order (including concurrent parallel branches), and persist its final output. This is the first fully operational end-to-end path.

### Deliverables

- `backend/internal/node/builtin/http_request/handler.go` — full `Execute()` implementation (templated URL/headers/body, returns `status_code`, `body`, `headers`)
- `backend/internal/engine/context.go` — `ExecutionContext` (thread-safe output map)
- `backend/internal/engine/event.go` — `NodeEvent` struct and `EventBus` (used in M4; wired but no WebSocket yet)
- `backend/internal/engine/runner.go` — goroutine fan-out, `readyQueue`, `pendingCount`, `resultCh`, retry logic
- `backend/internal/engine/engine.go` — `WorkflowEngine.Run()` + `Dispatch()`; persists run record; updates final status
- `backend/internal/store/mysql/run_store.go` — `CreateRun`, `UpdateRunStatus`, `GetRun`, `ListRuns`
- `backend/internal/api/run_handler.go` — `POST /workflows/:id/runs` (manual trigger), `GET /runs/:run_id`, `GET /workflows/:id/runs`
- `backend/internal/trigger/types.go` — `RunRequest`, `Dispatcher` interface
- Template validation at workflow save (`validateTemplates()` in `workflow_handler.go`): for every node, any `x-template: true` config field containing `{{` is parsed with `text/template`; parse errors return `VALIDATION_FAILED`

### Testable Criteria

```bash
# Trigger a 2-node sequential workflow
RUN=$(curl -s -X POST http://localhost:8080/workflows/$WF_ID/runs \
  -H 'Content-Type: application/json' \
  -d '{"initial_data":{"msg":"hello"}}' | jq -r '.run_id')

# Poll until done (or sleep 3s)
curl http://localhost:8080/runs/$RUN | jq '{status, final_output}'
# → {"status":"succeeded","final_output":{"n2":{"status_code":200,"body":"..."}}}

# Verify template substitution: n1 returns {"path":"items"}; n2 URL is "https://httpbin.org/anything/{{.n1.path}}"
curl http://localhost:8080/runs/$RUN2 | jq '.final_output.n2.status_code'
# → 200  (URL correctly resolved to /anything/items)

# Verify invalid templates are rejected at save time
curl -s -X POST http://localhost:8080/workflows \
  -H 'Content-Type: application/json' \
  -d '{"name":"Bad","nodes":[{"id":"n1","type_id":"http.request","position":{"x":0,"y":0},
      "config":{"url":"{{.broken","method":"GET"}}],"edges":[]}' | jq '.error.code'
# → "VALIDATION_FAILED"

# Verify a parallel-branch workflow (fan-out from n1 to n2 and n3, merge at n4)
# Both n2 and n3 should execute concurrently; n4 waits for both
# All four nodes appear as succeeded in the run record

# Verify failure propagation: workflow with a node pointing to an unreachable URL
# → run status = "failed"; downstream nodes never execute
```

### Dependencies
M1, M2

---

## M4: Real-Time Run Events

**Goal:** Clients can subscribe to a WebSocket endpoint and receive per-node status events as a workflow executes, instead of polling. Run history is fully queryable.

### Deliverables

- `backend/internal/api/ws_handler.go` — WebSocket upgrade on `GET /runs/:run_id/events`; subscribes to `EventBus`; streams `NodeEvent` JSON frames; closes on `run.succeeded` / `run.failed`
- `EventBus.Subscribe()` / `Publish()` wired into `runner.go` — events emitted at each node state transition
- `GET /workflows/:id/runs` query params: `status`, `since`, `until`, `limit`

### Testable Criteria

```bash
# Open a WebSocket connection (using wscat or websocat)
wscat -c ws://localhost:8080/runs/$RUN_ID/events

# Trigger the workflow in a second terminal
curl -X POST http://localhost:8080/workflows/$WF_ID/runs \
  -H 'Content-Type: application/json' -d '{}'

# WebSocket output (one frame per transition):
# {"run_id":"...","node_id":"n1","type":"node.running","timestamp":"..."}
# {"run_id":"...","node_id":"n1","type":"node.succeeded","timestamp":"...","output":{...}}
# {"run_id":"...","node_id":"n2","type":"node.running","timestamp":"..."}
# {"run_id":"...","node_id":"n2","type":"node.succeeded","timestamp":"...","output":{...}}
# {"run_id":"...","node_id":"","type":"run.succeeded","timestamp":"..."}
# (connection closes)

# Run history query
curl 'http://localhost:8080/workflows/$WF_ID/runs?status=succeeded&limit=5' \
  | jq '.runs | length'
# → 1 (or more after multiple runs)
```

### Dependencies
M1, M2, M3

---

## M5: Trigger System

**Goal:** Workflows fire automatically from two external sources: inbound HTTP webhooks (per-workflow stable URL) and cron schedules. The trigger system loads on startup and stays live without restart when workflows are updated.

### Deliverables

- `backend/internal/trigger/manager.go` — `TriggerManager`: `LoadAll()`, `Upsert()`, `Remove()`
- `backend/internal/trigger/webhook.go` — per-workflow route registered on chi sub-router; `POST /webhooks/:workflow_id` dispatches a run and returns `202 {"run_id":"..."}`
- `backend/internal/trigger/cron.go` — `robfig/cron` v3 wrapper; schedules loaded from DB at startup; `Add` / `Remove` update the live scheduler
- `PUT /workflows/:id` wires trigger upsert: saving a workflow with `trigger.kind = "cron"` arms the scheduler; changing to `"manual"` removes it
- `backend/internal/store/mysql/workflow_store.go` — `SaveTriggerConfig`, `GetTriggerConfig`, `ListTriggerConfigs`

### Testable Criteria

```bash
# --- Webhook trigger ---
# Update workflow trigger to webhook
curl -X PUT http://localhost:8080/workflows/$WF_ID \
  -d '{"trigger":{"kind":"webhook"}, ...}' | jq '.trigger.webhook_url'
# → "/webhooks/550e8400-..."

# Fire it externally
curl -X POST http://localhost:8080/webhooks/$WF_ID \
  -H 'Content-Type: application/json' \
  -d '{"customer_id": 42}'
# → 202 {"run_id":"abc..."}

curl http://localhost:8080/runs/abc... | jq '.status'
# → "succeeded"

# --- Cron trigger ---
# Set a workflow to run every minute
curl -X PUT http://localhost:8080/workflows/$WF_ID2 \
  -d '{"trigger":{"kind":"cron","cron_expr":"* * * * *"}, ...}'

# Wait 60–70 seconds, then check run history
curl "http://localhost:8080/workflows/$WF_ID2/runs?limit=3" | jq '.runs[].triggered_by'
# → "cron"
# → "cron"
```

### Dependencies
M1, M2, M3

---

## M6: AI Nodes

**Goal:** LLM Call and Embedding nodes are fully operational in live workflows. AI provider clients are behind an interface so OpenAI and Anthropic are interchangeable by config. Nodes support post-execution output parsers so structured data can be extracted from LLM completions and made available to downstream nodes.

### Deliverables

- `backend/internal/aiprovider/provider.go` — `LLMClient` and `EmbeddingClient` interfaces; `LLMRequest.Temperature` is `*float64` (nil = omit, pointer = send exact value including 0 for greedy sampling)
- `backend/internal/aiprovider/openai/client.go` — OpenAI chat completions + embeddings; temperature omitted from request when nil
- `backend/internal/aiprovider/anthropic/client.go` — Anthropic Messages API; returns error when response contains no text content block
- `backend/internal/node/builtin/llm/handler.go` — single `Handler` backing `llm.openai` and `llm.anthropic`; expands `prompt` and `system_msg` via `renderTemplate()` before calling `LLMClient`; returns `completion`, `prompt_tokens`, `completion_tokens`
- `backend/internal/node/builtin/embedding/handler.go` — `embedding.openai` node; `input` field is `"x-template": true`; calls `EmbeddingClient`; returns `embedding` (float32 slice as `[]any`)
- All three nodes registered in `main.go` with full `input_schema` / `output_schema`; `prompt`, `system_msg`, and `input` fields carry `"x-template": true`
- `backend/internal/node/outputparser/parser.go` — `Apply()` (merges extracted fields into output), `Validate()` / `ValidateAll()` (save-time checks); supports `json_path` (gjson dot-path) and `regex` (with `capture_group`)
- `store.OutputParser` struct and `store.WorkflowNode.OutputParsers` field — serialised as `output_parsers JSON` column (migration `0004_add_output_parsers`)
- Engine `runner.go` — calls `outputparser.Apply(out.Data, n.OutputParsers)` after `executeWithRetry()` succeeds; augmented map stored in `ExecutionContext` and published on `EventBus`
- `validateOutputParsers()` in `workflow_handler.go` — called on `POST/PUT /workflows`; returns `VALIDATION_FAILED` for invalid kind, empty json_path pattern, bad regex, or negative capture_group
- CEL expression validation deferred to M8 (not added in M6)

### Testable Criteria

```bash
# Workflow: manual trigger → LLM Call node
curl -X POST http://localhost:8080/workflows \
  -d '{
    "name":"LLM Test",
    "trigger":{"kind":"manual"},
    "timeout_seconds":30,
    "nodes":[{
      "id":"n1","type_id":"llm.openai","label":"Ask GPT",
      "config":{
        "api_key":"sk-...",
        "model":"gpt-4o-mini",
        "prompt":"Say hello in one sentence."
      }
    }],
    "edges":[]
  }'

RUN=$(curl -sX POST http://localhost:8080/workflows/$WF_ID/runs -d '{}' | jq -r '.run_id')
sleep 5
curl http://localhost:8080/runs/$RUN | jq '.final_output.n1.completion'
# → "Hello! How can I assist you today?"

# Prompt template: n1 (HTTP Request) feeds n2 (LLM Call)
# n2 prompt = "Using user ID {{._initial.user_id}} check for account compromise"
RUN2=$(curl -sX POST http://localhost:8080/workflows/$WF_ID2/runs \
  -d '{"initial_data":{"user_id":"42"}}' | jq -r '.run_id')
sleep 5
curl http://localhost:8080/runs/$RUN2 | jq '.final_output.n1.completion'
# → a response mentioning user 42 (confirms {{._initial.user_id}} resolved)

# Output parser: LLM returns JSON; parser extracts a field for downstream use
# n1 config: output_parsers: {"account_status": {"kind":"json_path","source":"completion","pattern":"status"}}
# n1 completion = '{"status":"compromised","risk":0.9}'
curl http://localhost:8080/runs/$RUN3 | jq '.final_output.n1.account_status'
# → "compromised"

# Output parser: regex extraction with capture group
# parser: {"user_id": {"kind":"regex","source":"completion","pattern":"user_id: (\\d+)","capture_group":1}}
# n1 completion = "Account review complete. user_id: 99 flagged."
curl http://localhost:8080/runs/$RUN4 | jq '.final_output.n1.user_id'
# → "99"

# Invalid output parser rejected at save time
curl -sX POST http://localhost:8080/workflows \
  -d '{"nodes":[{"output_parsers":{"x":{"kind":"regex","source":"completion","pattern":"[broken","capture_group":0}}}],...}'
  | jq '.error.code'
# → "VALIDATION_FAILED"

# Embedding node returns a vector
RUN5=$(curl -sX POST http://localhost:8080/workflows/$EMBED_WF_ID/runs -d '{}' | jq -r '.run_id')
sleep 5
curl http://localhost:8080/runs/$RUN5 | jq '.final_output.n1.embedding | length'
# → 1536
```

### Dependencies
M1, M2, M3, M4

---

## M7: RAG Nodes

**Goal:** Documents can be chunked, embedded, and stored in MySQL using the native `VECTOR` column. A RAG Retrieve node can find the most relevant chunks for a given query using `VEC_DISTANCE_COSINE`, making retrieval-augmented generation workflows possible.

### Deliverables

- Migration `0003_create_rag.up.sql` — `rag_documents` and `rag_chunks` tables with `VECTOR(1536)` column and `VECTOR INDEX`
- `backend/internal/store/mysql/rag_store.go` — `UpsertChunks()` (batch insert), `SearchChunks()` (`VEC_DISTANCE_COSINE` query)
- `backend/internal/node/builtin/rag_ingest/handler.go` — chunks input text, calls Embedding node's provider, upserts to `rag_chunks`
- `backend/internal/node/builtin/rag_retrieve/handler.go` — embeds query, calls `SearchChunks()`, returns top-K chunks with scores
- Both nodes registered with schemas

### Testable Criteria

```bash
# Workflow 1: Ingest — manual trigger → RAG Ingest node
# (ingests a passage about cogniflow)
curl -X POST http://localhost:8080/workflows/$INGEST_WF/runs \
  -d '{"initial_data":{"text":"cogniflow is a workflow orchestration platform..."}}'

# Workflow 2: Retrieve — manual trigger → RAG Retrieve node
RUN=$(curl -sX POST http://localhost:8080/workflows/$RETRIEVE_WF/runs \
  -d '{"initial_data":{"query":"What is cogniflow?"}}' | jq -r '.run_id')
sleep 5

curl http://localhost:8080/runs/$RUN | jq '.final_output.n1.chunks[0].chunk_text'
# → "cogniflow is a workflow orchestration platform..."

curl http://localhost:8080/runs/$RUN | jq '.final_output.n1.chunks[0].score'
# → 0.04  (cosine distance — lower is more similar)
```

### Dependencies
M1, M2, M3, M6 (for the embedding provider)

---

## M8: Advanced Deterministic Nodes

**Goal:** The full built-in node library is complete. Conditional branching with CEL expressions enables complex routing logic. Data transform, database read/write, and the Merge fan-in node cover the remaining deterministic use cases.

### Deliverables

- `backend/internal/node/builtin/conditional/handler.go` — CEL compile at save (`ValidateExpression`), evaluate at run time; engine routes on `result` boolean
- `backend/internal/node/builtin/data_transform/handler.go` — JSON template expansion using Go `text/template` or `gval`; returns transformed data map
- `backend/internal/node/builtin/db_query/handler.go` — parameterised `SELECT` against a caller-configured DSN; returns rows as `[]map[string]any`
- `backend/internal/node/builtin/db_write/handler.go` — parameterised `INSERT` / `UPDATE` / `DELETE`; returns `rows_affected`
- `backend/internal/node/builtin/merge/handler.go` — identity node; engine already handles fan-in synchronisation; `Execute()` is a no-op passthrough
- Engine `runner.go` updated: Conditional node's `branch_label` (`"true"` / `"false"`) used to filter which successor edges are pushed to `readyQueue`

### Testable Criteria

```bash
# Conditional branching workflow:
#   n1 (HTTP Request) → n2 (Conditional: ctx["n1"]["status_code"] == 200)
#   true branch → n3 (LLM Call: "Request succeeded")
#   false branch → n4 (LLM Call: "Request failed")

# Point n1 at httpbin.org/status/200 → n3 should run, n4 should not
RUN=$(curl -sX POST http://localhost:8080/workflows/$WF_ID/runs -d '{}' | jq -r '.run_id')
sleep 5
curl http://localhost:8080/runs/$RUN | jq '.final_output | keys'
# → ["n1","n2","n3"]  (n4 absent — was never scheduled)

# CEL validation rejects a non-boolean expression at save time
curl -X PUT http://localhost:8080/workflows/$WF_ID -d '{...conditional config: "1 + 1"...}' \
  | jq '.error.code'
# → "VALIDATION_FAILED"

# Parallel merge: n1 fans out to n2 and n3, both feed n4 (Merge)
# n2 and n3 should execute concurrently; n4 receives merged output of both
```

### Dependencies
M1, M2, M3, M4

---

## M9: gRPC Plugin Protocol

**Goal:** An out-of-process node extension written in any language can be connected to cogniflow at startup, appear in the node palette, and execute within a workflow — indistinguishable from a built-in node to the engine and to the frontend.

### Deliverables

- `backend/proto/plugin/v1/plugin.proto` — `NodePlugin` service with `Meta` and `Execute` RPCs
- Generated `plugin.pb.go` and `plugin_grpc.pb.go`
- `backend/internal/node/plugin/grpc_proxy.go` — `NodeHandler` adapter wrapping a gRPC client stub
- `backend/internal/node/plugin/registrar.go` — reads `PLUGIN_ADDRESSES`, dials each, calls `Meta()`, registers proxy
- Example plugin: `examples/plugins/echo/main.go` — a minimal Go gRPC server that implements `NodePlugin`; returns the input `upstream_data` as its output (useful for testing)

### Testable Criteria

```bash
# Start the echo plugin on port 50051
cd examples/plugins/echo && go run main.go --port 50051 &

# Start the backend with PLUGIN_ADDRESSES set
PLUGIN_ADDRESSES=localhost:50051 docker-compose up backend

# Plugin appears in the node registry
curl http://localhost:8080/node-types | jq '[.node_types[] | select(.category=="plugin")]'
# → [{"type_id":"echo.passthrough","display_name":"Echo","category":"plugin",...}]

# Use the plugin in a workflow
# n1 (HTTP Request) → n2 (echo.passthrough)
RUN=$(curl -sX POST http://localhost:8080/workflows/$WF_ID/runs -d '{}' | jq -r '.run_id')
sleep 3
curl http://localhost:8080/runs/$RUN | jq '.final_output.n2'
# → the upstream data from n1 echoed back
```

### Dependencies
M1, M2, M3

---

## M10: Frontend — Canvas & CRUD

**Goal:** A browser-based UI allows users to create, open, configure, and save workflows. The node palette is populated from the live API. Node configuration forms are dynamically generated from each node's `input_schema`.

### Deliverables

- `frontend/` scaffolded: Vite + React 18 + TypeScript, Tailwind CSS, React Router v6, Zustand
- `src/api/client.ts` — base fetch client (Content-Type, base URL from `VITE_API_BASE`)
- `src/hooks/useApi.ts` — typed wrappers for all REST endpoints used in this milestone
- `src/stores/useWorkflowStore.ts` — canvas nodes, edges, per-node configs, dirty flag
- `src/stores/useNodeTypeStore.ts` — fetches and caches `GET /node-types` at startup
- `src/pages/WorkflowListPage.tsx` — lists all workflows; links to editor; delete action
- `src/pages/WorkflowEditorPage.tsx` — React Flow canvas; drag-and-drop from palette; edge drawing
- `src/components/palette/NodePalette.tsx` + `PaletteNodeCard.tsx` — grouped, searchable
- `src/components/sidebar/ConfigSidebar.tsx` + `SchemaForm.tsx` — `@rjsf/core` form from `input_schema`; `x-sensitive` fields rendered as password inputs
- `src/components/sidebar/TemplateVariablePicker.tsx` — rendered below any field with `"x-template": true`; walks upstream edges via `useWorkflowStore` to find predecessor node IDs; for each predecessor, looks up its `output_schema` from `useNodeTypeStore` **plus any output_parsers defined on that node**; renders a clickable tree of `{{.nodeID.field}}` snippets; clicking inserts the snippet at the cursor in the focused input
- `src/components/sidebar/OutputParserPanel.tsx` — rendered below the main schema form on every node; "Add Extractor" button opens a form with fields: Name (output key), Source (dropdown from node's output_schema), Type (json_path / regex), Pattern, Capture Group (regex only); saved as `output_parsers` in the workflow JSON
- `src/components/shared/Navbar.tsx` — workflow name (editable), Save button, trigger settings icon
- `TriggerPanel` — modal for selecting manual / webhook / cron trigger; shows computed webhook URL; validates cron expression
- Save wires to `POST /workflows` (new) or `PUT /workflows/:id` (existing)

### Testable Criteria

```
1. Open http://localhost:3000
   → WorkflowListPage loads; any previously created workflows appear

2. Click "New Workflow"
   → Empty React Flow canvas; NodePalette shows "http.request", "llm.openai", etc.

3. Drag "HTTP Request" onto the canvas; click it
   → ConfigSidebar opens with a generated form (URL, method, headers fields)

4. Fill in URL field; drag a second "HTTP Request"; draw an edge between them

5. Click "Save"
   → POST /workflows succeeds; URL updates to /workflows/:id
   → Workflow appears in WorkflowListPage on next visit

6. Reload /workflows/:id
   → Nodes, edges, and config values are restored from the API
   → Sensitive fields show "***" (not the raw value)

7. Open TriggerPanel → select "webhook"
   → Webhook URL displayed; save persists the trigger type

8. Select the second HTTP Request node (n2) which has n1 connected upstream
   → URL field (x-template: true) shows a "Variables" section below it
   → n1's outputs are listed: n1.status_code, n1.body, n1.headers
   → Click "n1.body" → {{.n1.body}} is inserted into the URL field
   → Click Save; trigger a run
   → Run succeeds; n2's URL used the resolved value from n1's output
```

### Dependencies
M1, M2 (API must be running)

---

## M11: Frontend — Run & Observe

**Goal:** Users can trigger workflow runs from the UI, watch per-node status update in real time, and drill into past runs to see what each node produced.

### Deliverables

- `src/stores/useRunStore.ts` — active `run_id`, per-node status map, run history
- `src/hooks/useRunEvents.ts` — WebSocket hook; updates `useRunStore` from `NodeEvent` frames
- `src/components/run/RunStatusPanel.tsx` — bottom drawer; shows while run is active or just completed
- `src/components/run/NodeStatusList.tsx` — per-node status badge; expandable output/error JSON
- `src/components/canvas/CustomNode.tsx` updated — overlays status badge (pending/running/succeeded/failed colour) on each canvas node during a run
- Navbar **Run** button → `POST /workflows/:id/runs` with optional initial data input; opens `RunStatusPanel`; subscribes via `useRunEvents`
- `src/pages/RunHistoryPage.tsx` — `GET /workflows/:id/runs`; sortable table; link to RunDetailPage
- `src/pages/RunDetailPage.tsx` — `GET /runs/:run_id`; read-only React Flow graph snapshot with status colours; `NodeDetailList` shows input/output/error per node

### Testable Criteria

```
1. Open a saved workflow with 2 HTTP Request nodes
   → Click "Run"
   → RunStatusPanel slides up; nodes on canvas show "pending" badges

2. As execution proceeds:
   → n1 badge changes: pending → running → succeeded (green)
   → n2 badge changes: pending → running → succeeded (green)
   → RunStatusPanel shows "Run succeeded"

3. Expand n1 in NodeStatusList
   → Output JSON visible (status_code, body)

4. Navigate to /workflows/:id/runs
   → Table shows the completed run with timestamp and status

5. Click the run → RunDetailPage
   → Graph snapshot with green nodes
   → Each node row shows its output (or error if it failed)

6. Force a failure (point a node at an invalid URL)
   → Failed node shows red badge; downstream nodes stay grey (never ran)
   → RunDetailPage shows error message for the failed node
```

### Dependencies
M1, M2, M3, M4, M10

---

## M12: Production Build & Hardening

**Goal:** A developer can clone the repository, copy `.env.example` to `.env`, run `docker-compose up --build`, and have a fully functional cogniflow instance with no additional setup steps. The system handles edge cases, logs structured errors, and is documented for first-time users.

### Deliverables

- `frontend/nginx.conf` finalised — SPA routing (`try_files`), `/api/` and `/runs/` proxy to backend
- `frontend/Dockerfile` — multi-stage Node build + nginx serve
- `docker-compose.yml` finalised — all three services, health checks, `depends_on` ordering, volume for MySQL data
- `.env.example` — documents every required and optional variable (`COGNIFLOW_ENCRYPTION_KEY`, `MYSQL_PASSWORD`, `PLUGIN_ADDRESSES`, `LOG_LEVEL`, port overrides)
- `backend/cmd/server/main.go` — startup validation: refuses to start if `COGNIFLOW_ENCRYPTION_KEY` is missing or too short
- Structured logging throughout backend (e.g., `log/slog`) — request IDs, node execution times, error causes
- `README.md` updated — quick-start section: clone → copy .env → `docker-compose up --build` → open browser
- `backend/Makefile` — `make test`, `make lint` (`golangci-lint`), `make proto` (regenerate gRPC code)

### Testable Criteria

```bash
# Fresh clone on a machine with only Docker installed
git clone git@github.com:g8rswimmer/cogniflow.git
cd cogniflow
cp .env.example .env
# Edit .env: set COGNIFLOW_ENCRYPTION_KEY to a 32-byte base64 value

docker-compose up --build
# All three services start; backend logs "migrations applied", "server listening :8080"

open http://localhost:3000
# → WorkflowListPage loads

# Create a workflow with an HTTP Request node → LLM Call node
# Trigger it; see both nodes succeed; view output in RunDetailPage

# Confirm missing encryption key is caught
COGNIFLOW_ENCRYPTION_KEY="" docker-compose up backend
# → backend exits immediately with a clear error message, not a panic

# Confirm MySQL data survives a restart
docker-compose restart mysql
docker-compose restart backend
curl http://localhost:8080/workflows  # previously created workflows still present
```

### Dependencies
All prior milestones complete.

---

## Milestone Dependency Graph

```
M1 (Scaffold)
 └── M2 (Workflow CRUD + Node Registry)
      └── M3 (Execution Engine + Manual Trigger)
           ├── M4 (Real-Time Events)         ──► M11 (Frontend Run & Observe)
           ├── M5 (Trigger System)
           ├── M6 (AI Nodes)
           │    └── M7 (RAG Nodes)
           └── M8 (Advanced Deterministic Nodes)
      └── M9 (gRPC Plugin Protocol)
 └── M10 (Frontend Canvas & CRUD)            ──► M11
All ──────────────────────────────────────────► M12 (Production Build)
```

Milestones M5, M6, M8, and M9 can be developed in parallel once M3 is complete. M10 can begin in parallel with M5–M9 as long as M2 is done and a mock API or real backend is available.
