# ME2 Demo — Eval Execution & Deterministic Graders

> **Milestone:** ME2 from `PROJECT_PLAN_EVAL.md`
> **What you can show:** Eval suites run end-to-end. Each test case triggers a real workflow run (with optional node mocks), captures per-node outputs, evaluates string-match, numeric-threshold, and JSON-schema graders, and persists structured results — including per-grader verdicts. The suite's pass threshold determines whether each test case passed.

---

## Prerequisites

1. `.env` has `COGNIFLOW_ENCRYPTION_KEY` set:
   ```bash
   openssl rand -base64 32
   # paste into .env as COGNIFLOW_ENCRYPTION_KEY=<value>
   ```

2. Start the stack from the repo root:
   ```bash
   docker compose up --build -d
   ```

3. Verify healthy (~15 s after startup):
   ```bash
   curl -s http://localhost:8080/health | jq .
   # → {"status":"ok"}
   ```

---

## Step 1 — Create a workflow with one HTTP node

No API keys are required. All test cases either mock this node or exercise it against the public httpbin.org endpoint.

```bash
WF=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Fetch Slideshow",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "http-1",
        "type_id": "http.request",
        "label": "Fetch JSON",
        "position": {"x": 100, "y": 100},
        "config": {
          "url": "https://httpbin.org/json",
          "method": "GET"
        }
      }
    ],
    "edges": []
  }')

WF_ID=$(echo $WF | jq -r '.id')
echo "Workflow: $WF_ID"
```

---

## Step 2 — Create an eval suite with pass_threshold 0.8

```bash
SUITE=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Regression Suite",
    "description": "ME2 demo suite",
    "pass_threshold": 0.8,
    "max_concurrency": 1
  }')

SUITE_ID=$(echo $SUITE | jq -r '.id')
echo "Suite: $SUITE_ID"
```

---

## Step 3 — Create test case: happy path with mock + numeric grader

The HTTP node is mocked to return a canned response, so no real HTTP call is made. The numeric grader checks that the mock's `status_code` field equals 200.

```bash
TC1=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Happy path — mocked HTTP",
    "initial_data": {"ticket": "billing question"},
    "mocks": [
      {
        "node_id": "http-1",
        "output": {"status_code": 200, "body": "{\"accountId\":\"A-123\"}"}
      }
    ],
    "graders": [
      {
        "id": "g1",
        "name": "HTTP status is 200",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "http-1",
        "config": {
          "field_path": "status_code",
          "operator": "==",
          "threshold": 200
        }
      }
    ]
  }')

TC1_ID=$(echo $TC1 | jq -r '.id')
echo "Test case 1: $TC1_ID"
```

---

## Step 4 — Create test case: string-match grader on live HTTP response

No mocks — the real httpbin.org endpoint is called. The httpbin `/json` response always contains the string `"slideshow"`, so this grader should pass.

The `field_path` `"http-1.body"` navigates the workflow's final output: `{"http-1": {"body": "...", ...}}`.

```bash
TC2=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Live HTTP — body contains slideshow",
    "initial_data": {},
    "mocks": [],
    "graders": [
      {
        "id": "g2",
        "name": "Body contains slideshow",
        "type": "string_match",
        "scope": "workflow",
        "config": {
          "field_path": "http-1.body",
          "match_type": "contains",
          "expected_value": "slideshow"
        }
      }
    ]
  }')

TC2_ID=$(echo $TC2 | jq -r '.id')
echo "Test case 2: $TC2_ID"
```

---

## Step 5 — Create test case: JSON-schema grader

Check that the workflow's final output for `http-1` is an object (JSON Schema `type: object`).

```bash
TC3=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "http-1 output is an object",
    "initial_data": {"ticket": "test"},
    "mocks": [
      {
        "node_id": "http-1",
        "output": {"status_code": 200, "body": "ok"}
      }
    ],
    "graders": [
      {
        "id": "g3",
        "name": "http-1 output is object",
        "type": "json_schema",
        "scope": "node",
        "node_id": "http-1",
        "config": {
          "schema": {
            "type": "object",
            "properties": {
              "status_code": {"type": "number"},
              "body": {"type": "string"}
            },
            "required": ["status_code"]
          }
        }
      }
    ]
  }')

TC3_ID=$(echo $TC3 | jq -r '.id')
echo "Test case 3: $TC3_ID"
```

---

## Step 6 — Create test case: intentional failure

The string-match grader looks for `"XYZZY"` — a string that will never appear in a real response. This case should fail.

```bash
TC4=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Should fail — XYZZY not present",
    "initial_data": {"ticket": "test"},
    "mocks": [
      {
        "node_id": "http-1",
        "output": {"status_code": 200, "body": "normal response"}
      }
    ],
    "graders": [
      {
        "id": "g4",
        "name": "Body contains XYZZY",
        "type": "string_match",
        "scope": "node",
        "node_id": "http-1",
        "config": {
          "field_path": "body",
          "match_type": "contains",
          "expected_value": "XYZZY"
        }
      }
    ]
  }')

TC4_ID=$(echo $TC4 | jq -r '.id')
echo "Test case 4: $TC4_ID"
```

