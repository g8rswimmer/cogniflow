# cogniflow — Requirements Document

> **Status:** Draft v0.4 — subject to iteration  
> **Last Updated:** 2026-06-01

---

## 1. Overview

**cogniflow** is a workflow orchestration platform that enables users to build, configure, and run workflows composed of both AI-powered nodes and deterministic processing nodes. Workflows are authored through a visual web-based canvas, persisted to a backend store, and executed by a Go-based runtime engine.

The platform is designed for a single-user / internal-tool deployment with no authentication requirement in the initial version.

---

## 2. Goals

| # | Goal |
|---|------|
| G1 | Enable visual construction of workflows from a library of built-in and user-extended nodes |
| G2 | Support AI nodes (LLM, Embeddings, RAG) and deterministic nodes (HTTP, conditional, transform, database) |
| G3 | Allow workflows to be triggered manually, on a schedule, via inbound webhooks, or from message queues |
| G4 | Provide a node extension interface so developers can add new node types without modifying core code |
| G5 | Persist workflow configurations and allow them to be saved and recalled |
| G6 | Give users real-time visibility into workflow execution (status, logs, outputs) |

---

## 3. Scope

### In Scope (v1)

- Visual workflow canvas (React, browser-based)
- Built-in node library (AI + deterministic types listed in §5)
- Node plugin / extension system
- Workflow configuration persistence (save, load, list)
- Manual run trigger from the UI
- Inbound webhook trigger
- Cron/scheduled trigger
- Execution engine with sequential and parallel branch support (strict DAG — no cycles)
- Per-run execution logs and output visibility
- REST API between frontend and backend

### Out of Scope (v1)

- User authentication / authorization
- Multi-tenancy (multiple users or organizations)
- Billing or usage metering
- Workflow versioning / git integration
- Role-based access control
- Mobile interface
- Marketplace for community node extensions
- Workflow loops / cycles (strict DAGs only in v1; loops deferred to v2)
- Message queue triggers — Kafka, SQS, etc. (deferred to v2)

---

## 4. System Architecture (High-Level)

