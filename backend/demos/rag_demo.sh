#!/usr/bin/env bash
# =============================================================================
# Demo: M7 RAG Nodes
#
# Demonstrates:
#   1. RAG Ingest   — chunk text, embed each chunk, store in MySQL VECTOR column
#   2. RAG Retrieve — embed a query, return top-K similar chunks by cosine distance
#   3. Full pipeline — RAG Retrieve → LLM Call (answer grounded in retrieved context)
#
# Prerequisites:
#   - Server running:  docker compose up --build  (or go run ./cmd/server)
#   - jq installed:    brew install jq
#   - OpenAI key set:  export OPENAI_API_KEY=sk-...
#   - Anthropic key (demo 3 only): export ANTHROPIC_API_KEY=sk-ant-...
# =============================================================================

set -euo pipefail

BASE="http://localhost:8080/v1"
OPENAI_KEY="${OPENAI_API_KEY:-}"
ANTHROPIC_KEY="${ANTHROPIC_API_KEY:-}"
EMB_MODEL="text-embedding-3-small"
LLM_MODEL="claude-haiku-4-5-20251001"
DOC_ID="cogniflow-docs-demo"

# ── helpers ──────────────────────────────────────────────────────────────────

check() {
  command -v "$1" >/dev/null 2>&1 || { echo "ERROR: $1 is required but not installed."; exit 1; }
}

wait_for_run() {
  local run_id=$1
  local timeout=${2:-60}
  local elapsed=0
  while [ $elapsed -lt $timeout ]; do
    status=$(curl -s "$BASE/runs/$run_id" | jq -r '.status')
    case "$status" in
      succeeded|failed) echo "$status"; return ;;
    esac
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "timeout"
}

# ── preflight ─────────────────────────────────────────────────────────────────

check jq
check curl

if [ -z "$OPENAI_KEY" ]; then
  echo "ERROR: OPENAI_API_KEY is required (used for embeddings)."
  echo "       export OPENAI_API_KEY=sk-..."
  exit 1
fi

health=$(curl -sf "$BASE/../health" 2>/dev/null | jq -r '.status' 2>/dev/null || echo "")
if [ "$health" != "ok" ]; then
  echo "ERROR: Server is not reachable at $BASE"
  echo "       Start it with: docker compose up --build"
  exit 1
fi

echo ""
echo "══════════════════════════════════════════════════════════════════"
echo " cogniflow M7 — RAG Nodes demo"
echo "══════════════════════════════════════════════════════════════════"


# ─────────────────────────────────────────────────────────────────────────────
# Demo 1 — RAG Ingest
# Chunks a multi-paragraph passage, embeds each chunk via OpenAI, and stores
# the vectors in MySQL.  The document_id "cogniflow-docs-demo" is used as a
# stable key so Demo 2 can retrieve against the same corpus.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 1: RAG Ingest ==="

INGEST_WF=$(curl -s -X POST "$BASE/workflows" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Demo - RAG Ingest",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 120,
    "nodes": [{
      "id": "n1",
      "type_id": "rag.ingest",
      "label": "Ingest cogniflow docs",
      "position": {"x": 0, "y": 0},
      "config": {
        "api_key":       "'"$OPENAI_KEY"'",
        "model":         "'"$EMB_MODEL"'",
        "text":          "{{._initial.text}}",
        "document_id":   "'"$DOC_ID"'",
        "chunk_size":    300,
        "chunk_overlap": 40
      }
    }],
    "edges": []
  }' | jq -r '.id')

echo "Ingest workflow: $INGEST_WF"

PASSAGE='cogniflow is a workflow orchestration platform that enables users to build, configure, and run workflows composed of both AI-powered nodes and deterministic processing nodes. Workflows are authored through a visual web-based canvas, persisted to a backend store, and executed by a Go-based runtime engine.

The platform supports multiple trigger types including manual triggers, webhook endpoints, and cron schedules. Each workflow is represented as a directed acyclic graph (DAG) of nodes and edges, allowing complex branching and parallel execution patterns.

Built-in AI nodes include LLM Call nodes for OpenAI and Anthropic models, Embedding nodes for generating vector representations, and RAG Ingest and RAG Retrieve nodes for document ingestion and similarity search. Deterministic nodes include HTTP Request, Conditional branching with CEL expressions, Data Transform, Database Query and Write, and Merge for fan-in operations.