---

## Step 7 — Trigger the eval run

Returns `201` immediately; execution is async.

```bash
EVAL_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  -H 'Content-Type: application/json' -d '{}')

EVAL_RUN_ID=$(echo $EVAL_RUN | jq -r '.id')
echo "Eval run: $EVAL_RUN_ID"
```

---

## Step 8 — Poll until completed

TC1, TC3, and TC4 use mocks and complete immediately. TC2 makes a real HTTP call to httpbin.org — allow up to 15 s for that one. The whole suite usually finishes in under 20 s.

```bash
# Poll every 3 s until status changes from "pending"/"running"
for i in $(seq 1 20); do
  STATUS=$(curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq -r '.status')
  echo "Status: $STATUS"
  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then break; fi
  sleep 3
done
```

---

## Step 9 — Inspect the results

```bash
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq '{
  status,
  total_cases,
  passed_count,
  failed_count,
  error_count
}'
# → {
#     "status": "completed",
#     "total_cases": 4,
#     "passed_count": 3,
#     "failed_count": 1,
#     "error_count": 0
#   }
```

---

## Step 10 — Drill into per-grader verdicts

```bash
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq '
  .test_case_results[] | {
    name: .test_case_name,
    passed: .passed,
    verdicts: [.grader_results[].verdict]
  }'
# → {"name":"Happy path — mocked HTTP","passed":true,"verdicts":["pass"]}
# → {"name":"HTTP response contains slideshow","passed":true,"verdicts":["pass"]}
# → {"name":"http-1 output is an object","passed":true,"verdicts":["pass"]}
# → {"name":"Should fail — XYZZY not present","passed":false,"verdicts":["fail"]}
```

---

## Step 11 — Inspect node_outputs (debug view)

Retrieve the full TestCaseResult for the mock test case to see the captured node output.

```bash
TCR1_ID=$(curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID \
  | jq -r --arg name "Happy path — mocked HTTP" \
    '.test_case_results[] | select(.test_case_name==$name) | .id')

curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID/test-case-results/$TCR1_ID \
  | jq '{node_outputs, grader_results: [.grader_results[].verdict]}'
# → {
#     "node_outputs": {
#       "http-1": {"status_code": 200, "body": "...", "mocked": true}
#     },
#     "grader_results": ["pass"]
#   }
```

The `"mocked": true` field confirms the engine intercepted the node and used the canned output.

---

## Step 12 — Verify triggered_by = "eval" on the underlying runs

Eval-triggered workflow runs are tagged `triggered_by = "eval"` in the runs table.

```bash
WF_RUN_ID=$(curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID \
  | jq -r '.test_case_results[0].workflow_run_id')

curl -s http://localhost:8080/v1/runs/$WF_RUN_ID | jq '.triggered_by'
# → "eval"
```

---

## Step 13 — List runs for the suite (pagination)

```bash
curl -s "http://localhost:8080/v1/eval-suites/$SUITE_ID/runs?limit=5" \
  | jq '.eval_runs | length'
# → 1

# Trigger a second run to verify pagination
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  -H 'Content-Type: application/json' -d '{}'

sleep 2

curl -s "http://localhost:8080/v1/eval-suites/$SUITE_ID/runs?limit=5" \
  | jq '[.eval_runs[].status]'
# → ["completed", "running"]   (second run in progress)
```

---

## What ME2 delivers

| Capability | Implementation |
|------------|---------------|
| `POST /v1/eval-suites/{id}/runs` — trigger async run, returns run ID | `eval/runner.go EvalRunner.Execute()` |
| `GET /v1/eval-runs/{id}` — run status + all test case results | `eval/handler.go GetRun` |
| `GET /v1/eval-runs/{id}/test-case-results/{result_id}` — full result with `node_outputs` | `eval/handler.go GetTestCaseResult` |
| Node mock interception (`"mocked": true` in output; output parsers skipped) | `engine/runner.go executeNode` |
| `NodeMocks` field on `trigger.RunRequest` | `trigger/types.go` |
| String-match grader (exact / contains / regex) | `eval/graders/string_match.go` |
| Numeric-threshold grader (==, !=, >, >=, <, <=) | `eval/graders/numeric.go` |
| JSON Schema grader (draft-07 via `santhosh-tekuri/jsonschema/v5`) | `eval/graders/json_schema.go` |
| `BuildGrader()` dispatcher; returns error for llm_judge / checklist (ME3) | `eval/grader.go` |
| Per-node output capture for node-scoped graders | `eval/runner.go executeTestCase` |
| Pass-rate threshold per suite (configurable 0.0–1.0) | `eval/runner.go` |
| `triggered_by = "eval"` on underlying workflow runs | `engine/engine.go` |

## Next: ME3

ME3 adds LLM-as-judge and checklist graders — the two grader types that require an external LLM API call.
