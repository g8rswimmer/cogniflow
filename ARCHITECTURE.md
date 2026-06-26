# cogniflow — Architecture Document

> **Status:** Draft v0.3
> **Last Updated:** 2026-06-01

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Repository & Package Structure](#2-repository--package-structure)
3. [Core Go Interfaces](#3-core-go-interfaces)
4. [Execution Engine Design](#4-execution-engine-design)
5. [Node Extension — gRPC Plugin Protocol](#5-node-extension--grpc-plugin-protocol)
6. [Trigger System](#6-trigger-system)
7. [Frontend — React Component Structure](#7-frontend--react-component-structure)
8. [MySQL Schema](#8-mysql-schema)
9. [REST API Contract](#9-rest-api-contract)
10. [CEL Expression Evaluation](#10-cel-expression-evaluation)
11. [Template Variable Syntax](#11-template-variable-syntax)
12. [Output Parser System](#12-output-parser-system)
13. [Security](#13-security)
14. [Docker Compose Services](#14-docker-compose-services)
15. [Implementation Sequencing](#15-implementation-sequencing)

---

## 1. System Overview

### Runtime Component Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          Browser (React SPA)                             │
│                                                                          │
│  ┌─────────────────┐  ┌──────────────────┐  ┌───────────────────────┐   │
│  │  WorkflowCanvas │  │  ConfigSidebar   │  │  RunPanel /           │   │
│  │  (React Flow)   │  │  (JSON Schema    │  │  HistoryView          │   │
│  │                 │  │   driven forms)  │  │  (WebSocket consumer) │   │
│  └────────┬────────┘  └────────┬─────────┘  └──────────┬────────────┘   │
│           │                   │                        │                │
│           └───────────────────┴────────────────────────┘                │
│                     REST (fetch)  /  WS (ws://)                         │
└──────────────────────────────────────────────────────────────────────────┘
                               │
              ┌────────────────┼────────────────────┐
              │ HTTP :8080     │                     │ WS :8080
              ▼                ▼                     ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                       Go Backend  (single binary)                        │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                          api (chi router)                         │   │
│  │  /workflows  /node-types  /runs  /webhooks/:id  /health  /ws     │   │
│  └────────┬──────────────────────────────────┬────────────────────┬─┘   │
│           │                                  │                    │     │
│           ▼                                  ▼                    ▼     │
│  ┌─────────────────┐    ┌──────────────────────────┐   ┌──────────────┐ │
│  │  store          │    │  engine                  │   │  trigger     │ │
│  │  (MySQL via     │    │  (DAG runner, goroutine   │   │  (cron,      │ │
│  │   sqlx)         │    │   fan-out, event emitter) │   │   webhook,   │ │
│  └────────┬────────┘    └──────────┬───────────────┘   │   manual)    │ │
│           │                        │                   └──────┬───────┘ │
│           │             ┌──────────▼───────────────┐          │         │
│           │             │  node                    │          │         │
│           │             │  registry + handlers     │◄─────────┘         │
│           │             │  (built-in + gRPC proxy) │                    │
│           │             └──────────────────────────┘                    │
│           │                          │                                  │
│           │             ┌────────────▼──────────────┐                  │
│           │             │  aiprovider               │                  │
│           │             │  (OpenAI, Anthropic shim) │                  │
│           │             └───────────────────────────┘                  │
│           │                                                             │
│  ┌────────▼─────────────────────────────────────────────────────────┐  │
│  │                      crypto / config                              │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────┘
          │                                      │
          ▼                                      ▼
┌──────────────────┐               ┌─────────────────────────────┐
│  MySQL 9.0+      │               │  gRPC Plugin Processes       │
│  :3306           │               │  (external, any language)    │
│  workflows       │               │  :50051, :50052, …           │
│  runs            │               └─────────────────────────────┘
│  rag_chunks      │
└──────────────────┘
```

### Docker Compose Services

| Service | Image / Build | Role |
|---------|---------------|------|
| `mysql` | `mysql:9.0` | Primary datastore — workflows, runs, RAG vectors |
| `backend` | `./backend` (Go binary) | REST API, WebSocket, execution engine, trigger manager |
| `frontend` | `./frontend` (nginx serving built React SPA) | Serves static assets; proxies `/api` and `/ws` to `backend` |

---

## 2. Repository & Package Structure

### Monorepo Layout

```
cogniflow/                              # Repository root
├── backend/                            # Go service
│   ├── cmd/
│   │   └── server/
│   │       └── main.go                 # Binary entry point: wires all packages, starts HTTP server
│   ├── internal/
│   │   ├── api/
│   │   │   ├── router.go               # chi.Router setup, middleware (CORS, logging, recovery)
│   │   │   ├── workflow_handler.go     # HTTP handlers for /workflows CRUD
│   │   │   ├── run_handler.go          # HTTP handlers for /runs + POST trigger
│   │   │   ├── nodetype_handler.go     # HTTP handler for GET /node-types
│   │   │   ├── webhook_handler.go      # HTTP handler for POST /webhooks/{workflow_id}
│   │   │   ├── health_handler.go       # GET /health
│   │   │   ├── ws_handler.go           # WebSocket upgrade + event fan-out for /runs/{run_id}/events
│   │   │   ├── plugin_admin_handler.go # HTTP handlers for GET/POST/PUT/DELETE /admin/plugins
│   │   │   └── middleware.go           # Request ID, structured logging, content-type enforcement
│   │   ├── engine/
│   │   │   ├── engine.go               # WorkflowEngine implementation; Run() entry point
│   │   │   ├── dag.go                  # DAG adjacency-list builder, topological sort, cycle detection
│   │   │   ├── runner.go               # Goroutine orchestrator: ready-queue, fan-out, Merge node wait
│   │   │   ├── context.go              # ExecutionContext: thread-safe node output map
│   │   │   ├── event.go                # NodeEvent struct and EventBus (channel fan-out to WebSocket)
│   │   │   └── retry.go                # Retry policy evaluation and backoff logic
│   │   ├── node/
│   │   │   ├── handler.go              # NodeHandler interface + NodeMeta struct
│   │   │   ├── registry.go             # NodeRegistry: Register(), Lookup(), ListAll()
│   │   │   ├── builtin/
│   │   │   │   ├── llm/
│   │   │   │   │   └── handler.go      # LLM Call node — calls aiprovider.LLMClient
│   │   │   │   ├── embedding/
│   │   │   │   │   └── handler.go      # Embedding node — calls aiprovider.EmbeddingClient
│   │   │   │   ├── rag_retrieve/
│   │   │   │   │   └── handler.go      # RAG Retrieve node — MySQL VEC_DISTANCE_COSINE query
│   │   │   │   ├── rag_ingest/
│   │   │   │   │   └── handler.go      # RAG Ingest node — chunk, embed, upsert vectors
│   │   │   │   ├── http_request/
│   │   │   │   │   └── handler.go      # HTTP Request node — net/http client with template vars
│   │   │   │   ├── conditional/
│   │   │   │   │   └── handler.go      # Conditional node — cel-go compile + evaluate
│   │   │   │   ├── data_transform/
│   │   │   │   │   └── handler.go      # Data Transform node — JSON template / gval expression
│   │   │   │   ├── db_query/
│   │   │   │   │   └── handler.go      # DB Query node — read-only SQL via database/sql
│   │   │   │   ├── db_write/
│   │   │   │   │   └── handler.go      # DB Write node — insert/update/delete via database/sql
│   │   │   │   ├── merge/
│   │   │   │   │   └── handler.go      # Merge node — identity; engine handles the fan-in wait
│   │   │   │   └── loop_controller/
│   │   │   │       └── handler.go      # Loop Controller node — bounded iteration with CEL exit condition
│   │   │   └── plugin/
│   │   │       ├── grpc_proxy.go       # NodeHandler adapter that forwards calls to a gRPC plugin
│   │   │       └── registrar.go        # Dials plugin addresses at startup, registers proxy handlers
│   │   ├── trigger/
│   │   │   ├── manager.go              # TriggerManager: loads triggers from DB, starts cron + webhook
│   │   │   ├── cron.go                 # robfig/cron v3 wrapper; fires RunRequests on schedule
│   │   │   ├── webhook.go              # Registers per-workflow webhook routes at startup
│   │   │   └── types.go                # RunRequest struct; trigger-type constants
│   │   ├── store/
│   │   │   ├── store.go                # Store interface
│   │   │   ├── mysql/
│   │   │   │   ├── db.go               # *sqlx.DB init, ping, migration bootstrap
│   │   │   │   ├── workflow_store.go   # Workflow CRUD SQL
│   │   │   │   ├── run_store.go        # Run create/update/query SQL
│   │   │   │   ├── rag_store.go        # rag_documents + rag_chunks upsert + vector search
│   │   │   │   └── plugin_store.go     # plugin_registrations CRUD SQL
│   │   │   └── migrations/              # 0001–0029 (run automatically at startup via golang-migrate)
│   │   │       │                        # 0001: workflows
│   │   │       │                        # 0002: workflow_nodes, workflow_edges, node_configs
│   │   │       │                        # 0003: runs
│   │   │       │                        # 0004: output_parsers on workflow_nodes
│   │   │       │                        # 0005–0006: rag_documents, rag_chunks (VECTOR(768))
│   │   │       │                        # 0007–0008: composite PKs; orphan cleanup
│   │   │       │                        # 0009: plugin_registrations
│   │   │       │                        # 0010: widen branch_label to VARCHAR(100)
│   │   │       │                        # 0011: initial_data_schema on workflows
│   │   │       │                        # 0012–0015: eval_suites, test_cases, eval_runs, results
│   │   │       │                        # 0016: loop state on runs
│   │   │       │                        # 0017: grader_registrations
│   │   │       │                        # 0018–0021: workflow_versions; versioning on runs + eval_runs
│   │   │       │                        # 0022: organizations
│   │   │       │                        # 0023: users (role, permissions JSON)
│   │   │       │                        # 0024: invitations (token, expires_at)
│   │   │       │                        # 0025–0027: org_id on workflows, eval_suites, rag tables
│   │   │       │                        # 0028: webhook_secret on workflows (HMAC validation)
│   │   │       └── (...)                # 0029: email_templates (org-level SMTP overrides)
│   │   ├── aiprovider/
│   │   │   ├── provider.go             # LLMClient + EmbeddingClient interfaces
│   │   │   ├── openai/
│   │   │   │   └── client.go           # OpenAI implementation (chat completions + embeddings)
│   │   │   └── anthropic/
│   │   │       └── client.go           # Anthropic implementation (Messages API)
│   │   ├── crypto/
│   │   │   ├── encrypt.go              # AES-256-GCM encrypt/decrypt; envelope key loading
│   │   │   └── config_vault.go         # Wraps Store reads/writes to transparently encrypt sensitive fields
│   │   ├── auth/
│   │   │   ├── token.go                # JWT Claims struct; Sign() and Verify() using HS256
│   │   │   ├── middleware.go           # Authenticate, RequireRole, RequirePermission HTTP middleware
│   │   │   └── password.go             # bcrypt hash/check helpers
│   │   ├── email/
│   │   │   └── sender.go               # SMTP invite email sender (STARTTLS); org-level template overrides
│   │   └── eval/
│   │       ├── grader.go               # Grader interface; BuildGrader() factory for all grader types
│   │       ├── runner.go               # EvalRunner — async per-test-case workflow execution + grading
│   │       ├── scheduler.go            # EvalScheduler — cron-triggered eval suite runs
│   │       ├── handler.go              # HTTP handlers for all /v1/eval-* and /v1/eval-suites/* routes
│   │       ├── importer.go             # Bulk test case import (CSV / JSON)
│   │       ├── vault.go                # GraderVault — AES-256-GCM encryption for grader API keys
│   │       ├── event.go                # EvalEventBus — streams EvalEvent frames to WebSocket subscribers
│   │       ├── ws_handler.go           # WebSocket handler for /v1/eval-runs/{id}/events
│   │       ├── graders/
│   │       │   ├── string_match.go     # Exact / contains / prefix / suffix grader
│   │       │   ├── numeric.go          # Numeric threshold grader (gt/gte/lt/lte/eq)
│   │       │   ├── json_schema.go      # JSON Schema validation grader
│   │       │   ├── llm_judge.go        # LLM-as-judge grader (score 0–1 + reasoning)
│   │       │   └── checklist.go        # Checklist grader (partial scoring, per-item verdicts)
│   │       └── grader_plugin/
│   │           ├── grpc_proxy.go       # GraderHandler adapter forwarding to gRPC grader plugin
│   │           ├── registrar.go        # Dials grader plugin addresses; registers proxy handlers
│   │           └── registry.go         # GraderRegistry — thread-safe map of grader plugin handlers
│   ├── proto/
│   │   └── plugin/
│   │       └── v1/
│   │           ├── plugin.proto         # gRPC service definition for out-of-process node plugins
│   │           ├── plugin.pb.go         # Generated
│   │           └── plugin_grpc.pb.go    # Generated
│   ├── go.mod
│   ├── go.sum
│   ├── Makefile
│   └── Dockerfile
│
├── frontend/                           # React SPA
│   ├── src/
│   │   ├── components/
│   │   │   ├── canvas/
│   │   │   │   ├── WorkflowCanvas.tsx  # React Flow instance; node/edge render
│   │   │   │   ├── CustomNode.tsx      # Node card with status badge overlay
│   │   │   │   ├── CustomEdge.tsx      # Edge with true/false branch label
│   │   │   │   └── CanvasToolbar.tsx   # Zoom, fit, lock controls
│   │   │   ├── palette/
│   │   │   │   ├── NodePalette.tsx     # Left sidebar; grouped + searchable node list
│   │   │   │   └── PaletteNodeCard.tsx # Draggable node type card
│   │   │   ├── sidebar/
│   │   │   │   ├── ConfigSidebar.tsx             # Right sidebar; shows InitialDataSchemaEditor when no node selected; dispatches to ConditionalRuleBuilder for conditional.branch, SchemaForm for all others
│   │   │   │   ├── InitialDataSchemaEditor.tsx  # Workflow Inputs panel — field row editor for initial_data_schema
│   │   │   │   ├── SchemaForm.tsx                # @rjsf/core form driven by node input_schema
│   │   │   │   ├── TemplateVariablePicker.tsx    # Variable browser for x-template:true fields; shows _initial fields from schema
│   │   │   │   └── ConditionalRuleBuilder.tsx    # Visual rule builder for conditional.branch nodes; no raw CEL required
│   │   │   ├── run/
│   │   │   │   ├── RunStatusPanel.tsx  # Bottom drawer; live per-node status
│   │   │   │   ├── RunSummary.tsx      # run_id, status, elapsed time
│   │   │   │   └── NodeStatusList.tsx  # Per-node badge + expandable output/error
│   │   │   └── shared/
│   │   │       ├── Layout.tsx          # App shell with Navbar + <Outlet>
│   │   │       └── Navbar.tsx          # Workflow name, Save, Run, Settings
│   │   ├── pages/
│   │   │   ├── WorkflowListPage.tsx    # /workflows — grid of workflow cards
│   │   │   ├── WorkflowEditorPage.tsx  # /workflows/:id — canvas + palette + sidebar
│   │   │   ├── RunHistoryPage.tsx      # /workflows/:id/runs — sortable run table
│   │   │   └── RunDetailPage.tsx       # /runs/:run_id — graph snapshot + node details
│   │   ├── hooks/
│   │   │   ├── useRunEvents.ts         # WebSocket subscription for a run_id
│   │   │   └── useApi.ts               # Typed fetch wrappers for all REST endpoints
│   │   ├── stores/
│   │   │   ├── useWorkflowStore.ts     # Canvas nodes, edges, configs, dirty flag
│   │   │   ├── useNodeTypeStore.ts     # Cached GET /node-types registry
│   │   │   └── useRunStore.ts          # Active run_id, per-node status map, history
│   │   ├── types/
│   │   │   ├── workflow.ts             # Workflow, WorkflowNode, WorkflowEdge types
│   │   │   ├── node.ts                 # NodeMeta, NodeEvent, NodeStatus types
│   │   │   └── run.ts                  # Run, RunStatus, RunFilter types
│   │   ├── api/
│   │   │   └── client.ts               # Base fetch client; sets Content-Type, base URL
│   │   ├── App.tsx                     # React Router route definitions
│   │   └── main.tsx                    # Vite entry point; mounts <App />
│   ├── public/
│   │   └── favicon.ico
│   ├── nginx.conf                      # Serves SPA; proxies /api and /runs to backend
│   ├── package.json
│   ├── package-lock.json
│   ├── tsconfig.json
│   ├── vite.config.ts
│   ├── tailwind.config.ts
│   └── Dockerfile
│
├── docker-compose.yml                  # Orchestrates mysql + backend + frontend
├── .env.example                        # Template for required environment variables
├── .gitignore
├── REQUIREMENTS.md
├── ARCHITECTURE.md
└── README.md
```

### Backend Package Responsibilities

| Package | Responsibility |
|---------|---------------|
| `backend/cmd/server` | Binary entry point; dependency injection; HTTP server startup |
| `backend/internal/api` | HTTP routing, request parsing, response serialization, WebSocket upgrade; JWT-protected route registration |
| `backend/internal/engine` | DAG construction, topological scheduling, concurrent execution, event emission |
| `backend/internal/node` | NodeHandler interface, NodeRegistry, all built-in node implementations, gRPC proxy adapter |
| `backend/internal/trigger` | Cron scheduler, HMAC-validated webhook route registration, RunRequest dispatch |
| `backend/internal/store` | Store interface + MySQL implementation; schema migrations |
| `backend/internal/aiprovider` | LLM and embedding provider abstractions + concrete OpenAI/Anthropic clients |
| `backend/internal/crypto` | AES-256-GCM encrypt/decrypt helpers; config vault wrapper |
| `backend/internal/auth` | JWT sign/verify (HS256); bcrypt password helpers; HTTP middleware (Authenticate, RequireRole, RequirePermission) |
| `backend/internal/email` | SMTP transactional email sender for user invite flows; org-level template overrides |
| `backend/internal/eval` | Eval suite execution, grader dispatch, grader plugin registry, eval scheduling, HTTP handlers, WebSocket event streaming |
| `backend/proto/plugin/v1` | Protobuf definitions for the out-of-process node plugin gRPC contract |

### Frontend Module Responsibilities

| Module | Responsibility |
|--------|---------------|
| `src/components/canvas` | React Flow canvas, custom node/edge renderers, toolbar |
| `src/components/palette` | Draggable node type list, search, category grouping |
| `src/components/sidebar` | Right sidebar with two states: (1) **node selected** — config panel with `SchemaForm` (or `ConditionalRuleBuilder` for `conditional.branch`), `TemplateVariablePicker`, and `OutputParserPanel`; (2) **no node selected** — "Workflow Settings" panel with `InitialDataSchemaEditor` for defining the workflow's initial data schema (WF-09). `TemplateVariablePicker` and the textarea inline picker show declared `_initial` field chips when a schema is present. |
| `src/components/run` | Live run status panel and per-node detail display |
| `src/components/shared` | App shell, navigation, dismissible toast notifications (`ToastContainer` / `Toast`) |
| `src/pages` | Top-level route components |
| `src/hooks` | WebSocket subscription, typed REST fetch wrappers |
| `src/stores` | Zustand stores for workflow state (incl. `nodeErrors`/`fieldErrors` for save validation), node type cache, run state, toast queue |
| `src/types` | Shared TypeScript type definitions mirroring backend JSON shapes |
| `src/api` | Base HTTP client; `ApiError` class carries `validationErrors: FieldValidationError[]` parsed from `details.validation_errors` |

---

## 3. Core Go Interfaces

### `NodeHandler` — `backend/internal/node/handler.go`

```go
// NodeInput carries the merged output context from all immediate upstream nodes
// plus the node's own persisted configuration values.
type NodeInput struct {
    // UpstreamData is the merged key→value map of all upstream node outputs.
    // Keys are node IDs; values are arbitrary JSON-compatible maps.
    UpstreamData map[string]any

    // Config holds this node's saved configuration values (already decrypted).
    Config map[string]any
}

// NodeOutput is the data this node produces, forwarded to downstream nodes.
type NodeOutput struct {
    Data map[string]any
}

// NodeHandler is the interface every node type — built-in or plugin — must implement.
type NodeHandler interface {
    // Meta returns static metadata for this node type.
    // Called once at registration time; result is cached in the registry.
    Meta() NodeMeta

    // Execute runs the node's logic.
    // ctx carries a deadline derived from the workflow-level timeout.
    // Returning a non-nil error marks the node as failed and halts downstream execution.
    Execute(ctx context.Context, input NodeInput) (NodeOutput, error)
}
```

### `NodeMeta` — `backend/internal/node/handler.go`

```go
// NodeMeta is the static descriptor for a node type.
// The frontend consumes this via GET /node-types to render the palette and
// generate configuration forms from InputSchema and OutputSchema.
type NodeMeta struct {
    // TypeID is the globally unique identifier, e.g. "llm.openai", "http.request".
    TypeID      string `json:"type_id"`

    // DisplayName is the human-readable label shown in the palette.
    DisplayName string `json:"display_name"`

    // Category groups nodes in the palette UI: "ai" | "deterministic" | "plugin"
    Category    string `json:"category"`

    // Description is a short one-line description shown in the palette tooltip.
    Description string `json:"description"`

    // InputSchema is the JSON Schema (draft-07) describing the config form fields.
    // Properties marked with x-sensitive:true are encrypted at rest.
    InputSchema  json.RawMessage `json:"input_schema"`

    // OutputSchema is the JSON Schema describing the shape of NodeOutput.Data.
    OutputSchema json.RawMessage `json:"output_schema"`
}
```

### `NodeRegistry` — `backend/internal/node/registry.go`

```go
// NodeRegistry is the central catalog of all available node types.
// It is populated at startup by built-in registrations and plugin registrar.
type NodeRegistry interface {
    // Register adds a handler under its TypeID. Panics on duplicate TypeID.
    Register(handler NodeHandler)

    // TryRegister adds a handler under its TypeID, returning an error instead
    // of panicking on collision. Used by the admin API and plugin registrar.
    TryRegister(handler NodeHandler) error

    // Lookup returns the handler for a given TypeID, or an error if not found.
    Lookup(typeID string) (NodeHandler, error)

    // ListAll returns metadata for every registered node type, sorted by TypeID.
    ListAll() []NodeMeta

    // Unregister removes a handler from the registry by TypeID. If the handler
    // implements io.Closer, Close() is called before removal. Returns an error
    // if the TypeID is not registered.
    Unregister(typeID string) error
}
```

### `WorkflowEngine` — `backend/internal/engine/engine.go`

```go
// RunRequest is the unified trigger payload regardless of trigger source.
type RunRequest struct {
    WorkflowID  string
    InitialData map[string]any  // provided by caller (webhook body, manual input, etc.)
    TriggeredBy string          // "manual" | "webhook" | "cron"
}

// RunHandle allows the caller to observe or cancel an active run.
type RunHandle struct {
    RunID  string
    Events <-chan NodeEvent  // closed when the run terminates
    Cancel context.CancelFunc
}

// WorkflowEngine orchestrates workflow execution.
type WorkflowEngine interface {
    // Run starts an asynchronous workflow execution and returns immediately.
    // The caller subscribes to RunHandle.Events for real-time status.
    Run(ctx context.Context, req RunRequest) (RunHandle, error)

    // Status returns the current status of a run (used for HTTP polling fallback).
    Status(ctx context.Context, runID string) (RunStatus, error)
}
```

### `TriggerManager` — `backend/internal/trigger/manager.go`

```go
// TriggerManager owns the lifecycle of all workflow triggers.
type TriggerManager interface {
    // LoadAll reads trigger configs from the store and arms all active triggers.
    // Called once at startup after the HTTP server is accepting requests.
    LoadAll(ctx context.Context) error

    // Upsert creates or replaces the trigger for a workflow.
    // Called by the API when a workflow is saved with a new trigger config.
    Upsert(ctx context.Context, workflowID string, cfg TriggerConfig) error

    // Remove disarms and deletes the trigger for a workflow.
    Remove(ctx context.Context, workflowID string) error
}

// TriggerConfig is the persisted trigger configuration for a workflow.
type TriggerConfig struct {
    Kind     string  // "manual" | "webhook" | "cron"
    CronExpr string  // set when Kind == "cron"
}
```

### `Store` — `backend/internal/store/store.go`

```go
// Store is the persistence interface. The MySQL implementation lives in
// internal/store/mysql/. Tests can provide an in-memory stub.
type Store interface {
    // --- Workflow ---
    CreateWorkflow(ctx context.Context, w Workflow) (Workflow, error)
    GetWorkflow(ctx context.Context, id string) (Workflow, error)
    ListWorkflows(ctx context.Context) ([]WorkflowSummary, error)
    UpdateWorkflow(ctx context.Context, w Workflow) (Workflow, error)
    DeleteWorkflow(ctx context.Context, id string) error

    // --- Runs ---
    CreateRun(ctx context.Context, r Run) (Run, error)
    UpdateRunStatus(ctx context.Context, runID string, status RunStatus, output map[string]any) error
    GetRun(ctx context.Context, runID string) (Run, error)
    ListRuns(ctx context.Context, f RunFilter) ([]Run, error)

    // --- RAG ---
    UpsertChunks(ctx context.Context, chunks []RAGChunk) error
    SearchChunks(ctx context.Context, embedding []float32, topK int, docFilter string) ([]RAGChunkResult, error)

    // --- Triggers ---
    SaveTriggerConfig(ctx context.Context, workflowID string, cfg TriggerConfig) error
    GetTriggerConfig(ctx context.Context, workflowID string) (TriggerConfig, error)
    ListTriggerConfigs(ctx context.Context) ([]WorkflowTrigger, error)

    // --- Plugin Registrations ---
    SavePluginRegistration(ctx context.Context, reg PluginRegistration) error
    GetPluginRegistration(ctx context.Context, typeID string) (PluginRegistration, error)
    ListPluginRegistrations(ctx context.Context) ([]PluginRegistration, error)
    DeletePluginRegistration(ctx context.Context, typeID string) error
}

// RunFilter controls ListRuns queries.
type RunFilter struct {
    WorkflowID string
    Status     RunStatus   // empty string means all
    Since      time.Time
    Until      time.Time
    Limit      int
}
```

---

## 4. Execution Engine Design

### DAG Representation in Memory

The engine builds an in-memory graph from the persisted `workflow_nodes` and `workflow_edges` rows each time a run starts.

```go
// internal/engine/dag.go

// DAG holds the adjacency lists for fast traversal.
type DAG struct {
    // Nodes maps node ID → WorkflowNode (type, config, retry policy, etc.)
    Nodes map[string]WorkflowNode

    // Successors maps node ID → slice of immediate downstream node IDs
    Successors map[string][]string

    // Predecessors maps node ID → slice of immediate upstream node IDs
    Predecessors map[string][]string

    // TopologicalOrder is a deterministic execution order derived at build time.
    TopologicalOrder []string

    // OutEdges maps node ID → outgoing edges, preserving branch_label for
    // conditional routing.
    OutEdges map[string][]WorkflowEdge

    // Ancestors maps node ID → all transitively reachable ancestor node IDs.
    // Used by executeNode to populate NodeInput.UpstreamData with the full
    // ancestor chain rather than only immediate predecessors, so a node can
    // reference any upstream output regardless of hop distance.
    Ancestors map[string][]string
}

// Build constructs the DAG from raw node and edge lists.
// Returns ErrCycleDetected if the graph is not acyclic.
func Build(nodes []WorkflowNode, edges []WorkflowEdge) (*DAG, error)

// CycleDetect runs a DFS-based cycle check. Called at workflow save time by the API
// so that invalid graphs are rejected before reaching the engine.
func CycleDetect(nodes []WorkflowNode, edges []WorkflowEdge) error
```

Cycle detection uses depth-first search with a three-colour (white/grey/black) mark; a grey-to-grey back edge signals a cycle. This runs in O(V + E).

### Concurrency Model

```
RunRequest
    │
    ▼
engine.Run()
  │  Creates run record in DB (status=running)
  │  Spawns supervisor goroutine (go runner.Execute(dag, execCtx))
  │  Returns RunHandle immediately
  │
  ▼
runner.Execute(dag, execCtx)
  │
  │  readyQueue chan string  ← initially: all nodes with in-degree == 0
  │  pendingCount sync.Map   ← node ID → number of unfinished predecessors
  │  resultCh    chan nodeResult
  │
  │  For each node popped from readyQueue:
  │      go executeNode(node, execCtx)  ← runs in its own goroutine
  │
  │  executeNode:
  │      1. Emit NodeEvent{status: running}
  │      2. Merge upstream outputs from ExecutionContext
  │      3. Call registry.Lookup(node.TypeID).Execute(ctx, input)
  │      4. On success: store output in ExecutionContext
  │                     send nodeResult{ok} to resultCh
  │                     Emit NodeEvent{status: succeeded, output}
  │      5. On failure: send nodeResult{err} to resultCh
  │                     Emit NodeEvent{status: failed, error}
  │
  │  Supervisor loop (select on resultCh):
  │      On success:
  │          for each successor of completed node:
  │              decrement pendingCount[successor]
  │              if pendingCount[successor] == 0: push to readyQueue
  │      On failure:
  │          cancel the run-scoped context (ctx.Cancel)
  │          drain remaining results (ignore successes, collect errors)
  │          mark run as failed in DB
  │
  │  When readyQueue is empty AND all goroutines have returned:
  │      collect final output (outputs of sink nodes — nodes with no successors)
  │      persist final output to runs table
  │      mark run as succeeded
  │      close RunHandle.Events channel
```

**Sync primitives used:**
- `readyQueue`: `chan string` (buffered, size = number of nodes)
- `pendingCount`: `sync.Map[string, int32]` with `atomic.AddInt32` for decrement
- `resultCh`: `chan nodeResult` (buffered, size = number of nodes)
- `ExecutionContext`: guarded by a `sync.RWMutex` (concurrent reads during fan-out, exclusive write after node completes)

**Merge node special case:** The Merge node's `Execute()` is a no-op; the engine's fan-in decrement is the actual synchronisation. When all predecessors of a Merge node complete, the engine atomically resolves the merged upstream context before scheduling Merge's `executeNode` goroutine.

### Data Flow Between Nodes

```go
// internal/engine/context.go

// ExecutionContext is the shared, run-scoped output store.
// Each key is a node ID; each value is that node's NodeOutput.Data.
type ExecutionContext struct {
    mu      sync.RWMutex
    outputs map[string]map[string]any
}

func (ec *ExecutionContext) Set(nodeID string, data map[string]any)
func (ec *ExecutionContext) MergeUpstream(ancestorIDs []string) map[string]any
```

`MergeUpstream` takes a read lock and iterates over the supplied ancestor IDs — the **transitive closure** of the current node's predecessors, not just immediate predecessors. Each ancestor's output map is included in the returned `UpstreamData` keyed by the ancestor's node ID. Only ancestors that have already produced output (i.e., are in `ExecutionContext.outputs`) are included; skipped or not-yet-executed ancestors are silently omitted.

This means a node any number of hops downstream can reference any upstream ancestor:
- `{{.n1.status_code}}` — works even when `n1 → n2 (conditional) → n3`; `n3` sees `n1`, `n2`, and `_initial`
- `ctx["n1"]["status_code"]` in a CEL conditional — same ancestry applies

**Ancestor computation:** the `DAG` struct stores `Ancestors map[string][]string` — the transitive predecessor set per node — computed in `Build()` via DFS from each node up through `Predecessors`. Parallel branches are excluded: nodes that executed concurrently on a sibling branch are not in each other's ancestor set, preserving deterministic data isolation.

Data is **never mutated after being written** to `ExecutionContext`. Each `executeNode` goroutine receives an immutable snapshot via `MergeUpstream`.

### Per-Node Event Streaming to WebSocket

```go
// internal/engine/event.go

type NodeEventType string

const (
    EventNodePending   NodeEventType = "node.pending"
    EventNodeRunning   NodeEventType = "node.running"
    EventNodeSucceeded NodeEventType = "node.succeeded"
    EventNodeFailed    NodeEventType = "node.failed"
    EventRunSucceeded  NodeEventType = "run.succeeded"
    EventRunFailed     NodeEventType = "run.failed"
)

type NodeEvent struct {
    RunID     string         `json:"run_id"`
    NodeID    string         `json:"node_id"`
    Type      NodeEventType  `json:"type"`
    Timestamp time.Time      `json:"timestamp"`
    Output    map[string]any `json:"output,omitempty"`  // only on succeeded
    Error     string         `json:"error,omitempty"`   // only on failed
}

// EventBus fans out events to all active WebSocket subscribers for a run.
type EventBus struct {
    mu          sync.RWMutex
    subscribers map[string][]chan NodeEvent  // run_id → subscriber channels
}

func (b *EventBus) Subscribe(runID string) (<-chan NodeEvent, func())
func (b *EventBus) Publish(event NodeEvent)
```

`ws_handler.go` calls `EventBus.Subscribe(runID)` during WebSocket upgrade. The returned channel is read in a goroutine that JSON-encodes each `NodeEvent` and writes it to the WebSocket connection. The cleanup function is called in a `defer` when the WebSocket closes.

### Error Handling

When `executeNode` returns a non-nil error:

1. The node's status is set to `failed` in `resultCh`.
2. The supervisor calls `cancel()` on the run-scoped `context.Context` — all in-flight goroutines that respect context cancellation abort promptly.
3. The supervisor waits for all still-running goroutines to drain `resultCh` (with a short timeout).
4. The run record in MySQL is updated to `status=failed` with a structured error JSON containing the failing node ID and error message.
5. A `run.failed` event is published to the `EventBus`.

**Retry policy (EE-07):** Before emitting a failure result, `runner.go` checks the node's `RetryPolicy` (max retries, initial backoff, multiplier). If attempts remain, the node is re-executed with exponential backoff within the same goroutine. Only exhausted retries propagate to the supervisor as a failure.

---

## 5. Node Extension — gRPC Plugin Protocol

### Proto Definition — `backend/proto/plugin/v1/plugin.proto`

```protobuf
syntax = "proto3";
package plugin.v1;
option go_package = "github.com/g8rswimmer/cogniflow/proto/plugin/v1;pluginv1";

// NodePlugin is the service contract every out-of-process node extension must implement.
service NodePlugin {
  // Meta returns the static descriptor for this node type.
  // Called once at startup during plugin registration.
  rpc Meta(MetaRequest) returns (MetaResponse);

  // Execute runs the node logic for a single invocation.
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}

message MetaRequest {}

message MetaResponse {
  string type_id       = 1;
  string display_name  = 2;
  string category      = 3;
  string description   = 4;
  // JSON Schema (UTF-8 encoded) for the config form fields.
  bytes  input_schema  = 5;
  // JSON Schema (UTF-8 encoded) for the output data shape.
  bytes  output_schema = 6;
}

message ExecuteRequest {
  // upstream_data is a JSON object (map of node-id → output-map).
  bytes upstream_data = 1;
  // config is a JSON object of decrypted config values for this node instance.
  bytes config        = 2;
  // timeout_ms is the remaining execution budget in milliseconds.
  int64 timeout_ms    = 3;
}

message ExecuteResponse {
  oneof result {
    bytes       data  = 1;  // JSON object — the node's output map on success
    PluginError error = 2;
  }
}

message PluginError {
  string message = 1;
  string code    = 2;  // machine-readable error code for UI display
}
```

### Plugin Registration at Startup

Plugin processes are discovered via the `PLUGIN_ADDRESSES` environment variable — a comma-separated list of `host:port` values (e.g., `localhost:50051,localhost:50052`).

In `backend/internal/node/plugin/registrar.go`:

```
startup sequence:
  1. Parse PLUGIN_ADDRESSES
  2. For each address:
       a. grpc.Dial(address, grpc.WithBlock(), timeout=5s)
       b. Call Meta() RPC to retrieve NodeMeta
       c. Construct a grpcProxy{conn, client, meta} — implements NodeHandler
       d. registry.Register(grpcProxy)
  3. Any address that fails to connect or returns an invalid Meta is logged
     and skipped (not fatal — built-in nodes remain available)
```

### Lifecycle: gRPC Node vs In-Process Node

The engine calls `registry.Lookup(typeID)` and receives a `NodeHandler`. The calling code is identical regardless of whether the handler is a Go struct or a `grpcProxy`.

Inside `grpcProxy.Execute()`:

```go
func (p *grpcProxy) Execute(ctx context.Context, input NodeInput) (NodeOutput, error) {
    upstreamJSON, _ := json.Marshal(input.UpstreamData)
    configJSON, _   := json.Marshal(input.Config)

    deadline, _ := ctx.Deadline()
    timeoutMs   := time.Until(deadline).Milliseconds()

    resp, err := p.client.Execute(ctx, &pluginv1.ExecuteRequest{
        UpstreamData: upstreamJSON,
        Config:       configJSON,
        TimeoutMs:    timeoutMs,
    })
    if err != nil {
        return NodeOutput{}, fmt.Errorf("grpc plugin %s: %w", p.meta.TypeID, err)
    }
    if e := resp.GetError(); e != nil {
        return NodeOutput{}, fmt.Errorf("[%s] %s", e.Code, e.Message)
    }
    var data map[string]any
    json.Unmarshal(resp.GetData(), &data)
    return NodeOutput{Data: data}, nil
}
```

gRPC connections are kept alive for the process lifetime. If a plugin process crashes, subsequent `Execute` calls return a gRPC transport error, which surfaces as a node failure. Plugin reconnection is deferred to v2.

### Admin API — Dynamic Registration & Persistence

Plugins can also be registered and managed at runtime through an admin HTTP API. Registrations made via this API are stored in the `plugin_registrations` table and automatically restored on the next server startup.

#### `PluginRegistration` struct — `backend/internal/store/store.go`

```go
type PluginRegistration struct {
    TypeID       string          // matches NodeMeta.TypeID; primary key in DB
    Address      string          // gRPC host:port
    DisplayName  string
    Category     string          // always "plugin"
    Description  string
    InputSchema  json.RawMessage
    OutputSchema json.RawMessage
    RegisteredAt time.Time
}
```

#### Startup Flow (updated)

```
1. Load all rows from plugin_registrations (DB-persisted plugins)
   For each:
     a. grpc.NewClient(address)
     b. Call Meta() with 5s timeout to verify still reachable + schema unchanged
     c. registry.TryRegister(grpcProxy{...})
     If unreachable: log warning, skip (remains in DB for next startup attempt)

2. Process PLUGIN_ADDRESSES env var (ephemeral — not stored in DB)
   For each address: same dial → Meta() → TryRegister flow
   If TypeID already registered by step 1: log warning, skip

3. HTTP port opens
```

#### Admin API Registration Flow (`POST /v1/admin/plugins`)

```
Request body: {"address": "host:port"}

1. grpc.NewClient(address, insecure)
2. Call Meta() with 5s timeout
   → error: return 502 PLUGIN_UNAVAILABLE
3. registry.TryRegister(grpcProxy{...})
   → duplicate TypeID error: return 409 PLUGIN_ALREADY_REGISTERED
4. store.SavePluginRegistration(ctx, PluginRegistration{...})
5. Return 201 with the full PluginRegistration JSON
```

#### Admin API Deregistration Flow (`DELETE /v1/admin/plugins/{type_id}`)

```
1. registry.Unregister(typeID)  → closes gRPC conn, removes from map
   → not found: return 404
2. store.DeletePluginRegistration(ctx, typeID)
3. Return 204 No Content
```

#### Admin API Update Flow (`PUT /v1/admin/plugins/{type_id}`)

```
Request body: {"address": "new-host:port"}

1. grpc.NewClient(newAddress)
2. Call Meta() — TypeID in response must match {type_id} in path
   → mismatch or unreachable: return 422 / 502
3. registry.Unregister(typeID)   → closes old conn
4. registry.TryRegister(newProxy)
5. store.SavePluginRegistration(ctx, updated registration)
6. Return 200 with updated PluginRegistration JSON
```

#### Invariants

- `PLUGIN_ADDRESSES` plugins are **ephemeral**: not stored in the DB, not listed or manageable by the admin API. They disappear after the server restarts unless re-added via the env var or the admin API.
- DB-persisted plugins and env-var plugins share the same `NodeRegistry`; TypeID uniqueness is enforced by `TryRegister`.
- `Unregister` does not delete DB rows — it only removes the in-memory registry entry and closes the gRPC connection. The admin `DELETE` endpoint is the only path that removes the DB row.

---

## 6. Trigger System

### Common `RunRequest` Path

All three trigger types eventually call:

```go
// internal/trigger/types.go

type RunRequest struct {
    WorkflowID  string
    InitialData map[string]any
    TriggeredBy string  // "manual" | "webhook" | "cron"
}

// Dispatcher is the shared sink for all trigger types.
type Dispatcher interface {
    Dispatch(ctx context.Context, req RunRequest) (string, error)  // returns run_id
}
```

`engine.Dispatch()` is a thin wrapper around `engine.Run()` that persists the `RunRequest` and returns the run ID synchronously, allowing the HTTP webhook handler to return `202 Accepted` immediately.

### Webhook Trigger

**Endpoint registration (startup):**

At server startup, `TriggerManager.LoadAll()` reads all workflows whose `trigger_kind = 'webhook'` from the DB and registers a chi route for each:

```
POST /webhooks/{workflow_id}
```

New webhooks created while the server is running are added via `router.Mount()` on the live sub-router, which is safe because chi uses a read/write mutex internally.

**Request handling:**

```
POST /v1/webhooks/{workflow_id}
  → parse JSON body (max 1 MB)
  → look up workflow (verify it exists and has webhook trigger)
  → if workflow.webhook_secret is set: validate X-Cogniflow-Signature HMAC-SHA256 header
  → build RunRequest{WorkflowID, InitialData: body, TriggeredBy: "webhook"}
  → engine.Dispatch(req)  ← non-blocking
  → 202 Accepted {"run_id": "<uuid>"}
```

Webhook URLs are stable and deterministic: `/v1/webhooks/{workflow_id}` where `workflow_id` is the UUID assigned at workflow creation (TR-05). The `webhook_secret` field (set via workflow update) enables HMAC-SHA256 request validation.

### Cron Trigger

**Library:** `github.com/robfig/cron/v3` with standard 5-field POSIX cron expressions.

```go
// internal/trigger/cron.go

type CronTrigger struct {
    scheduler  *cron.Cron
    dispatcher Dispatcher
    entryIDs   map[string]cron.EntryID  // workflow_id → cron entry ID
    mu         sync.Mutex
}

func (ct *CronTrigger) Add(workflowID, expr string) error
func (ct *CronTrigger) Remove(workflowID string)
```

`LoadAll()` calls `CronTrigger.Add()` for every workflow with `trigger_kind = 'cron'`. When the cron fires:

```go
dispatcher.Dispatch(ctx, RunRequest{
    WorkflowID:  workflowID,
    InitialData: map[string]any{},
    TriggeredBy: "cron",
})
```

`Upsert` / `Remove` on the `TriggerManager` call the corresponding `CronTrigger` method to update the live scheduler without restart.

### Manual Trigger

The frontend calls `POST /workflows/{id}/runs` with an optional JSON body `{"initial_data": {...}}`. The `run_handler.go` constructs a `RunRequest{TriggeredBy: "manual"}` and passes it to `engine.Dispatch()`. Response is `201 Created {"run_id": "..."}`.

---

## 7. Frontend — React Component Structure

### Tech Choices

| Concern | Choice |
|---------|--------|
| Framework | React 18 + TypeScript |
| Canvas | React Flow (`@xyflow/react`) |
| State management | Zustand |
| HTTP client | `fetch` (native), wrapped in typed helpers |
| WebSocket | Native `WebSocket` API with a custom React hook |
| Form generation | `@rjsf/core` (react-jsonschema-form) |
| Styling | Tailwind CSS |
| Routing | React Router v6 |
| Build | Vite |

### Route / Page Structure

```
/login                                       → LoginPage
/accept-invite                               → AcceptInvitePage
/                                            → redirect to /workflows
/workflows                                   → WorkflowListPage
/workflows/new                               → WorkflowEditorPage (new blank workflow)
/workflows/:id                               → WorkflowEditorPage (load existing)
/workflows/:id/runs                          → RunHistoryPage
/runs/:run_id                                → RunDetailPage
/workflows/:id/versions                      → WorkflowVersionHistoryPage
/workflows/:id/versions/:version_number      → WorkflowVersionDetailPage
/workflows/:id/eval-suites                   → EvalSuiteListPage
/eval-suites/:id                             → EvalSuiteDetailPage
/eval-runs/:id                               → EvalRunDetailPage
/admin/orgs                                  → AdminOrgsPage (system_admin only)
/admin/grader-plugins                        → GraderPluginAdminPage (system_admin only)
/org/users                                   → OrgUsersPage (org_admin or system_admin)
```

### Component Tree

```
App
├── Layout
│   ├── Navbar (workflow name, Save button, Run button, Settings icon)
│   └── <Outlet>
│
├── WorkflowListPage
│   └── WorkflowCard[]  (name, last run status, trigger type, actions)
│
├── WorkflowEditorPage
│   ├── NodePalette          (left sidebar)
│   │   ├── PaletteSearch
│   │   └── PaletteNodeCard[] (draggable; grouped by category)
│   │
│   ├── WorkflowCanvas       (centre — React Flow instance)
│   │   ├── CustomNode[]     (renders each node with status badge during runs)
│   │   ├── CustomEdge[]     (conditional edges show true/false labels)
│   │   └── CanvasToolbar    (zoom, fit, lock)
│   │
│   ├── ConfigSidebar        (right sidebar — shown when a node is selected)
│   │   ├── NodeTypeHeader   (icon, display_name, description)
│   │   └── SchemaForm       (@rjsf/core renders the node's input_schema)
│   │       ├── SensitiveField          (password input for x-sensitive:true fields)
│   │       └── TemplateVariablePicker  (variable browser for x-template:true fields)
│   │
│   ├── TriggerPanel         (modal/sheet — workflow-level trigger config)
│   │   ├── TriggerTypeSelect (manual / webhook / cron)
│   │   ├── CronInput        (shown when cron selected; validates expr)
│   │   └── WebhookURLDisplay (read-only computed URL for webhook type)
│   │
│   └── RunStatusPanel       (bottom drawer — visible during/after a run)
│       ├── RunSummary       (run_id, status, elapsed time)
│       └── NodeStatusList   (per-node status badge + expandable output/error)
│
├── RunHistoryPage
│   └── RunTable             (sortable by time/status; links to RunDetailPage)
│
└── RunDetailPage
    ├── RunSummary
    ├── WorkflowGraphPreview  (read-only React Flow snapshot with status colours)
    └── NodeDetailList        (each node: status, input, output, error, duration)
```

### Dynamic Form Generation (JSON Schema → UI)

`ConfigSidebar` passes the selected node's `input_schema` (fetched once via `GET /node-types` and cached in Zustand) to `@rjsf/core`:

```tsx
<Form
  schema={selectedNode.meta.input_schema}
  formData={selectedNode.config}
  onChange={({ formData }) => updateNodeConfig(selectedNode.id, formData)}
  uiSchema={buildUiSchema(selectedNode.meta.input_schema)}
/>
```

`buildUiSchema()` walks the schema and:
- Sets `ui:widget: "password"` for any property with `x-sensitive: true`
- Marks any property with `x-template: true` so `SchemaForm` renders a `TemplateVariablePicker` below that field

Template fields still accept plain text — the picker is optional and can be ignored. No other custom widget code is needed for v1.

### State Management — Zustand

Four stores:

1. **`useWorkflowStore`** — canvas nodes, edges, per-node configs, dirty flag, workflow metadata (id, name, trigger config). Also holds `nodeErrors: Record<string, string[]>` and `fieldErrors: Record<string, Record<string, string>>` — populated by `setValidationErrors()` when a save returns `VALIDATION_FAILED`, cleared by `clearValidationErrors()` on the next successful save.
2. **`useNodeTypeStore`** — cached `GET /node-types` response (loaded once at app startup); `NodeMeta[]` for palette rendering and schema lookup.
3. **`useRunStore`** — active `run_id`, per-node status map, panel visibility, `connectionLost` flag.
4. **`useToastStore`** — dismissible notification queue; `addToast(type, title, message?)` appends an entry; `ToastContainer` renders and auto-removes entries after their configured duration.

React Flow's `onNodesChange` / `onEdgesChange` callbacks write directly into `useWorkflowStore`, keeping a single source of truth.

### WebSocket Integration

```tsx
// src/hooks/useRunEvents.ts

export function useRunEvents(runId: string | null) {
  const setNodeStatus = useRunStore(s => s.setNodeStatus)

  useEffect(() => {
    if (!runId) return
    const ws = new WebSocket(`ws://${location.host}/runs/${runId}/events`)
    ws.onmessage = (evt) => {
      const event: NodeEvent = JSON.parse(evt.data)
      setNodeStatus(event.node_id, event.type, event.output, event.error)
      if (event.type === 'run.succeeded' || event.type === 'run.failed') {
        ws.close()
      }
    }
    return () => ws.close()
  }, [runId])
}
```

`WorkflowEditorPage` and `RunDetailPage` both call `useRunEvents`, keeping node status badges in sync.

---

## 8. MySQL Schema

### Table Definitions

```sql
-- ============================================================
-- workflows
-- ============================================================
CREATE TABLE workflows (
    id                    CHAR(36)     NOT NULL,
    name                  VARCHAR(255) NOT NULL,
    description           TEXT,
    trigger_kind          ENUM('manual','webhook','cron') NOT NULL DEFAULT 'manual',
    trigger_cron_expr     VARCHAR(100),
    -- Advisory JSON Schema for the run's initial data (migration 0011). NULL = no schema defined.
    initial_data_schema   JSON         NULL,
    timeout_seconds       INT          NOT NULL DEFAULT 300,
    created_at            DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at            DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                       ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_workflows_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- workflow_nodes
-- One row per node instance in a workflow graph.
-- position_x / position_y are for React Flow canvas rendering.
-- ============================================================
CREATE TABLE workflow_nodes (
    id               CHAR(36)         NOT NULL,
    workflow_id      CHAR(36)         NOT NULL,
    type_id          VARCHAR(100)     NOT NULL,
    label            VARCHAR(255),
    position_x       DOUBLE           NOT NULL DEFAULT 0,
    position_y       DOUBLE           NOT NULL DEFAULT 0,
    retry_max        TINYINT UNSIGNED NOT NULL DEFAULT 0,
    retry_backoff_ms INT UNSIGNED     NOT NULL DEFAULT 1000,
    PRIMARY KEY (id),
    CONSTRAINT fk_wn_workflow FOREIGN KEY (workflow_id)
        REFERENCES workflows (id) ON DELETE CASCADE,
    INDEX idx_wn_workflow_id (workflow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- workflow_edges
-- One row per directed edge between two nodes.
-- branch_label is used by Conditional nodes ('true'/'false').
-- ============================================================
CREATE TABLE workflow_edges (
    id           CHAR(36)     NOT NULL,
    workflow_id  CHAR(36)     NOT NULL,
    source_id    CHAR(36)     NOT NULL,
    target_id    CHAR(36)     NOT NULL,
    branch_label VARCHAR(20),
    PRIMARY KEY (id),
    CONSTRAINT fk_we_workflow FOREIGN KEY (workflow_id)
        REFERENCES workflows (id) ON DELETE CASCADE,
    INDEX idx_we_workflow_id (workflow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- node_configs
-- Per-node configuration key-value pairs.
-- Sensitive properties store ciphertext in encrypted_value;
-- plain_value is NULL for those rows.
-- ============================================================
CREATE TABLE node_configs (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    node_id         CHAR(36)        NOT NULL,
    config_key      VARCHAR(255)    NOT NULL,
    plain_value     MEDIUMTEXT,
    -- AES-256-GCM ciphertext (base64): nonce(12B) || ciphertext || tag(16B)
    encrypted_value MEDIUMBLOB,
    is_sensitive    BIT(1)          NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uq_nc_node_key (node_id, config_key),
    CONSTRAINT fk_nc_node FOREIGN KEY (node_id)
        REFERENCES workflow_nodes (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- runs
-- One row per workflow execution.
-- Per-node events stream via WebSocket only (not persisted).
-- Only the final workflow output is stored here.
-- ============================================================
CREATE TABLE runs (
    id           CHAR(36)     NOT NULL,
    workflow_id  CHAR(36)     NOT NULL,
    triggered_by ENUM('manual','webhook','cron') NOT NULL,
    status       ENUM('pending','running','succeeded','failed') NOT NULL DEFAULT 'pending',
    started_at   DATETIME(3),
    finished_at  DATETIME(3),
    final_output JSON,
    error_detail JSON,
    PRIMARY KEY (id),
    CONSTRAINT fk_r_workflow FOREIGN KEY (workflow_id)
        REFERENCES workflows (id) ON DELETE CASCADE,
    INDEX idx_runs_workflow_status_started (workflow_id, status, started_at),
    INDEX idx_runs_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- rag_documents
-- ============================================================
CREATE TABLE rag_documents (
    id          CHAR(36)     NOT NULL,
    workflow_id CHAR(36)     NOT NULL,
    source_uri  VARCHAR(2048),
    title       VARCHAR(512),
    ingested_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT fk_rd_workflow FOREIGN KEY (workflow_id)
        REFERENCES workflows (id) ON DELETE CASCADE,
    INDEX idx_rd_workflow_id (workflow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- rag_chunks
-- MySQL 9.0+ VECTOR(1536) stores the float32 embedding.
-- Dimension 1536 matches OpenAI text-embedding-3-small.
-- ============================================================
CREATE TABLE rag_chunks (
    id          CHAR(36)         NOT NULL,
    document_id CHAR(36)         NOT NULL,
    chunk_index INT UNSIGNED     NOT NULL,
    chunk_text  MEDIUMTEXT       NOT NULL,
    embedding   VECTOR(1536)     NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_rc_document FOREIGN KEY (document_id)
        REFERENCES rag_documents (id) ON DELETE CASCADE,
    INDEX idx_rc_document_id (document_id),
    VECTOR INDEX vidx_rc_embedding (embedding)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- plugin_registrations
-- One row per plugin registered via the admin API.
-- Plugins registered only via PLUGIN_ADDRESSES (ephemeral) are
-- not stored here. No foreign keys — referential integrity is
-- enforced at the application layer (see database conventions).
-- ============================================================
CREATE TABLE plugin_registrations (
    type_id       VARCHAR(100)  NOT NULL,
    address       VARCHAR(500)  NOT NULL,
    display_name  VARCHAR(255)  NOT NULL,
    category      VARCHAR(100)  NOT NULL DEFAULT 'plugin',
    description   TEXT,
    input_schema  JSON          NOT NULL,
    output_schema JSON          NOT NULL,
    registered_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (type_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### Vector Column Usage

RAG Retrieve issues similarity search using MySQL's native vector functions:

```sql
SELECT
    rc.id,
    rc.chunk_text,
    VEC_DISTANCE_COSINE(rc.embedding, :query_embedding) AS score
FROM rag_chunks rc
JOIN rag_documents rd ON rc.document_id = rd.id
WHERE rd.workflow_id = :workflow_id
ORDER BY score ASC
LIMIT :top_k;
```

`:query_embedding` is passed as a `VECTOR` binary literal from the Go driver. MySQL 9.0's `VECTOR INDEX` (HNSW-based ANN) accelerates this query.

### Sensitive Config Storage

The `node_configs` table stores sensitive values in `encrypted_value` (MEDIUMBLOB). The `ConfigVault` in `backend/internal/crypto/config_vault.go` intercepts `GetWorkflow` and decrypts values before returning to the engine. The API layer then replaces sensitive field values with `"***"` before serialisation, so raw secrets never reach the browser.

---

## 9. REST API Contract

### Endpoint Summary

All routes are prefixed `/v1/` unless noted. Routes marked **🔒** require a valid JWT Bearer token. Routes marked **🛡️** require a specific role.

**Public (no auth)**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/v1/config` | Public runtime configuration (e.g. feature flags) |
| `POST` | `/v1/auth/login` | Issue a JWT from email + password |
| `GET` | `/v1/auth/invite/{token}` | Fetch invite details for accept-invite flow |
| `POST` | `/v1/auth/accept-invite` | Accept an invite; set password; activate account |
| `POST` | `/v1/webhooks/{workflow_id}` | Inbound webhook trigger (HMAC-validated inside handler) |
| `POST` | `/v1/eval-webhooks/{suite_id}` | Inbound eval webhook trigger (HMAC-validated inside handler) |

**Authenticated**

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/v1/auth/me` | (any authed user) | Current user info from JWT |
| `GET` | `/v1/node-types` | `workflow:read` | List all registered node types |
| `GET` | `/v1/workflows` | `workflow:read` | List workflows (org-scoped) |
| `POST` | `/v1/workflows` | `workflow:write` | Create a workflow |
| `GET` | `/v1/workflows/{id}` | `workflow:read` | Get a workflow (full definition) |
| `PUT` | `/v1/workflows/{id}` | `workflow:write` | Replace a workflow definition |
| `DELETE` | `/v1/workflows/{id}` | `workflow:write` | Delete a workflow |
| `GET` | `/v1/workflows/{id}/versions` | `workflow:read` | List version history |
| `GET` | `/v1/workflows/{id}/versions/{version_number}` | `workflow:read` | Get a specific version |
| `POST` | `/v1/workflows/{id}/versions/{version_number}/restore` | `workflow:write` | Restore workflow to a previous version |
| `POST` | `/v1/workflows/{id}/runs` | `workflow:run` | Manually trigger a run |
| `GET` | `/v1/workflows/{id}/runs` | `workflow:read` | List runs for a workflow |
| `GET` | `/v1/runs/{run_id}` | `workflow:read` | Get a single run |
| `WS` | `/v1/runs/{run_id}/events` | `workflow:read` | Stream real-time run events |
| `GET` | `/v1/workflows/{workflow_id}/eval-suites` | `eval:read` | List eval suites for a workflow |
| `POST` | `/v1/workflows/{workflow_id}/eval-suites` | `eval:write` | Create an eval suite |
| `GET` | `/v1/eval-suites/{suite_id}` | `eval:read` | Get an eval suite |
| `PUT` | `/v1/eval-suites/{suite_id}` | `eval:write` | Update an eval suite |
| `DELETE` | `/v1/eval-suites/{suite_id}` | `eval:write` | Delete an eval suite |
| `GET` | `/v1/eval-suites/{suite_id}/test-cases` | `eval:read` | List test cases |
| `POST` | `/v1/eval-suites/{suite_id}/test-cases` | `eval:write` | Create a test case |
| `PUT` | `/v1/eval-suites/{suite_id}/test-cases/order` | `eval:write` | Reorder test cases |
| `POST` | `/v1/eval-suites/{suite_id}/test-cases/import` | `eval:write` | Bulk import test cases |
| `GET` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | `eval:read` | Get a test case |
| `PUT` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | `eval:write` | Update a test case |
| `DELETE` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | `eval:write` | Delete a test case |
| `POST` | `/v1/eval-suites/{suite_id}/runs` | `eval:run` | Trigger an eval run |
| `GET` | `/v1/eval-suites/{suite_id}/runs` | `eval:read` | List eval runs |
| `GET` | `/v1/eval-runs/{eval_run_id}` | `eval:read` | Get an eval run |
| `WS` | `/v1/eval-runs/{eval_run_id}/events` | `eval:read` | Stream real-time eval run events |
| `GET` | `/v1/eval-runs/{eval_run_id}/compare` | `eval:read` | Compare two eval runs |
| `GET` | `/v1/eval-runs/{eval_run_id}/test-case-results/{result_id}` | `eval:read` | Get a single test case result |

**Org-admin routes** (`org_admin` or `system_admin` role required)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/org/users` | List users in the caller's org |
| `POST` | `/v1/org/users/invite` | Invite a new user to the org |
| `PUT` | `/v1/org/users/{id}/role` | Change a user's role |
| `PUT` | `/v1/org/users/{id}/permissions` | Override a user's permission scopes |
| `DELETE` | `/v1/org/users/{id}` | Remove a user from the org |
| `GET` | `/v1/org/email-settings` | Get org email settings |
| `PUT` | `/v1/org/email-settings` | Upsert org email settings (SMTP + templates) |
| `DELETE` | `/v1/org/email-settings` | Delete org email settings |

**System-admin routes** (`system_admin` role required)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/admin/orgs` | List all organizations |
| `POST` | `/v1/admin/orgs` | Create an organization |
| `DELETE` | `/v1/admin/orgs/{id}` | Delete an organization |
| `GET` | `/v1/admin/users` | List all users across all orgs |
| `DELETE` | `/v1/admin/users/{id}` | Delete any user |
| `PUT` | `/v1/admin/orgs/{org_id}/email-settings` | Set email settings for any org |
| `GET` | `/v1/admin/plugins` | List persisted node plugin registrations |
| `POST` | `/v1/admin/plugins` | Register a node plugin by gRPC address |
| `PUT` | `/v1/admin/plugins/{type_id}` | Update a node plugin's address |
| `DELETE` | `/v1/admin/plugins/{type_id}` | Unregister a node plugin |
| `GET` | `/v1/admin/grader-plugins` | List persisted grader plugin registrations |
| `POST` | `/v1/admin/grader-plugins` | Register a grader plugin by gRPC address |
| `PUT` | `/v1/admin/grader-plugins/{type_id}` | Update a grader plugin's address |
| `DELETE` | `/v1/admin/grader-plugins/{type_id}` | Unregister a grader plugin |

### Request / Response Examples

**`GET /node-types`**

```json
{
  "node_types": [
    {
      "type_id": "llm.openai",
      "display_name": "LLM Call (OpenAI)",
      "category": "ai",
      "description": "Send a prompt to an OpenAI chat model and receive a completion.",
      "input_schema": {
        "type": "object",
        "required": ["model", "prompt"],
        "properties": {
          "api_key":     { "type": "string", "title": "API Key", "x-sensitive": true },
          "model":       { "type": "string", "title": "Model", "default": "gpt-4o" },
          "system_msg":  { "type": "string", "title": "System Message", "x-template": true },
          "prompt":      { "type": "string", "title": "Prompt", "x-template": true },
          "max_tokens":  { "type": "integer", "title": "Max Tokens", "default": 1024 },
          "temperature": { "type": "number", "title": "Temperature", "default": 0.7 }
        }
      },
      "output_schema": {
        "type": "object",
        "properties": {
          "completion":          { "type": "string" },
          "prompt_tokens":       { "type": "integer" },
          "completion_tokens":   { "type": "integer" }
        }
      }
    }
  ]
}
```

**`POST /workflows`** — Request:

```json
{
  "name": "Customer Support Flow",
  "description": "Classify and respond to support tickets",
  "trigger": { "kind": "webhook" },
  "timeout_seconds": 120,
  "nodes": [
    {
      "id": "node-1",
      "type_id": "llm.openai",
      "label": "Classify Intent",
      "position": { "x": 100, "y": 200 },
      "config": {
        "model": "gpt-4o",
        "prompt": "Classify the following: {{input.text}}",
        "api_key": "sk-..."
      },
      "retry_policy": { "max_retries": 2, "backoff_ms": 500 }
    }
  ],
  "edges": [
    { "id": "edge-1", "source_id": "node-1", "target_id": "node-2", "branch_label": null }
  ]
}
```

Response (`201 Created`):

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Customer Support Flow",
  "trigger": { "kind": "webhook", "webhook_url": "/webhooks/550e8400-e29b-41d4-a716-446655440000" },
  "nodes": [{ "...": "same as input; api_key returned as ***" }],
  "edges": [{ "...": "same as input" }],
  "created_at": "2026-05-29T14:00:00.000Z",
  "updated_at": "2026-05-29T14:00:00.000Z"
}
```

**`POST /workflows/:id/runs`** — Request:

```json
{ "initial_data": { "text": "My order hasn't arrived." } }
```

Response (`201 Created`):

```json
{
  "run_id": "7b9e3c1a-1234-4f6b-a8d2-000000000001",
  "workflow_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "triggered_by": "manual",
  "started_at": "2026-05-29T14:01:00.000Z"
}
```

**`GET /runs/:run_id`** — Response (`200 OK`):

```json
{
  "run_id": "7b9e3c1a-...",
  "workflow_id": "550e8400-...",
  "status": "succeeded",
  "triggered_by": "manual",
  "started_at": "2026-05-29T14:01:00.000Z",
  "finished_at": "2026-05-29T14:01:04.321Z",
  "final_output": {
    "node-2": { "completion": "Your order is delayed. Apologies!", "prompt_tokens": 42 }
  },
  "error_detail": null
}
```

**`POST /webhooks/:workflow_id`** — Response (`202 Accepted`):

```json
{ "run_id": "abc123-..." }
```

### WebSocket Event Schema

`WS /runs/:run_id/events` — server sends one JSON text frame per event.

```json
{ "run_id": "7b9e3c1a-...", "node_id": "node-1", "type": "node.running",   "timestamp": "2026-05-29T14:01:00.512Z", "output": null, "error": null }
{ "run_id": "7b9e3c1a-...", "node_id": "node-1", "type": "node.succeeded", "timestamp": "2026-05-29T14:01:02.100Z", "output": { "completion": "..." }, "error": null }
{ "run_id": "7b9e3c1a-...", "node_id": "node-2", "type": "node.failed",    "timestamp": "2026-05-29T14:01:03.000Z", "output": null, "error": "http status 429: rate limit exceeded" }
{ "run_id": "7b9e3c1a-...", "node_id": "",        "type": "run.succeeded",  "timestamp": "2026-05-29T14:01:04.321Z", "output": null, "error": null }
```

Event types: `node.pending`, `node.running`, `node.succeeded`, `node.failed`, `run.succeeded`, `run.failed`.

### Error Response Format

```json
{
  "error": {
    "code": "CYCLE_DETECTED",
    "message": "Workflow graph contains a cycle between nodes node-3 → node-1",
    "details": {}
  }
}
```

For `VALIDATION_FAILED` responses, `details` is populated with a `validation_errors` array so the frontend can highlight specific nodes and fields without parsing the human-readable message string:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Workflow validation failed: 2 error(s)",
    "details": {
      "validation_errors": [
        { "node_id": "llm.anthropic-1748976543210", "field": "prompt",     "message": "invalid template: unexpected EOF" },
        { "node_id": "conditional.branch-1748976600000", "field": "expression", "message": "CEL compile error: undeclared reference to 'x'" }
      ]
    }
  }
}
```

`FieldValidationError` struct (defined in `backend/internal/api/response.go`):
```go
type FieldValidationError struct {
    NodeID  string `json:"node_id,omitempty"`  // empty for workflow-level errors
    Field   string `json:"field,omitempty"`    // empty when the error covers the whole node
    Message string `json:"message"`
}
```

`writeValidationErrors(w, summary, []FieldValidationError)` is used instead of `writeError` when validation produces structured node/field context. All four save-time validators (`validateRequiredFields`, `validateTemplates`, `validateCELExpressions`, `validateOutputParsers`) accumulate errors across every node rather than failing on the first one, so the full list is returned in a single response.

Standard error codes: `NOT_FOUND`, `VALIDATION_FAILED`, `CYCLE_DETECTED`, `WORKFLOW_SAVE_FAILED`, `ENGINE_ERROR`, `INTERNAL_ERROR`.

---

## 10. Conditional Node — Rule Engine

The conditional node (`conditional.branch`) supports two config formats that coexist without migration:

- **New format** (`config.rules`): structured rules defined visually in the UI; CEL is generated internally by the backend. Edges carry the rule label as `branch_label`.
- **Legacy format** (`config.expression`): a single raw CEL string; edges labeled `"true"` / `"false"`. Existing workflows continue to operate unchanged.

### New Format: Structured Rules

#### Data Structures

```go
// internal/node/builtin/conditional/handler.go

type ConditionalCondition struct {
    NodeID    string `json:"node_id"`    // upstream node whose output to inspect
    Field     string `json:"field"`      // field name in that node's output map
    Operator  string `json:"operator"`   // "==" | "!=" | ">" | ">=" | "<" | "<=" | "contains"
    Value     string `json:"value"`      // right-hand operand (always stored as string)
    ValueType string `json:"value_type"` // "string" | "number" | "boolean" — drives CEL literal formatting
}

type ConditionalRule struct {
    Label      string                 `json:"label"`      // unique per node; "fallback" is reserved
    Logic      string                 `json:"logic"`      // "AND" | "OR" — applies to all conditions in this rule
    Conditions []ConditionalCondition `json:"conditions"` // at least one required
}
```

**Config shape** (`config["rules"]` is `[]ConditionalRule`):

```json
{
  "rules": [
    {
      "label": "success",
      "logic": "AND",
      "conditions": [
        { "node_id": "n1", "field": "status_code", "operator": "==", "value": "200", "value_type": "number" }
      ]
    },
    {
      "label": "error",
      "logic": "AND",
      "conditions": [
        { "node_id": "n1", "field": "status_code", "operator": ">=", "value": "400", "value_type": "number" }
      ]
    }
  ]
}
```

#### CEL Generation — `rulesToCEL(rule ConditionalRule) string`

For each condition, emit a CEL term:

| Operator | Generated CEL |
|----------|--------------|
| `==`, `!=`, `>`, `>=`, `<`, `<=` | `ctx["<node_id>"]["<field>"] <op> <literal>` |
| `contains` | `ctx["<node_id>"]["<field>"].contains("<value>")` |

Literal formatting by `value_type`:
- `"string"` → `"value"` (quoted)
- `"number"` → `value` (unquoted)
- `"boolean"` → `true` / `false`

Conditions are joined with ` && ` (AND logic) or ` || ` (OR logic).

#### Validation — `ValidateRules(rules []ConditionalRule) error`

Called at workflow save time (`POST/PUT /workflows`):
- Rejects an empty rules slice
- Rejects empty or duplicate rule labels; rejects `"fallback"` as a label (reserved)
- Rejects rules with zero conditions or unknown operators
- Generates CEL for each rule and compiles it; rejects if not bool-typed

#### Execution

`Execute()` detects the config format at runtime:

1. If `config["expression"]` is a non-empty string → **legacy path**: evaluate the raw CEL, return `{"result": bool}` (unchanged behaviour).
2. If `config["rules"]` is present → **new path**: unmarshal rules, evaluate each in definition order using `rulesToCEL` + cached CEL program, return `{"matched_rule": "<first matching label>"}` or `{"matched_rule": "fallback"}` when none match.

#### Engine Routing — `branchAllows()` (internal/engine/runner.go)

```go
func branchAllows(edge store.WorkflowEdge, output map[string]any) bool {
    if edge.BranchLabel == nil { return true }
    label := *edge.BranchLabel

    // New format: string match against matched_rule
    if mr, ok := output["matched_rule"].(string); ok {
        return mr == label
    }
    // Legacy format: bool match against "true"/"false" label
    if res, ok := output["result"].(bool); ok {
        return (label == "true") == res
    }
    return false
}
```

Edges whose `branch_label` does not match `matched_rule` are suppressed; the existing `propagateSkip` mechanism prevents downstream deadlock.

### Legacy Format: Raw CEL Expression

For nodes with `config["expression"]` set, behaviour is identical to the pre-M14 implementation:

```go
func ValidateExpression(expr string) error {
    env, _ := cel.NewEnv(cel.Variable("ctx", cel.MapType(cel.StringType, cel.DynType)))
    ast, issues := env.Compile(expr)
    if issues != nil && issues.Err() != nil {
        return fmt.Errorf("CEL compile error: %w", issues.Err())
    }
    if ast.OutputType() != cel.BoolType {
        return fmt.Errorf("CEL expression must evaluate to bool, got %s", ast.OutputType())
    }
    return nil
}
```

Returns `{"result": bool}`; edges labeled `"true"` or `"false"` route accordingly.

### Frontend — ConditionalRuleBuilder

`ConfigSidebar` detects `type_id === "conditional.branch"` and renders `ConditionalRuleBuilder` instead of `SchemaForm`. The builder provides:
- Per-rule cards with an editable label, AND/OR logic toggle, and a list of conditions
- Each condition row: upstream-field dropdown (from ancestor node output_schemas) + operator selector + value input
- Add/remove/reorder rules; Add/remove conditions
- A static **Fallback** chip (fires when no rule matches)
- Legacy-format detection: shows a banner with a "Migrate" button for old `expression`-only nodes

When rules are renamed or deleted, `useWorkflowStore.syncConditionalEdgeLabels()` nullifies stale edge labels automatically.

---

## 11. Template Variable Syntax

### Overview

Node config fields can reference the outputs of upstream nodes using Go `text/template` syntax. This enables workflows like "use the LLM completion as the body of the next HTTP request" without writing code.

The mechanism is enabled field-by-field via a JSON Schema extension, consistent with the existing `"x-sensitive": true` convention.

### JSON Schema Extensions

#### `x-template`

Any `input_schema` property marked `"x-template": true` accepts template expressions. Fields without this marker are stored and passed as literal strings.

Example — HTTP Request node `url` field:

```json
"url": {
  "type": "string",
  "title": "URL",
  "x-template": true
}
```

Fields may carry both `x-sensitive: true` and `x-template: true` simultaneously (e.g., a URL that contains an auth token derived from an upstream node).

#### `x-textarea`

A property marked `"x-textarea": true` renders as a multi-line scrollable `<textarea>` in the Config Sidebar instead of a single-line `<input>`. This flag is independent of `x-template`.

When both flags are set (e.g., LLM `prompt` and `system_msg`), `SchemaForm` renders the `TextareaTemplateWidget`: a textarea with an inline variable picker (upstream node dropdown → field dropdown → Insert button) directly below it. The standard chip-based `TemplateVariablePicker` panel is suppressed for these fields since they carry their own inline picker.

Example — LLM Call node `prompt` and `system_msg` fields:

```json
"prompt":     { "type": "string", "title": "Prompt",         "x-template": true, "x-textarea": true },
"system_msg": { "type": "string", "title": "System Message", "x-template": true, "x-textarea": true }
```

### Template Syntax

Go `text/template` is used. The template data is `NodeInput.UpstreamData` — a `map[string]any` keyed by node ID, where each value is that node's output map.

| Expression | Meaning |
|-----------|---------|
| `{{.n1.status_code}}` | `status_code` field from the output of node with ID `n1` |
| `{{.n1.body}}` | `body` string from node `n1`'s output |
| `{{._initial.customer_id}}` | `customer_id` from the run's initial data |
| `{{index .n1 "some-key"}}` | key with a hyphen or special character |

The template data map shape at runtime:

```go
// template data = NodeInput.UpstreamData
// Contains ALL transitive ancestors, not just immediate predecessors.
// Example for node n3 in the chain n1 → n2 → n3:
map[string]any{
    "_initial": map[string]any{"customer_id": 42},  // run initial data (always present)
    "n1":       map[string]any{"status_code": 200, "body": "..."},  // grandparent ancestor
    "n2":       map[string]any{"completion": "Hello!"},              // direct predecessor
}
```

Templates are evaluated per field using Go's `text/template` with `Option("missingkey=error")` so a reference to a non-existent node or field fails fast rather than silently producing an empty string.

**Scope rule:** A node can reference any ancestor — any node reachable by following edges backward from the current node in the DAG. Parallel sibling nodes (nodes on a concurrent branch that share no edge path with the current node) are not ancestors and cannot be referenced. This makes data flow deterministic: the set of accessible upstream keys is fixed by the workflow graph topology, not by execution timing.

### Optionality

Template syntax is **always optional**. A field marked `"x-template": true` behaves exactly like a plain string field unless the stored value contains `{{`. This means:

- `https://api.example.com/items` → stored and used as-is, no template processing
- `https://api.example.com/{{.n1.path}}` → `{{.n1.path}}` is resolved at execution time

The `TemplateVariablePicker` in the UI is a convenience helper — users can dismiss it and type literal values freely. Existing workflows with no template references are unaffected by this feature.

### Save-Time Validation

When a workflow is saved (`POST /workflows` or `PUT /workflows/:id`), the API validates every `x-template: true` field value that contains `{{`:

```go
// internal/api/workflow_handler.go — validateTemplates()

func validateTemplates(nodes []store.WorkflowNode, registry *node.NodeRegistry) error {
    for _, n := range nodes {
        h, err := registry.Lookup(n.TypeID)
        if err != nil {
            continue // unknown type caught by node-type validation
        }
        templateKeys := parseTemplateKeys(h.Meta().InputSchema)
        for _, key := range templateKeys {
            val, ok := n.Config[key].(string)
            if !ok || !strings.Contains(val, "{{") {
                continue
            }
            if _, err := template.New("").Parse(val); err != nil {
                return fmt.Errorf("node %q field %q: invalid template: %w", n.ID, key, err)
            }
        }
    }
    return nil
}
```

Parse errors return `VALIDATION_FAILED` — identical to the CEL validation pattern for Conditional nodes (§10). Valid templates are stored as-is; expansion happens at execution time inside `Execute()`.

### Which Node Types Support Templates

| Node Type | Template-capable fields |
|-----------|------------------------|
| `http.request` | `url`, `body`, header values |
| `llm.openai` / `llm.anthropic` | `prompt`, `system_msg` |
| `embedding` | `input` |
| `data_transform` | `template` expression |
| `db_query` / `db_write` | parameterised query values |

### Frontend Integration

`ConfigSidebar` checks whether the focused field has `"x-template": true` in the node's `input_schema`. If so, it renders a `TemplateVariablePicker` component below the input:

```
TemplateVariablePicker
  └── walks upstream edges in useWorkflowStore to find predecessor node IDs
  └── for each predecessor node:
        looks up its NodeMeta.output_schema from useNodeTypeStore
        renders a tree of clickable {{.nodeID.field}} snippets
  └── _initial section:
        if workflow.initialDataSchema is defined → one chip per declared field ({{._initial.fieldname}})
        otherwise                                → single {{._initial}} chip (backward compatible)
  └── clicking a snippet: inserts it at the cursor position in the focused input field
```

This gives users a discoverable, click-to-insert experience without memorising upstream node IDs or output field names.

---

## 12. Output Parser System

### Overview

After a node's `Execute()` completes successfully, the engine applies zero or more **output parsers** defined on that node. Each parser extracts a named value from the raw output and merges it into the node's effective output map. Downstream nodes can then reference extracted fields via the standard template syntax — `{{.nodeID.extracted_field}}` — exactly as they would reference any native output field.

This is particularly useful for LLM nodes whose completions contain structured data: rather than requiring a downstream Data Transform node, the author can extract `user_id` or `account_status` directly from the completion text and route on those values immediately.

### Data Model

`OutputParser` is defined in `backend/internal/store/store.go` and stored as a JSON blob in the `output_parsers` column on `workflow_nodes` (added by migration `0004_add_output_parsers`).

```go
// store/store.go

type OutputParser struct {
    Kind         string `json:"kind"`           // "json_path" | "regex"
    Source       string `json:"source"`         // field in raw output to read (e.g. "completion")
    Pattern      string `json:"pattern"`        // gjson dot-path or regex pattern
    CaptureGroup int    `json:"capture_group"`  // regex only: 0 = full match, 1+ = group index
}

// WorkflowNode.OutputParsers is a map from extracted field name → parser rule.
// e.g. {"user_id": OutputParser{Kind:"json_path", Source:"completion", Pattern:"user.id"}}
```

### Extraction Strategies

| Kind | Pattern syntax | Example |
|------|---------------|---------|
| `json_path` | gjson dot-path (`field`, `a.b`, `arr.0.x`) | `result.score` extracts `{"result":{"score":0.98}}` → `0.98` (float64) |
| `regex` | Go `regexp` | `(?:user_id: )(\d+)` with `capture_group: 1` extracts the digits after `"user_id: "` |

For `json_path`, extracted values preserve their native JSON type — booleans remain `bool`, numbers remain `float64`, and strings remain `string`. JSON null is treated as no-match and the field is omitted. This means downstream CEL expressions can use typed comparisons like `ctx["n1"]["compromised"] == true` rather than `== "true"`. For `regex`, extracted values are always strings (inherent to regexp group capture). The source field (e.g. `completion`) is read from the node's raw output; if it is absent or not a string, the extractor is silently skipped.

### Execution-Time Flow

```
executeNode()
  └── executeWithRetry()  →  raw NodeOutput.Data
  └── outputparser.Apply(out.Data, n.OutputParsers)
        ├── for each parser: extract value from source field
        ├── successful extractions merged into a new output map
        └── failed extractions silently omitted (no match, bad pattern)
  └── augmented outData stored in ExecutionContext + published on EventBus
```

The extraction is transparent to the node handler; handlers never see or configure parsers.

### Save-Time Validation

`validateOutputParsers()` in `workflow_handler.go` is called on every `POST /workflows` and `PUT /workflows/:id`. It delegates to `outputparser.ValidateAll()`:

- `json_path`: pattern must be non-empty
- `regex`: pattern must compile; `capture_group` must be ≥ 0

Validation failures return `VALIDATION_FAILED` — same as template and CEL validation.

### Implementation

```
backend/internal/node/outputparser/parser.go   — Apply(), Validate(), ValidateAll()
backend/internal/store/store.go                — OutputParser struct, WorkflowNode.OutputParsers
backend/internal/store/mysql/
  migrations/0004_add_output_parsers.up.sql    — ALTER TABLE workflow_nodes ADD COLUMN output_parsers JSON
backend/internal/engine/runner.go              — Apply() called after executeWithRetry()
backend/internal/api/workflow_handler.go       — validateOutputParsers() at save time
```

Dependencies: `github.com/tidwall/gjson` for JSON path queries.

### Frontend Configuration (M10)

The `ConfigSidebar` renders an **Output Parsers** section below the main schema form for every node. Users can:

1. Click **Add Extractor** to open a mini-form with fields:
   - **Name** — the new output field name (e.g. `user_id`)
   - **Source** — dropdown populated from the node's `output_schema` fields
   - **Type** — `json_path` or `regex`
   - **Pattern** — gjson path or regex string
   - **Capture Group** — (regex only) integer ≥ 0; default 0 = full match
2. Save the workflow; extracted field names appear in the `TemplateVariablePicker` for downstream nodes alongside native output fields.

---

## 13. Security

### AES-256-GCM Encryption of Sensitive Config Values

**Key management:** The data encryption key (DEK) is a 32-byte random value provided as a base64-encoded environment variable: `COGNIFLOW_ENCRYPTION_KEY`. The backend refuses to start if absent or fewer than 32 decoded bytes. In production, injected via Docker Compose `env_file` pointing to a `.env` outside the repository (`.gitignore`d).

**Encryption (write path):**

```
plaintext → AES-256-GCM
  key:    DEK (32 bytes)
  nonce:  12 random bytes per operation (crypto/rand)
  output: base64(nonce || ciphertext || 16-byte GCM auth tag)
          → stored in node_configs.encrypted_value
```

**Decryption (read path):** `base64-decode → split nonce(12) | rest → cipher.Open → plaintext`

The `ConfigVault` wraps the Store: it decrypts `encrypted_value` blobs on `GetWorkflow` before returning to the engine. The API layer replaces sensitive field values with `"***"` before serialisation. Decrypted values are never logged.

### Other Security Measures (v1)

| Area | Measure |
|------|---------|
| SQL injection | All queries use parameterised statements via `sqlx` — no string interpolation |
| CORS | `Access-Control-Allow-Origin` restricted to `COGNIFLOW_ALLOWED_ORIGIN` env var |
| Request size | HTTP bodies capped at 1 MB via `http.MaxBytesReader` |
| CEL sandbox | `cel-go` runs in a sandboxed interpreter; no file system or network access |
| DB credentials | MySQL password via environment variable, not hardcoded |
| gRPC plugins | v1: localhost-only, no TLS; TLS deferred to v2 |
| Dependency scanning | `govulncheck` in CI |

---

## 14. Docker Compose Services

```yaml
version: "3.9"

services:

  mysql:
    image: mysql:9.0
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:-cogniflow_root}
      MYSQL_DATABASE:      cogniflow
      MYSQL_USER:          cogniflow
      MYSQL_PASSWORD:      ${MYSQL_PASSWORD:-cogniflow_pass}
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost",
             "-u", "cogniflow", "--password=${MYSQL_PASSWORD:-cogniflow_pass}"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 30s

  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    restart: unless-stopped
    depends_on:
      mysql:
        condition: service_healthy
    environment:
      DSN: "cogniflow:${MYSQL_PASSWORD:-cogniflow_pass}@tcp(mysql:3306)/cogniflow?parseTime=true&multiStatements=true"
      COGNIFLOW_ENCRYPTION_KEY: ${COGNIFLOW_ENCRYPTION_KEY}
      COGNIFLOW_ALLOWED_ORIGIN: "http://localhost:${FRONTEND_PORT:-3000}"
      PLUGIN_ADDRESSES: ${PLUGIN_ADDRESSES:-}
      PORT: "8080"
      LOG_LEVEL: ${LOG_LEVEL:-info}
    ports:
      - "${BACKEND_PORT:-8080}:8080"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 5

  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile
      args:
        VITE_API_BASE: "http://localhost:${BACKEND_PORT:-8080}"
    restart: unless-stopped
    depends_on:
      backend:
        condition: service_healthy
    ports:
      - "${FRONTEND_PORT:-3000}:80"

volumes:
  mysql_data:
```

### Backend Dockerfile (`backend/Dockerfile`)

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o cogniflow ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=builder /app/cogniflow .
ENTRYPOINT ["./cogniflow"]
```

### Frontend Dockerfile (`frontend/Dockerfile`)

```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
ARG VITE_API_BASE=http://localhost:8080
ENV VITE_API_BASE=$VITE_API_BASE
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

### `frontend/nginx.conf`

```nginx
server {
    listen 80;

    location / {
        root   /usr/share/nginx/html;
        index  index.html;
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://backend:8080/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }

    location /runs/ {
        proxy_pass http://backend:8080/runs/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### Startup Ordering

```
mysql (health: mysqladmin ping)
  └──► backend (depends_on mysql healthy; runs golang-migrate at startup)
         └──► frontend (depends_on backend healthy; serves pre-built static assets)
```

The backend binary applies any pending `golang-migrate` migrations at startup before opening the HTTP port. This ensures the schema is always current before the backend declares itself healthy.

---

## 15. Implementation Sequencing

Recommended build order respecting inter-package dependencies:

| Step | Work |
|------|------|
| 1 | **Schema migrations** (`backend/internal/store/migrations/`) — establishes the DB contract |
| 2 | **`backend/internal/crypto`** — needed by store and config vault before any config is persisted |
| 3 | **`backend/internal/store`** — foundational; `aiprovider` and `node` depend on it for RAG |
| 4 | **`backend/internal/node/handler.go` + `registry.go`** — interfaces all built-in nodes implement |
| 5 | **Built-in node handlers** — one package at a time, starting with `http_request` (simplest) |
| 6 | **`backend/proto/plugin/v1`** + **`backend/internal/node/plugin/`** — gRPC proxy; parallel with step 5 |
| 7 | **`backend/internal/engine`** — DAG, runner, event bus; unit-testable with stub node handlers |
| 8 | **`backend/internal/trigger`** — cron and webhook; depends on engine's `Dispatch` method |
| 9 | **`backend/internal/api`** — HTTP handlers and WebSocket; depends on store, engine, trigger |
| 10 | **`backend/cmd/server/main.go`** — wires everything together |
| 11 | **`frontend/`** — can begin using a mock API; integrate with real backend last |