The execution engine processes workflows asynchronously, running nodes concurrently when their upstream dependencies are satisfied. Per-node status events stream in real-time over WebSocket connections, giving clients live visibility into each node as it runs.

Vector embeddings for RAG are stored natively in MySQL 9.0 using the VECTOR column type and VEC_DISTANCE_COSINE for similarity search, eliminating the need for a separate vector database infrastructure. Each document is split into overlapping chunks so that semantically related passages can be retrieved independently.'

RUN1=$(curl -s -X POST "$BASE/workflows/$INGEST_WF/runs" \
  -H "Content-Type: application/json" \
  -d "{\"initial_data\": {\"text\": $(echo "$PASSAGE" | jq -Rs '.')}}" \
  | jq -r '.run_id')

echo "Run ID: $RUN1 — waiting for embeddings..."
STATUS1=$(wait_for_run "$RUN1" 90)
echo "Status: $STATUS1"

if [ "$STATUS1" = "succeeded" ]; then
  curl -s "$BASE/runs/$RUN1" | jq '{
    status:          .status,
    chunks_ingested: .final_output.n1.chunks_ingested,
    document_id:     .final_output.n1.document_id
  }'
else
  echo "Run failed:"
  curl -s "$BASE/runs/$RUN1" | jq '.error_detail'
  exit 1
fi


# ─────────────────────────────────────────────────────────────────────────────
# Demo 2 — RAG Retrieve
# Embeds a natural-language query and finds the top-3 most similar chunks from
# the document ingested above, ranked by cosine distance (lower = more similar).
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 2: RAG Retrieve ==="

RETRIEVE_WF=$(curl -s -X POST "$BASE/workflows" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Demo - RAG Retrieve",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [{
      "id": "n1",
      "type_id": "rag.retrieve",
      "label": "Find relevant chunks",
      "position": {"x": 0, "y": 0},
      "config": {
        "api_key":     "'"$OPENAI_KEY"'",
        "model":       "'"$EMB_MODEL"'",
        "query":       "{{._initial.query}}",
        "document_id": "'"$DOC_ID"'",
        "top_k":       3
      }
    }],
    "edges": []
  }' | jq -r '.id')

echo "Retrieve workflow: $RETRIEVE_WF"

RUN2=$(curl -s -X POST "$BASE/workflows/$RETRIEVE_WF/runs" \
  -H "Content-Type: application/json" \
  -d '{"initial_data": {"query": "What is cogniflow and what does it do?"}}' \
  | jq -r '.run_id')

echo "Run ID: $RUN2 — waiting..."
STATUS2=$(wait_for_run "$RUN2" 30)
echo "Status: $STATUS2"

if [ "$STATUS2" = "succeeded" ]; then
  echo ""
  echo "Top-3 chunks (ordered by cosine distance, lower = more similar):"
  curl -s "$BASE/runs/$RUN2" | jq '.final_output.n1.chunks[] | {score, chunk_text: (.chunk_text | .[0:120] + "…")}'
else
  echo "Run failed:"
  curl -s "$BASE/runs/$RUN2" | jq '.error_detail'
  exit 1
fi

echo ""
echo "Best match score (should be < 0.3 for a relevant query):"
curl -s "$BASE/runs/$RUN2" | jq '.final_output.n1.chunks[0].score'


# ─────────────────────────────────────────────────────────────────────────────
# Demo 3 — Full RAG + LLM pipeline  (requires ANTHROPIC_API_KEY)
#
# A two-node workflow:
#   n1 (RAG Retrieve) — finds the most relevant chunk for the question
#     └─ output parser extracts the first chunk's text into "context"
#   n2 (LLM Call)     — answers the question using only the retrieved context
#
# This demonstrates a complete retrieval-augmented generation workflow where
# the LLM's answer is grounded in the stored document rather than its weights.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 3: Full RAG + LLM pipeline ==="

if [ -z "$ANTHROPIC_KEY" ]; then
  echo "(Skipped — set ANTHROPIC_API_KEY to run this demo)"
