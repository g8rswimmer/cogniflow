# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Planning Documents

Always read these before starting implementation work:

- **`REQUIREMENTS.md`** — functional and non-functional requirements; the authoritative source for what the system must do
- **`ARCHITECTURE.md`** — package structure, core Go interfaces, MySQL schema, REST API contract, CEL usage, gRPC plugin protocol, and Docker Compose setup
- **`PROJECT_PLAN.md`** — milestones M1–M15 with deliverables and testable criteria; use this to understand what has been built and what comes next
- **`REQUIREMENTS_EVAL.md`** — requirements for the workflow evaluation / quality-testing feature
- **`ARCHITECTURE_EVAL.md`** — architecture for the eval subsystem (graders, eval runner, grader plugins)
- **`PROJECT_PLAN_EVAL.md`** — milestones ME1–ME5 for the eval feature

The memory file `milestone-status.md` (in the project memory directory) tracks which milestones are complete. **Update it whenever a milestone is finished.**

---

## Definition of Done

A milestone is not complete until all of the following pass from `backend/`:

1. `go build ./...` — no compile errors
2. `go test ./...` — all tests pass, coverage >80% on new packages
3. `golangci-lint run ./...` — zero warnings (default linter set)

Run these in order before marking a milestone done or committing final work.

---

## Commands

All Go commands must be run from `backend/` (the Go module root). All frontend commands run from `frontend/`.

```bash
# Backend
cd backend
go build ./...                         # compile check
go build -o bin/server ./cmd/server    # build binary
go run ./cmd/server                    # run locally (requires DB_DSN env var)
go test ./...                          # all tests
go test ./internal/engine/...          # single package
go test -run TestCycleDetect ./internal/engine/...  # single test
golangci-lint run ./...                # lint (must be zero warnings before milestone complete)

# Frontend (once scaffolded)
cd frontend
npm install
npm run dev     # Vite dev server
npm run build   # production bundle
npm run lint
```

```bash
# Full stack
docker compose up --build              # build + start mysql + backend (+ frontend when added)
docker compose up --build backend      # rebuild backend only

# Verify M1 health
curl http://localhost:8080/health
docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow -e "SHOW TABLES;"
```

---

## Architecture

### Repository layout

This is a monorepo. The Go module root is `backend/` (not the repo root). The frontend will live in `frontend/`. `docker-compose.yml` and `.env` are at the repo root.

```
cogniflow/
├── backend/          ← Go module: github.com/g8rswimmer/cogniflow
│   ├── cmd/server/   ← binary entry point (main.go)
│   └── internal/
│       ├── api/      ← HTTP handlers + chi router (JWT-protected routes)
│       ├── engine/   ← DAG execution engine
│       ├── node/     ← NodeHandler interface, registry, all built-in nodes, gRPC proxy
│       ├── trigger/  ← cron + webhook (HMAC-validated) trigger manager
│       ├── store/    ← Store interface + MySQL implementation + migrations
│       ├── aiprovider/ ← LLMClient / EmbeddingClient interfaces + OpenAI/Anthropic clients
│       ├── crypto/   ← AES-256-GCM helpers + config vault
│       ├── auth/     ← JWT sign/verify, RBAC middleware (Authenticate, RequireRole, RequirePermission)
│       ├── email/    ← SMTP invite email sender with configurable templates
│       └── eval/     ← Eval suite runner, graders, grader plugins, scheduler, event bus
├── frontend/         ← React 18 + TypeScript SPA (Vite, React Flow, Zustand, Tailwind)
└── docker-compose.yml
```

### Core node interface

Every node type — built-in or plugin — implements `NodeHandler` in `backend/internal/node/handler.go`:

```go
type NodeHandler interface {
    Meta() NodeMeta
    Execute(ctx context.Context, input NodeInput) (NodeOutput, error)
}
```

`NodeInput.UpstreamData` is the merged key→value map from all immediate upstream node outputs (keyed by node ID). `NodeInput.Config` holds the saved, pre-decrypted configuration for that node instance. Registering a new built-in node means implementing this interface and calling `registry.Register(handler)` in `main.go`.

`NodeMeta.InputSchema` and `OutputSchema` are `json.RawMessage` (not `map[string]any`). Properties marked `"x-sensitive": true` in `InputSchema` are encrypted at rest by the config vault.

### Execution engine flow

`engine.Run()` is non-blocking — it returns a `RunHandle` immediately and executes asynchronously. Internally:

1. `dag.Build()` constructs an adjacency list; `dag.CycleDetect()` uses DFS three-colour (white/grey/black).
2. A `readyQueue chan string` feeds a goroutine pool; `pendingCount sync.Map` tracks unresolved upstream dependencies per node.
3. Completed nodes push `NodeEvent` structs to an `EventBus` (channel fan-out); WebSocket clients subscribe to the bus for the run's `run_id`.
4. Per-node events stream live but are **not stored in the database** — only the final workflow output and run status are persisted.

### Node registry and plugins

`NodeRegistry` is a thread-safe map populated at startup. Built-in nodes register themselves; out-of-process plugins connect via gRPC (proto at `backend/proto/plugin/v1/plugin.proto`, addresses from `PLUGIN_ADDRESSES` env var). The gRPC proxy implements `NodeHandler` and forwards `Meta()` / `Execute()` calls over the wire.

### Trigger system

