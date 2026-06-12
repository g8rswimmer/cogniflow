# ME1 Demo — Eval Data Foundation & CRUD API

> **Milestone:** ME1 from `PROJECT_PLAN_EVAL.md`
> **What you can show:** Eval suites and test cases can be created, saved, and retrieved via API. Save-time validation catches invalid mock references and malformed grader configs before anything runs. Sensitive `api_key` values are stored encrypted and returned masked.

---

## Prerequisites

1. `.env` has `COGNIFLOW_ENCRYPTION_KEY` set (generate one if needed):
   ```bash
   openssl rand -base64 32
   # paste the output into .env as COGNIFLOW_ENCRYPTION_KEY=<value>
   ```

2. Start the stack from the repo root:
   ```bash
   docker compose up --build -d
   ```

3. Verify the server is healthy (~15 s after startup):
   ```bash
   curl -s http://localhost:8080/health | jq .
   # → {"status":"ok"}
   ```

---

## Step 1 — Create a workflow

Eval suites are linked to a workflow. Create one with two nodes so mock node ID validation has something to check against.

```bash
WF=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Support Ticket Router",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [
      {
        "id": "http-1",
        "type_id": "http.request",
        "label": "Fetch Account",
        "position": {"x": 100, "y": 100},
        "config": {
          "url": "https://httpbin.org/get",
          "method": "GET"
        }
      },
      {
        "id": "llm-1",
        "type_id": "llm.openai",
        "label": "Draft Reply",
        "position": {"x": 300, "y": 100},
        "config": {
          "prompt": "You are a support agent. Respond to: {{.initial_data.ticket}}"
        }
      }
    ],
    "edges": [
      {"id": "e1", "source_id": "http-1", "target_id": "llm-1"}
    ]
  }')

echo $WF | jq '{id, name}'
# → {"id":"wf-uuid-...","name":"Support Ticket Router"}

WF_ID=$(echo $WF | jq -r '.id')
NODE_ID="http-1"
```

---

## Step 2 — Create an eval suite

```bash
SUITE=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Smoke Suite",
    "description": "Basic regression checks",
    "pass_threshold": 0.8,
    "max_concurrency": 1
  }')

echo $SUITE | jq '{id, name, pass_threshold}'
# → {"id":"es-uuid-...","name":"Smoke Suite","pass_threshold":0.8}

SUITE_ID=$(echo $SUITE | jq -r '.id')
```

---

## Step 3 — Retrieve the suite and list suites for the workflow

```bash
# Get suite — shows test_cases array (empty so far)
curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID \
  | jq '{name, pass_threshold, test_cases}'
# → {"name":"Smoke Suite","pass_threshold":0.8,"test_cases":[]}

# List all suites for this workflow
curl -s http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  | jq '.eval_suites | length'
# → 1
```

---

## Step 4 — Create a test case with a mock and a grader

```bash
TC=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Happy path",
    "initial_data": {"ticket": "billing question"},
    "mocks": [
      {
        "node_id": "'$NODE_ID'",
        "output": {"status_code": 200, "body": "ok"}
      }
    ],
    "graders": [
      {
        "id": "g1",
        "name": "Status is 200",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "'$NODE_ID'",
        "config": {
          "field_path": "status_code",
          "operator": "==",
          "threshold": 200
        }
      }
    ]
  }')

echo $TC | jq '{id, name}'
# → {"id":"tc-uuid-...","name":"Happy path"}

TC_ID=$(echo $TC | jq -r '.id')
```

---

## Step 5 — Verify api_key is masked in responses

LLM grader configs contain a sensitive `api_key`. The server encrypts it at rest (AES-256-GCM) and always returns `"***"` in API responses — the plaintext key is never exposed after the initial write.

```bash
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "LLM quality check",
    "initial_data": {},
    "mocks": [],
    "graders": [{
      "id": "g2",
      "name": "Quality",
      "type": "llm_judge",
      "scope": "workflow",
      "config": {
        "provider": "openai",
        "model": "gpt-4o",
        "api_key": "sk-real-key",
        "rubric": "Is the response helpful?"
      }
    }]
  }' | jq '.graders[0].config.api_key'
# → "***"
```

---

## Step 6 — Validation: invalid mock node_id rejected

The server validates mock `node_id` values against the linked workflow's node list at save time.

```bash
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bad mock",
    "initial_data": {},
    "mocks": [{"node_id": "does-not-exist", "output": {}}],
    "graders": []
  }' | jq '.error'
# → {
#     "code": "VALIDATION_FAILED",
#     "message": "Test case validation failed: 1 error(s)",
#     "details": {
#       "validation_errors": [
#         {"field":"mocks[0].node_id","message":"node ID \"does-not-exist\" not found in workflow"}
#       ]
#     }
#   }
```

---

## Step 7 — Validation: invalid regex pattern rejected

```bash
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bad regex",
    "initial_data": {},
    "mocks": [],
    "graders": [{
      "id": "g3",
      "name": "Regex check",
      "type": "string_match",
      "scope": "workflow",
      "config": {
        "field_path": "completion",
        "match_type": "regex",
        "expected_value": "(broken"
      }
    }]
  }' | jq '.error.code'
# → "VALIDATION_FAILED"
```

---

## Step 8 — Reorder test cases

```bash
# Add a second test case
TC2=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Edge case",
    "initial_data": {"ticket": "refund request"},
    "mocks": [],
    "graders": []
  }' | jq -r '.id')

# Current order
curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  | jq '[.test_cases[].name]'
# → ["Happy path","Edge case"]   (Happy path was created first)

# Put Edge case first
curl -s -X PUT http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases/order \
  -H 'Content-Type: application/json' \
  -d '{"case_ids":["'$TC2'","'$TC_ID'"]}'
# → 204 No Content

# Verify new order
curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  | jq '[.test_cases[].name]'
# → ["Edge case","Happy path"]
```

---

## Step 9 — Delete a test case

```bash
curl -s -X DELETE \
  http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases/$TC2
# → 204 No Content

curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  | jq '.test_cases | length'
# → 1   (only Happy path remains)
```

---

## Step 10 — Delete the suite (cascades to all test cases)

```bash
curl -s -X DELETE http://localhost:8080/v1/eval-suites/$SUITE_ID
# → 204 No Content

curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID | jq '.error.code'
# → "NOT_FOUND"
```

---

## What ME1 delivers

| Capability | Implementation |
|------------|---------------|
| Eval suite CRUD (`/v1/workflows/{id}/eval-suites`, `/v1/eval-suites/{id}`) | `internal/eval/handler.go` |
| Test case CRUD with mocks + graders (`/v1/eval-suites/{id}/test-cases`) | `internal/eval/handler.go` |
| Test case reorder | `store.ReorderTestCases` |
| Save-time mock validation (node IDs checked against live workflow) | `validateMocks()` |
| Save-time grader validation (regex compiled; JSON Schema parsed) | `validateGraderConfigs()` |
| `api_key` AES-256-GCM encrypted at rest, returned as `"***"` | `internal/eval/vault.go` |
| Cascade delete (results → runs → test cases → suite) | `eval_store.go DeleteEvalSuite` |
| DB schema (`eval_suites`, `eval_test_cases`, `eval_runs`, `eval_test_case_results`) | `migrations/0012_create_eval_tables` |

## Next: ME2

ME2 adds eval run execution — trigger a suite run, have each test case execute the workflow with mocked nodes, capture per-node outputs, evaluate string match / numeric / JSON schema graders, and persist per-grader verdicts.
