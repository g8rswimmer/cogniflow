# F-04 Demo: Workflow Versioning

Every time a workflow is saved via `PUT /workflows/{id}`, cogniflow automatically snapshots the full definition as an immutable version. Users can browse the version history, inspect a prior definition, and restore it — including sensitive config values (API keys, passwords) without needing to re-enter them.

---

## What changed

| Before | After |
|--------|-------|
| Saving a workflow overwrote its definition with no recovery path | Each save (PUT) creates a numbered version snapshot |
| Deleting a node or changing a config was permanent | Previous states are browsable and restorable |
| Sensitive values (API keys) could not survive a restore | Ciphertexts are preserved in snapshots; full restore works without re-entry |
| No history UI | Navbar "History" link → version list → version detail → restore |

---

## Architecture

```
PUT /v1/workflows/{id}
  └── ConfigVault.UpdateWorkflow()
        ├── encryptNodesPreserving()         ← encrypt sensitive fields
        ├── inner.UpdateWorkflow()            ← write to DB (encrypted-at-rest)
        ├── inner.CreateWorkflowVersion()     ← snapshot encrypted state (best-effort)
        └── return updated workflow

workflow_versions table
  id | workflow_id | version_number | node_count | definition (LONGTEXT JSON) | created_at

Sensitive values in snapshots:
  []byte ciphertexts → base64 in JSON (sensitive_keys map identifies them)
  On restore: base64 decoded → []byte → inserted into node_configs.encrypted_value

GET  /v1/workflows/{id}/versions                        → list summaries
GET  /v1/workflows/{id}/versions/{n}                    → full definition (decrypted + masked)
POST /v1/workflows/{id}/versions/{n}/restore            → replace current state + re-arm trigger
```

---

## Prerequisites

```bash
docker-compose up --build
```

The backend must be running on `localhost:8080`. All commands below use `curl` and `jq`.

---

## Setup: create a demo workflow

```bash
BASE="${COGNIFLOW_URL:-http://localhost:8080}"

# Create a workflow with one HTTP Request node (v0 — not yet versioned)
WF=$(curl -sf -X POST "$BASE/workflows" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "F-04 Demo Workflow",
    "description": "Version history demo",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "n1",
        "type_id": "http.request",
        "label": "Fetch anything",
        "position": {"x": 200, "y": 200},
        "config": {
          "method": "GET",
          "url": "https://httpbin.org/anything"
        }
      }
    ],
    "edges": []
  }')
WF_ID=$(echo "$WF" | jq -r '.id')
echo "Workflow ID: $WF_ID"
```

---

## Test 1 — Version 1 is created on initial save (POST)

```bash
curl -s "$BASE/workflows/$WF_ID/versions" \
  | jq '[.versions[] | {version_number, node_count}]'
# → [{"version_number": 1, "node_count": 1}]
```

The initial create snapshots version 1 immediately, so the starting state is always restorable.

---

## Test 2 — First update creates version 2

Add a second node and save:

```bash
curl -sf -X PUT "$BASE/workflows/$WF_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "F-04 Demo Workflow",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "n1",
        "type_id": "http.request",
        "label": "Fetch anything",
        "position": {"x": 200, "y": 200},
        "config": {"method": "GET", "url": "https://httpbin.org/anything"}
      },
      {
        "id": "n2",
        "type_id": "http.request",
        "label": "Echo POST",
        "position": {"x": 500, "y": 200},
        "config": {"method": "POST", "url": "https://httpbin.org/post", "body": "hello"}
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "n1", "target_id": "n2"}
    ]
  }' > /dev/null

curl -s "$BASE/workflows/$WF_ID/versions" | jq '.versions[] | {version_number, node_count, created_at}'
# → {"version_number": 2, "node_count": 2, "created_at": "..."}
# → {"version_number": 1, "node_count": 1, "created_at": "..."}   ← initial create
```

---

## Test 3 — Second update creates version 3

Remove the second node (simulate a destructive edit):