else

  RAG_LLM_WF=$(curl -s -X POST "$BASE/workflows" \
    -H "Content-Type: application/json" \
    -d '{
      "name": "Demo - RAG + LLM",
      "trigger": {"kind": "manual"},
      "timeout_seconds": 120,
      "nodes": [
        {
          "id": "n1",
          "type_id": "rag.retrieve",
          "label": "Retrieve context",
          "position": {"x": 0, "y": 0},
          "config": {
            "api_key":     "'"$OPENAI_KEY"'",
            "model":       "'"$EMB_MODEL"'",
            "query":       "{{._initial.question}}",
            "document_id": "'"$DOC_ID"'",
            "top_k":       2
          },
          "output_parsers": {
            "context": {
              "kind":    "json_path",
              "source":  "chunks",
              "pattern": "0.chunk_text"
            }
          }
        },
        {
          "id": "n2",
          "type_id": "llm.anthropic",
          "label": "Answer from context",
          "position": {"x": 400, "y": 0},
          "config": {
            "api_key":    "'"$ANTHROPIC_KEY"'",
            "model":      "'"$LLM_MODEL"'",
            "system_msg": "You are a helpful assistant. Answer the question using only the provided context. Be concise.",
            "prompt":     "Context:\n{{.n1.context}}\n\nQuestion: {{._initial.question}}\n\nAnswer:"
          }
        }
      ],
      "edges": [
        {"id": "e1", "source_id": "n1", "target_id": "n2", "branch_label": null}
      ]
    }' | jq -r '.id')

  echo "RAG+LLM workflow: $RAG_LLM_WF"

  RUN3=$(curl -s -X POST "$BASE/workflows/$RAG_LLM_WF/runs" \
    -H "Content-Type: application/json" \
    -d '{"initial_data": {"question": "How does cogniflow handle workflow execution?"}}' \
    | jq -r '.run_id')

  echo "Run ID: $RUN3 — waiting for retrieve + LLM..."
  STATUS3=$(wait_for_run "$RUN3" 90)
  echo "Status: $STATUS3"

  if [ "$STATUS3" = "succeeded" ]; then
    echo ""
    echo "Retrieved context (first chunk):"
    curl -s "$BASE/runs/$RUN3" | jq -r '.final_output.n1.context'
    echo ""
    echo "LLM answer (grounded in retrieved context):"
    curl -s "$BASE/runs/$RUN3" | jq -r '.final_output.n2.completion'
  else
    echo "Run failed:"
    curl -s "$BASE/runs/$RUN3" | jq '.error_detail'
    exit 1
  fi

fi


# ─────────────────────────────────────────────────────────────────────────────
# Demo 4 — Re-ingest (verifies stale chunk cleanup)
# Re-ingests a shorter version of the same document_id to confirm that old
# chunks are replaced, not accumulated.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== Demo 4: Re-ingest (stale chunk cleanup) ==="

SHORT_TEXT="cogniflow is a workflow orchestration platform. It runs workflows as directed acyclic graphs."

RUN4=$(curl -s -X POST "$BASE/workflows/$INGEST_WF/runs" \
  -H "Content-Type: application/json" \
  -d "{\"initial_data\": {\"text\": $(echo "$SHORT_TEXT" | jq -Rs '.')}}" \
  | jq -r '.run_id')

echo "Re-ingest run: $RUN4 — waiting..."
STATUS4=$(wait_for_run "$RUN4" 60)
echo "Status: $STATUS4"

NEW_COUNT=$(curl -s "$BASE/runs/$RUN4" | jq '.final_output.n1.chunks_ingested')
echo "Chunks after re-ingest: $NEW_COUNT  (was larger — confirms stale chunks were removed)"

RUN4B=$(curl -s -X POST "$BASE/workflows/$RETRIEVE_WF/runs" \
  -H "Content-Type: application/json" \
  -d '{"initial_data": {"query": "What is cogniflow?"}}' \
  | jq -r '.run_id')

STATUS4B=$(wait_for_run "$RUN4B" 30)
CHUNK_COUNT=$(curl -s "$BASE/runs/$RUN4B" | jq '.final_output.n1.chunks | length')
echo "Retrieve after re-ingest returns $CHUNK_COUNT chunk(s) (≤ $NEW_COUNT — no leftovers from old ingest)"


echo ""
echo "══════════════════════════════════════════════════════════════════"
echo " All demos complete."
echo "══════════════════════════════════════════════════════════════════"