`TriggerManager` (in `internal/trigger/`) loads trigger configs from the DB at startup and activates:
- **Webhook**: registers a stable `POST /webhooks/{workflow_id}` route
- **Cron**: `github.com/robfig/cron/v3` scheduler fires `RunRequest` on schedule
- **Manual**: `POST /workflows/{id}/runs` from the API

All three converge on `engine.Run(ctx, RunRequest{...})`.

### Data persistence

MySQL 9.0+ is the sole datastore — workflow definitions, run history, AND RAG vector embeddings (`VECTOR(768)` column after migration 0006, `VEC_DISTANCE_COSINE` / `VEC_DISTANCE_L2` for similarity search). No separate vector store.

Migrations live in `backend/internal/store/mysql/migrations/` and are embedded via `//go:embed` and run automatically on startup via `golang-migrate`. Add new migrations as `NNNN_<description>.up.sql` / `.down.sql`.

### Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DB_DSN` | yes | MySQL DSN: `user:pass@tcp(host:3306)/dbname?parseTime=true` |
| `PORT` | no | HTTP listen port (default `8080`) |
| `COGNIFLOW_ENCRYPTION_KEY` | yes | 32-byte AES-256 key for sensitive config values |
| `JWT_SECRET` | yes | HMAC-SHA256 signing key for JWT tokens (at least 32 chars) |
| `JWT_TTL` | no | JWT token lifetime (default `24h`); accepts Go duration strings |
| `BOOTSTRAP_ADMIN_EMAIL` | no | Email for auto-created system_admin user on first startup |
| `BOOTSTRAP_ADMIN_PASSWORD` | no | Password for the bootstrap admin (only used when no users exist) |
| `FRONTEND_URL` | no | Base URL of the frontend (used in invite email links) |
| `PLUGIN_ADDRESSES` | no | Comma-separated `host:port` list of gRPC plugin processes |
| `OLLAMA_BASE_URL` | no | If set, RAG nodes use Ollama for embeddings (e.g. `http://localhost:11434`); otherwise OpenAI is used |

Copy `.env.example` → `.env` before running locally.

### Go interface conventions

Follow standard Go interface design:

- **Interfaces belong to the consumer, not the producer.** A package that provides a concrete type does not declare an interface for it. The package that *depends on* the type declares the minimal interface it needs. This keeps packages decoupled and avoids forcing every consumer to satisfy a producer-owned contract.
- **Constructors return concrete types.** `NewFoo()` returns `*Foo`, not an interface. Returning a concrete type gives callers full access to the type and lets them choose what interface (if any) to use at their call site.
- **Accept interfaces, return concrete types.** Function parameters may use interfaces when the function genuinely needs to work with multiple implementations (e.g. `store.Store` in the API layer). Return values use concrete types.

Example — correct:
```go
// node/registry.go — producer defines only the concrete type
type NodeRegistry struct { ... }
func NewRegistry() *NodeRegistry { ... }

// crypto/config_vault.go — consumer takes the concrete type directly
func NewConfigVault(inner store.Store, cipher *Cipher, registry *node.NodeRegistry) *ConfigVault
```

Example — incorrect:
```go
// node/registry.go — do NOT do this
type Registry interface { ... }          // interface defined by producer ✗
func NewRegistry() Registry { ... }     // constructor hides concrete type ✗
```

### Database conventions

**No foreign keys.** Database tables must not declare `FOREIGN KEY` constraints or `REFERENCES` clauses. Referential integrity is enforced at the application layer: Go store methods explicitly delete child rows in the correct order before deleting a parent row. This applies to all migrations and to the SQLite test schema in `testdb_test.go`.

Consequences to keep in mind when writing store code:
- `DeleteWorkflow` must explicitly delete `node_configs`, `workflow_nodes`, `workflow_edges`, and `runs` before deleting the `workflows` row.
- `replaceNodesAndEdges` must explicitly delete `node_configs` before deleting `workflow_nodes`.
- `UpsertChunks` must explicitly delete existing `rag_chunks` for the document before inserting new ones.
- Node and edge IDs are workflow-scoped (composite PK on `workflow_id, id`). `node_configs` carries `workflow_id` so configs can be deleted with a simple `WHERE workflow_id = ?` rather than a subquery join through `workflow_nodes`.

### JSON conventions

All JSON struct tags and API request/response bodies use **snake_case** (`type_id`, `display_name`, `input_schema`, etc.). Never use camelCase in JSON tags.

### Authentication

All API routes (except `/health`, `/v1/config`, and the public auth endpoints) require a `Bearer` JWT token in the `Authorization` header. Tokens are issued by `POST /v1/auth/login`. The JWT carries `user_id`, `org_id`, `role`, and `permissions` claims.

Route protection uses three levels:
- **`public`** — no auth (health, config, webhook triggers, invite accept)
- **`authed`** — valid JWT required (`GET /v1/auth/me`)
- **`perm`** — valid JWT + a specific permission scope (e.g. `workflow:read`, `eval:write`)
- **`role`** — valid JWT + a specific role (e.g. `org_admin`, `system_admin`)

All data-scoped queries are automatically filtered by `org_id` from the JWT via `store.WithOrgID(ctx)`. Do not pass `org_id` as an explicit query parameter in store calls — it is injected by middleware.

### Frontend

React 18 + TypeScript. Key libraries: `@xyflow/react` (canvas), `zustand` (state), `@rjsf/core` (JSON schema-driven config forms), Tailwind CSS, Vite. The config sidebar renders node forms directly from `NodeMeta.input_schema` — no hand-written form components per node type. Communicates with the backend via REST + WebSocket (`/v1/runs/{run_id}/events`).