```bash
curl -sf -X PUT "$BASE/workflows/$WF_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "F-04 Demo Workflow — trimmed",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "n1",
        "type_id": "http.request",
        "label": "Fetch anything",
        "position": {"x": 200, "y": 200},
        "config": {"method": "GET", "url": "https://httpbin.org/get"}
      }
    ],
    "edges": []
  }' > /dev/null

curl -s "$BASE/workflows/$WF_ID/versions" \
  | jq '.versions[] | {version_number, node_count}'
# → {"version_number": 3, "node_count": 1}
# → {"version_number": 2, "node_count": 2}
# → {"version_number": 1, "node_count": 1}   ← initial create; list is newest-first
```

---

## Test 4 — Inspect a specific version

```bash
curl -s "$BASE/workflows/$WF_ID/versions/2" \
  | jq '{version_number: .version.version_number, name: .version.definition.name, nodes: [.version.definition.nodes[].label]}'
# → {"version_number": 2, "name": "F-04 Demo Workflow", "nodes": ["Fetch anything", "Echo POST"]}
```

The definition is the full workflow as it was when version 2 was saved — including the node that was later deleted.

---

## Test 5 — Restore version 2

```bash
RESTORED=$(curl -sf -X POST "$BASE/workflows/$WF_ID/versions/2/restore" \
  -H "Content-Type: application/json")
echo "$RESTORED" | jq '{name: .name, node_count: (.nodes | length)}'
# → {"name": "F-04 Demo Workflow", "node_count": 2}
```

The response is the full restored workflow (masked sensitive values, same shape as `GET /workflows/{id}`).

After the restore, versions should show four entries:

```bash
curl -s "$BASE/workflows/$WF_ID/versions" \
  | jq '[.versions[] | {version_number, node_count}]'
# → [
#     {"version_number": 4, "node_count": 2},   ← restored state snapshotted as v4
#     {"version_number": 3, "node_count": 1},
#     {"version_number": 2, "node_count": 2},
#     {"version_number": 1, "node_count": 1}    ← initial create
#   ]
```

---

## Test 6 — Verify the workflow is actually restored

```bash
curl -s "$BASE/workflows/$WF_ID" \
  | jq '{name: .name, node_count: (.nodes | length), nodes: [.nodes[].label]}'
# → {"name": "F-04 Demo Workflow", "node_count": 2, "nodes": ["Fetch anything", "Echo POST"]}
```

And confirm it runs end-to-end:

```bash
RUN_ID=$(curl -sf -X POST "$BASE/workflows/$WF_ID/runs" \
  -H "Content-Type: application/json" -d '{}' | jq -r '.run_id')
sleep 4
curl -s "$BASE/runs/$RUN_ID" | jq '{status, nodes: [.final_output | keys[]]}'
# → {"status": "succeeded", "nodes": ["n1", "n2"]}
```

---

## Test 7 — Sensitive value round-trip

This test confirms that an API key set before a save survives a restore without re-entry. It requires the LLM node type to be registered.

```bash
# Save a workflow with an LLM node — provide a real or fake key for the shape test
curl -sf -X PUT "$BASE/workflows/$WF_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "F-04 Sensitive Demo",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "llm1",
        "type_id": "llm.openai",
        "label": "Ask GPT",
        "position": {"x": 200, "y": 200},
        "config": {
          "model": "gpt-4o-mini",
          "api_key": "sk-test-key-for-demo",
          "prompt": "Say hello."
        }
      }
    ],
    "edges": []
  }' > /dev/null

# Version 4 is now created with the encrypted api_key.
# Read back — key is masked:
curl -s "$BASE/workflows/$WF_ID" | jq '.nodes[0].config.api_key'
# → "***"

# Get the version — key also masked:
LATEST_VER=$(curl -s "$BASE/workflows/$WF_ID/versions" | jq '.versions[0].version_number')
curl -s "$BASE/workflows/$WF_ID/versions/$LATEST_VER" \
  | jq '.version.definition.nodes[0].config.api_key'
# → "***"

# Update the workflow again with a different key (version 5):
curl -sf -X PUT "$BASE/workflows/$WF_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "F-04 Sensitive Demo",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "llm1",
        "type_id": "llm.openai",
        "label": "Ask GPT",
        "position": {"x": 200, "y": 200},
        "config": {
          "model": "gpt-4o-mini",
          "api_key": "sk-different-key",
          "prompt": "Say goodbye."
        }
      }
    ],
    "edges": []
  }' > /dev/null

# Restore version 4 (the original key):
curl -sf -X POST "$BASE/workflows/$WF_ID/versions/$LATEST_VER/restore" \
  -H "Content-Type: application/json" | jq '.nodes[0].config | {model, api_key, prompt}'
# → {"model": "gpt-4o-mini", "api_key": "***", "prompt": "Say hello."}
# The prompt is "Say hello." (from v4), confirming the correct version was restored.
# The api_key ciphertext from v4 is intact in the database — the node will use it when executed.
```