```
┌─────────────────────────────────────────────────────────────┐
│                     Browser (React)                         │
│  ┌──────────────┐  ┌─────────────────┐  ┌───────────────┐  │
│  │ Workflow     │  │ Node Config     │  │ Run History   │  │
│  │ Canvas       │  │ Panel           │  │ & Logs        │  │
│  └──────┬───────┘  └────────┬────────┘  └───────┬───────┘  │
└─────────┼──────────────────┼───────────────────┼───────────┘
          │  REST/WebSocket  │                   │
┌─────────┼──────────────────┼───────────────────┼───────────┐
│         ▼       Go Backend (API + Engine)       ▼           │
│  ┌──────────────┐  ┌──────────────────┐  ┌──────────────┐  │
│  │ Workflow API │  │ Execution Engine │  │ Trigger      │  │
│  │ (CRUD)       │  │ (DAG runner)     │  │ Manager      │  │
│  └──────────────┘  └────────┬─────────┘  └──────┬───────┘  │
│                             │                    │          │
│  ┌──────────────────────────▼────────────────────▼───────┐  │
│  │                  Node Registry                        │  │
│  │  (built-in nodes + dynamically registered extensions) │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────────┐ │
│  │ Persistence│  │ AI Provider  │  │ External Systems    │ │
│  │ (DB/Store) │  │ Clients      │  │ (Kafka, SQS, HTTP)  │ │
│  └────────────┘  └──────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## 5. Functional Requirements

### 5.1 Workflow Management

| ID | Requirement |
|----|-------------|
| WF-01 | Users can create a new workflow with a name and optional description |
| WF-02 | Users can open an existing workflow and edit it on the canvas |
| WF-03 | Users can save a workflow configuration (nodes, edges, node configs) |
| WF-04 | Users can delete a workflow |
| WF-05 | Users can list all saved workflows |
| WF-06 | A workflow is represented as a directed acyclic graph (DAG) of nodes and edges |
| WF-07 | Workflows support parallel branches (one node's output fans out to multiple downstream nodes) |
| WF-08 | Each workflow has a configurable trigger (manual, webhook, cron, or queue) |

### 5.2 Node System

#### 5.2.1 Shared Node Contract

All nodes (built-in and extended) must conform to the following interface:

| ID | Requirement |
|----|-------------|
| ND-01 | Every node has a unique type identifier (e.g., `llm.openai`, `http.request`) |
| ND-02 | Every node declares its input schema (the fields it accepts from upstream data or configuration) |
| ND-03 | Every node declares its output schema (the shape of data it emits to downstream nodes) |
| ND-04 | Every node has a configuration form rendered in the UI sidebar |
| ND-05 | Every node exposes an `Execute(ctx, input) → (output, error)` operation that the engine calls |
| ND-06 | Nodes can emit structured errors with a human-readable message |
| ND-07 | Node config fields marked `"x-template": true` in their `input_schema` optionally accept Go template syntax (`{{.nodeID.field}}`) to reference upstream node outputs at runtime; `{{._initial.key}}` references the run's initial data. Template syntax is **opt-in** — a plain literal value is always accepted; template evaluation is only triggered when the value contains `{{`. Fields additionally marked `"x-textarea": true` render as multi-line textareas in the UI (e.g. LLM `prompt` and `system_msg` fields), and include an inline variable picker (upstream node dropdown → field dropdown → Insert button) so users can insert template snippets without typing node IDs manually. |
| ND-08 | Template expressions in config fields are validated at workflow save time (parse error → `VALIDATION_FAILED`) so broken templates are caught before execution. A field with no `{{` is stored and used as a literal string with no validation overhead. |
| ND-09 | Nodes support **output parsers** — optional post-execution extraction rules applied by the engine after `Execute()` succeeds. Each parser extracts a named field from the raw output using `json_path` (gjson dot-path) or `regex` (with capture group selection). `json_path` extractions preserve the native JSON type of the matched value (bool, number, or string), enabling downstream CEL conditionals to use typed comparisons (e.g. `ctx["n1"]["compromised"] == true`). Regex extractions always produce strings. Extracted fields are merged into the node's output and are immediately available to downstream nodes via template syntax (e.g. `{{.n1.user_id}}`). Parsers are validated at workflow save time; invalid patterns return `VALIDATION_FAILED`. |
| ND-10 | The UI exposes an **Output Parsers** configuration section on every node's config sidebar. Users define a name, source field, extraction type (json_path / regex), pattern, and optional capture group. Extracted field names appear in the `TemplateVariablePicker` for downstream nodes alongside the node's declared `output_schema` fields. |

#### 5.2.2 Built-In AI Nodes

| ID | Node Type | Description |
|----|-----------|-------------|
| AI-01 | **LLM Call** | Send a prompt (with optional system message) to an LLM provider (OpenAI, Anthropic, etc.). Returns the completion text and token usage. Provider and model are configurable. The `prompt` and `system_msg` fields support template variable syntax so upstream node outputs (e.g. a previous HTTP response body or user-supplied initial data) can be injected into the prompt before the model is called. Output parsers (ND-09) can be used to extract structured fields from the completion text for downstream use. |
| AI-02 | **Embedding** | Generate a vector embedding for one or more text inputs using a configurable provider and model. Returns the embedding vector(s). |
| AI-03 | **RAG Retrieve** | Given a query, generate an embedding and retrieve the top-K most relevant chunks using MySQL 9.0+ native vector search (`VEC_DISTANCE_COSINE` / `VEC_DISTANCE_L2`). Returns matching chunks with scores. |
| AI-04 | **RAG Ingest** | Chunk and embed a document or set of documents and upsert the resulting vectors into a MySQL `VECTOR` column. No separate vector store infrastructure is required. |

#### 5.2.3 Built-In Deterministic Nodes

| ID | Node Type | Description |
|----|-----------|-------------|
| DT-01 | **HTTP Request** | Make an HTTP call (GET, POST, PUT, DELETE, PATCH). URL, method, headers, and body are configurable (with support for template variables from upstream data). Returns status code, headers, and response body. |
| DT-02 | **Conditional** | Evaluate one or more named **rules** against the workflow data context using `google/cel-go`. Rules are defined visually in the UI (upstream field + operator + value) rather than written as raw CEL; the backend generates CEL internally. Each rule may contain multiple conditions combined with AND or OR (applied uniformly within a rule). Rules are evaluated in order; the first matching rule routes execution to the correspondingly labelled downstream edge. If no rule matches, the reserved `"fallback"` edge fires. The UI requires no CEL knowledge. Expressions are validated at save time. Backward-compatible: existing workflows storing a raw `expression` field continue to operate as a single-rule true/false branch unchanged. |
| DT-03 | **Data Transform** | Apply a mapping or transformation to the data context using a configurable template/expression (e.g., JSON template, jq-style filter). Returns the transformed data. |
| DT-04 | **Database Query** | Execute a read (SELECT) query against a configured relational database. Returns the result set. |
| DT-05 | **Database Write** | Execute an insert/update/delete statement against a configured relational database. Returns rows affected. |
| DT-06 | **Merge** | Wait for all upstream parallel branches to complete and merge their outputs into a single data context. |

### 5.3 Node Extension System

| ID | Requirement |
|----|-------------|
| EX-01 | The backend exposes a well-defined Go interface (`NodeHandler`) that developers implement to create new node types |
| EX-02 | New node types can be registered with the node registry at startup without modifying core engine code |
| EX-03 | Each registered node type provides: type ID, display name, input/output schema definitions, and an Execute function |
| EX-04 | The frontend node configuration panel is driven by the node's declared schema, rendering appropriate form controls automatically |
| EX-05 | Extended node types appear alongside built-in nodes in the UI canvas node palette |
| EX-06 | Out-of-process node extensions are supported via a gRPC plugin protocol. External processes implement a defined `.proto` service contract and register with the node registry at startup. This allows node extensions written in any language. |
| EX-07 | The gRPC node plugin contract mirrors the in-process `NodeHandler` interface: `Metadata() → NodeMeta`, `Execute(ctx, input) → (output, error)` |
| EX-08 | The backend exposes an admin HTTP API (`POST /v1/admin/plugins`, `GET /v1/admin/plugins`, `DELETE /v1/admin/plugins/{type_id}`, `PUT /v1/admin/plugins/{type_id}`) for registering, listing, deregistering, and updating out-of-process node extensions at runtime without restarting the server |
| EX-09 | Plugin registrations made via the admin API are persisted to the database; on startup the server automatically re-establishes gRPC connections to all persisted plugins before opening the HTTP port |
| EX-10 | The admin `POST /v1/admin/plugins` endpoint validates the supplied address by dialing it and calling `Meta()` before persisting — unreachable processes or invalid metadata are rejected with a descriptive error; the plugin appears in `GET /v1/node-types` immediately upon successful registration |
| EX-11 | A plugin can be deregistered via `DELETE /v1/admin/plugins/{type_id}`; the node registry entry and database record are removed immediately; workflows that subsequently invoke that node type receive a clear `node type not found` error |

### 5.4 Trigger System

| ID | Requirement |
|----|-------------|
| TR-01 | **Manual:** A user can click "Run" in the UI to trigger a workflow with optional initial input data |
| TR-02 | **Webhook:** The system exposes a unique inbound HTTP endpoint per workflow; a POST to that endpoint with a JSON body triggers the workflow with the body as the initial data |
| TR-03 | **Cron:** A workflow can be configured with a cron expression; the engine triggers it on schedule |
| TR-04 | Each workflow supports exactly one active trigger configuration at a time |
| TR-05 | Webhook endpoints are deterministic and stable (tied to the workflow ID) |
| TR-06 | (v2) Message queue triggers (Kafka, SQS) are deferred to v2 |

### 5.5 Execution Engine

| ID | Requirement |
|----|-------------|
| EE-01 | The engine executes a workflow as a strict DAG (directed acyclic graph) — cycles are not permitted. The system validates acyclicity when a workflow is saved and rejects any graph containing a cycle. |
| EE-02 | Nodes with no unresolved upstream dependencies execute concurrently (parallel branch support) |
| EE-03 | Data flows between nodes: each node receives the merged output context of **all transitively connected upstream nodes** — any node reachable via the DAG edge path from the current node's position, not just its immediate predecessors. This means a node three hops downstream can reference the output of the root node directly (e.g. `{{.n1.status_code}}`). The context is also available for template expansion in config fields (see ND-07) and for CEL conditional expressions (see DT-02). |
| EE-04 | Each execution is assigned a unique run ID |
| EE-05 | The engine records per-node execution status: pending, running, succeeded, failed |
| EE-06 | On node failure, the engine stops execution of dependent downstream nodes and marks the run as failed |
| EE-07 | (Optional v1) Configurable retry policy per node (max retries, backoff) |
| EE-08 | Execution timeout is configurable per workflow |

### 5.6 Configuration Persistence

| ID | Requirement |
|----|-------------|
| CF-01 | Workflow definitions (graph topology + per-node configs) are stored in a persistent database |
| CF-02 | Sensitive values in node configs (API keys, DB passwords) are stored encrypted at rest |
| CF-03 | The API supports full CRUD for workflow definitions |
| CF-04 | Execution run history (run ID, status, start/end time, final workflow output) is persisted in MySQL |
| CF-05 | Per-node inputs and outputs are emitted as structured log/event entries during execution and surfaced via the run events WebSocket endpoint for real-time debugging — they are not stored permanently in the database |
| CF-06 | Run history is queryable by workflow ID, status, and time range |

### 5.7 Web Interface

| ID | Requirement |
|----|-------------|
| UI-01 | A visual workflow canvas where nodes can be dragged, dropped, and connected with edges |
| UI-02 | A node palette listing all available node types (built-in + extensions), searchable |
| UI-03 | Clicking a node opens a configuration sidebar with a generated form for that node's config schema |
| UI-11 | For config fields marked `"x-template": true` (without `"x-textarea": true`), the sidebar displays a variable picker chip panel listing the outputs of all upstream nodes; clicking a chip inserts the `{{.nodeID.field}}` snippet into the focused field. For fields with both `"x-template": true` and `"x-textarea": true`, a compact inline picker (node dropdown → field dropdown → Insert button) is rendered directly below the textarea, replacing the separate chip panel for those fields. |
| UI-12 | Save validation errors are surfaced at three levels: (a) **canvas node highlight** — nodes with errors show a red ring and a `!` badge; hovering the badge reveals the error messages as a tooltip; (b) **config sidebar field-level errors** — when a node with errors is selected, each invalid field shows an inline error message below the input (driven by RJSF `extraErrors`); (c) **toast notification** — a dismissible toast appears in the top-right corner with the full error summary and auto-dismisses after 6 s. All error indicators clear automatically on the next successful save. |
| UI-04 | Users can name/rename a workflow |
| UI-05 | A "Save" button persists the current workflow definition |
| UI-06 | A "Run" button manually triggers a workflow (with optional initial data input) |
| UI-07 | A run status panel shows real-time execution progress (per-node status via WebSocket or polling) |
| UI-08 | A run history view lists past executions with status and timestamps |
| UI-09 | Clicking a past run shows per-node input/output and logs |
| UI-10 | Workflow trigger configuration is accessible from the workflow settings panel |

---

## 6. Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NF-01 | **Extensibility:** New node types can be added with zero changes to the core engine or frontend routing |
| NF-02 | **Performance:** The engine should handle workflows with up to 50 nodes without measurable latency beyond node execution time |
| NF-03 | **Reliability:** A failed node run must not corrupt the workflow definition or other runs |
| NF-04 | **Observability:** All node executions emit structured logs. The system should expose a health check endpoint |
| NF-05 | **Deployment:** The system runs via Docker Compose (`docker-compose up`) for both local development and self-hosted production. A `docker-compose.yml` is provided that starts the Go backend, React frontend (served as static files), and MySQL 9.0+. |

---

## 7. Technical Requirements

### 7.1 Frontend (React)

| ID | Requirement |
|----|-------------|
| FE-01 | Built with React (TypeScript preferred) |
| FE-02 | Workflow canvas built on **React Flow** (or equivalent) for node/edge rendering and interaction |
| FE-03 | Node configuration forms are dynamically generated from the node's JSON schema |
| FE-04 | Communicates with the Go backend via REST API (and WebSocket for real-time run status) |
| FE-05 | State management for the canvas (node positions, connections, config values) |

### 7.2 Backend (Go)

| ID | Requirement |
|----|-------------|
| BE-01 | Written in Go (1.22+) |
| BE-02 | REST API (JSON over HTTP) for all frontend interactions |
| BE-03 | WebSocket endpoint for streaming run execution events to the frontend |
| BE-04 | Modular package structure: `api`, `engine`, `node`, `trigger`, `store` |
| BE-05 | Node interface defined as a Go interface in the `node` package; all built-in nodes implement it |
| BE-06 | AI provider clients abstracted behind an interface to allow swapping providers |

### 7.3 Data Persistence

| ID | Requirement |
|----|-------------|
| DB-01 | **MySQL 9.0+** as the primary datastore (workflow definitions, run history, and vector embeddings for RAG) |
| DB-02 | RAG embeddings stored in a `VECTOR` column; similarity search uses MySQL's native `VEC_DISTANCE_COSINE` / `VEC_DISTANCE_L2` functions and ANN indexing — no separate vector store required |
| DB-03 | Schema migrations managed with a migration tool (e.g., golang-migrate) |
| DB-04 | Sensitive node config values encrypted before storage (AES-256 or similar) |
| DB-05 | (Optional) Redis for run-state caching and pub/sub for real-time events |

### 7.4 API Design

| Resource | Endpoints |
|----------|-----------|
| Workflows | `GET /workflows`, `POST /workflows`, `GET /workflows/{id}`, `PUT /workflows/{id}`, `DELETE /workflows/{id}` |
| Node Types | `GET /node-types` (returns registry of all available node types + schemas) |
| Runs | `POST /workflows/{id}/runs`, `GET /workflows/{id}/runs`, `GET /runs/{runId}` |
| Triggers | `POST /webhooks/{workflowId}` (inbound webhook) |
| Admin — Plugins | `GET /admin/plugins`, `POST /admin/plugins`, `PUT /admin/plugins/{typeId}`, `DELETE /admin/plugins/{typeId}` |
| Health | `GET /health` |
| WebSocket | `WS /runs/{runId}/events` (real-time execution events) |

---

## 8. Future Considerations (v2+)

| # | Item |
|---|------|
| F-01 | Workflow loops / cycles with configurable max-iteration guard |
| F-02 | Message queue triggers — Kafka topic and AWS SQS queue subscriptions |
| F-03 | User authentication and multi-tenancy |
| F-04 | Workflow versioning and change history |
| F-05 | Persistent per-node execution data (full run replay) |
| F-06 | Community node extension marketplace |
