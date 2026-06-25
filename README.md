# cogniflow

A visual workflow orchestration platform for building, configuring, and running AI-powered workflows from a browser canvas.

---

## Quick start

**Requirements:** Docker + Docker Compose (no Go or Node needed).

```bash
# 1. Clone
git clone https://github.com/g8rswimmer/cogniflow.git
cd cogniflow

# 2. Create your environment file
cp .env.example .env
```

Open `.env` and set the two required secrets:

```bash
# AES-256 key for encrypting sensitive node config values at rest
openssl rand -base64 32
# paste as COGNIFLOW_ENCRYPTION_KEY=<output>

# HMAC-SHA256 key for signing JWT tokens
openssl rand -hex 32
# paste as JWT_SECRET=<output>
```

Also set the bootstrap admin credentials so you can log in on first run:

```
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=<choose a strong password>
```

On first startup the server creates the "Default" org and this `system_admin` account automatically. Open `http://localhost:3000/login` and sign in with these credentials.

```bash
# 3. Start everything
docker compose up --build
```

Wait for the backend log line `server starting :8080`, then open:

```
http://localhost:3000
```

---

## What you can do

- **Drag-and-drop** node types onto the canvas and connect them with edges
- **Configure** each node in the right-hand sidebar — API keys, prompts, query DSNs
- **Save** and **run** workflows manually, on a schedule (cron), or from an inbound webhook
- **Watch** per-node status update live in the run panel as the workflow executes
- **Browse** run history and inspect each node's output
- **Evaluate** workflows with eval suites — define test cases with fixed inputs, node mocks, and graders (string match, numeric threshold, LLM-as-judge, JSON schema, checklist); trigger runs and drill into per-grader verdicts with full checklist breakdowns
- **Automate eval runs** on a cron schedule or from a CI pipeline via a per-suite inbound webhook (Bearer-token authenticated); the suite UI surfaces the one-time secret, a copy-ready curl example, and a Rotate Secret action

---

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `COGNIFLOW_ENCRYPTION_KEY` | **yes** | — | 32-byte AES-256 key (base64). Generate: `openssl rand -base64 32` |
| `JWT_SECRET` | **yes** | — | HMAC-SHA256 signing key for JWT tokens (≥ 32 chars). Generate: `openssl rand -hex 32` |
| `JWT_TTL` | no | `24h` | JWT token lifetime. Accepts Go duration strings: `1h`, `30m`, etc. |
| `BOOTSTRAP_ADMIN_EMAIL` | no | — | Email for the initial `system_admin` account created on first startup |
| `BOOTSTRAP_ADMIN_PASSWORD` | no | — | Password for the bootstrap admin (min 8 chars). Change before production. |
| `FRONTEND_URL` | no | `http://localhost:3000` | Public URL of the frontend — used to build accept-invite links in emails |
| `MYSQL_PASSWORD` | no | `cogniflow_pass` | MySQL app-user password |
| `MYSQL_ROOT_PASSWORD` | no | `cogniflow_root` | MySQL root password (init only) |
| `LOG_LEVEL` | no | `info` | `debug` \| `info` \| `warn` \| `error` |
| `PLUGIN_ADDRESSES` | no | — | Comma-separated gRPC addresses for external node plugins |
| `OLLAMA_BASE_URL` | no | — | Use Ollama for RAG embeddings instead of OpenAI (e.g. `http://localhost:11434`) |
| `COGNIFLOW_ALLOWED_ORIGIN` | no | — | CORS allowed origin when frontend and backend are on different hosts |
| `BACKEND_PORT` | no | `8080` | Host port for the backend |
| `FRONTEND_PORT` | no | `3000` | Host port for the frontend nginx |

All variables can be placed in a `.env` file at the repo root. See `.env.example` for a commented template.

---

## Development

```bash
# Backend (Go 1.22+) — from backend/
go run ./cmd/server          # requires DB_DSN and COGNIFLOW_ENCRYPTION_KEY in env
go test ./...
golangci-lint run ./...

# Frontend (Node 20+) — from frontend/
npm install
npm run dev                  # Vite dev server at http://localhost:3000
                             # proxies /v1/* to http://localhost:8080 automatically
```

Run the database only:

```bash
docker compose up mysql
```

Then start the backend locally with:

```bash
cd backend
export DB_DSN="cogniflow:cogniflow_pass@tcp(localhost:3306)/cogniflow?parseTime=true&multiStatements=true"
export COGNIFLOW_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export JWT_SECRET="$(openssl rand -hex 32)"
export BOOTSTRAP_ADMIN_EMAIL=admin@example.com
export BOOTSTRAP_ADMIN_PASSWORD=localdevpassword
go run ./cmd/server
```

---

## Try the demo workflow

[`DEMO.md`](DEMO.md) walks through building a complete **IT Support Ticket Triage** workflow end-to-end: an Anthropic LLM node classifies urgency, a visual conditional node routes tickets to an escalation or standard-reply branch, and a merge node collects the result. It covers every major feature — output parsers, template variables, edge labels, live run events, and run history.

For the **Workflow Evaluation** feature, see [`DEMO_ME4.md`](DEMO_ME4.md) (suite and test case authoring) and [`DEMO_ME5.md`](DEMO_ME5.md) (triggering runs and inspecting results).

For **Automated Eval Runs**, see [`MEF2_DEMO.md`](MEF2_DEMO.md) (cron scheduling), [`MEF3_DEMO.md`](MEF3_DEMO.md) (CI webhook trigger), and [`MEF4_DEMO.md`](MEF4_DEMO.md) (frontend trigger UI).

---

## Architecture

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the full system design: package structure, Go interfaces, MySQL schema, REST API contract, WebSocket event protocol, gRPC plugin protocol, and Docker Compose service graph.

The project follows a 15-milestone plan documented in [`PROJECT_PLAN.md`](PROJECT_PLAN.md).