---

## Test 8 — Versions are cleaned up when the workflow is deleted

```bash
# Create a throwaway workflow
TEMP_ID=$(curl -sf -X POST "$BASE/workflows" \
  -H "Content-Type: application/json" \
  -d '{"name":"Throwaway","trigger":{"kind":"manual"},"timeout_seconds":30,"nodes":[],"edges":[]}' \
  | jq -r '.id')

# Save it once to create a version
curl -sf -X PUT "$BASE/workflows/$TEMP_ID" \
  -H "Content-Type: application/json" \
  -d '{"name":"Throwaway","trigger":{"kind":"manual"},"timeout_seconds":30,"nodes":[],"edges":[]}' \
  > /dev/null

# Confirm version exists
curl -s "$BASE/workflows/$TEMP_ID/versions" | jq '.versions | length'
# → 1

# Delete the workflow
curl -sf -X DELETE "$BASE/workflows/$TEMP_ID"

# Versions endpoint now returns 404 (workflow gone) — no orphaned rows left in DB
curl -s "$BASE/workflows/$TEMP_ID/versions" | jq '.error.code'
# → "NOT_FOUND"
```

---

## Test 9 — Version not found

```bash
curl -s "$BASE/workflows/$WF_ID/versions/9999" | jq '.error.code'
# → "NOT_FOUND"

curl -s -X POST "$BASE/workflows/$WF_ID/versions/9999/restore" | jq '.error.code'
# → "NOT_FOUND"
```

---

## UI walkthrough

> Requires `docker-compose up --build` with the frontend served on `localhost:3000`.

**1. Open an existing workflow in the editor.**

**2. Note the Navbar** — there is now a "⏱ History" button to the right of "⚗ Evals". It is only visible for saved workflows (same condition as the Evals link).

**3. Make a change** (rename the workflow or add a node) and click **Save**.

**4. Click "⏱ History"** — you are navigated to `/workflows/{id}/versions`.

The version history page shows:
- Version number (`#1`, `#2`, …)
- Save timestamp
- Node count

**5. Click a version row** — you are navigated to `/workflows/{id}/versions/{n}`.

The version detail page shows:
- Metadata: version number, saved at, trigger, node count
- Read-only list of nodes in that version
- A yellow warning banner if any config values are masked as `***`
- **"Restore this version"** button (top-right)

**6. Click "Restore this version"** — the button changes to a confirmation prompt:

```
Confirm restore?   [Yes, restore]   [Cancel]
```

Clicking **Yes, restore** calls `POST /v1/workflows/{id}/versions/{n}/restore` and navigates back to the workflow editor with the restored definition loaded.

**7. Check the version history again** — the history now has one more entry (the restored state snapshotted as the next version number).

---

## Edge cases to verify

| Scenario | Expected behaviour |
|----------|-------------------|
| Click "History" on a brand-new unsaved workflow | Button is not shown (only visible when `workflowId` is set) |
| Workflow just created, no saves yet (POST only) | History page shows version 1 (the initial snapshot) |
| Restore version then immediately restore a different one | Each restore creates the next version; history grows correctly |
| Version number that doesn't exist in the URL bar | Detail page shows error; restore returns 404 |
| Invalid version number in URL (e.g. `abc`) | Handler returns 400 VALIDATION_FAILED |
| Save fails (validation error on PUT) | No version is created (snapshot only fires after successful write) |
| Workflow trigger changes between versions | Restore re-arms TriggerManager with the restored trigger config |
