# cogniflow — Workflow Evaluation Project Plan

> **Status:** Draft v0.1
> **Last Updated:** 2026-06-10
> **References:** REQUIREMENTS_EVAL.md v0.3 · ARCHITECTURE_EVAL.md v0.1

Each milestone leaves the system in a **runnable, demo-able state**. Later milestones build on earlier ones. No milestone is purely internal — every one can be exercised by starting the system and observing real behaviour via `curl` or the browser.

---

## Milestone Overview

| # | Milestone | Key Capability Unlocked |
|---|-----------|------------------------|
| ME1 | [Data Foundation & CRUD API](#me1-data-foundation--crud-api) | Eval suites and test cases can be created, saved, and retrieved via API |
| ME2 | [Eval Execution & Deterministic Graders](#me2-eval-execution--deterministic-graders) | Suites run end-to-end; string match, numeric, and JSON schema graders produce verdicts; node mocks intercept execution |
| ME3 | [LLM Graders](#me3-llm-graders) | LLM-as-judge and checklist graders produce AI-generated verdicts and partial scores |
| ME4 | [Frontend — Suite & Test Case Authoring](#me4-frontend--suite--test-case-authoring) | Full browser UI to create and configure suites, test cases, mocks, and all grader types |
| ME5 | [Frontend — Run & Observe](#me5-frontend--run--observe) | Trigger eval runs from the UI, watch results populate, drill into per-grader verdicts and checklist breakdowns |

---

## Dependency Graph

```
ME1 (Data Foundation & CRUD API)
 ├── ME2 (Eval Execution + Deterministic Graders)
 │    └── ME3 (LLM Graders)
 │                └─────────────────────────────────┐
 └── ME4 (Frontend — Authoring)                     │
                   └── ME5 (Frontend — Run & Observe)┘
```

ME2 and ME4 can be developed in parallel once ME1 is complete. ME5 requires both ME3 (full grader coverage) and ME4 (authoring UI).

---

## ME1: Data Foundation & CRUD API

**Goal:** The eval data model is fully persisted and accessible via a REST API. Developers can create eval suites, add test cases (with initial data, mocks, and grader definitions), list and delete them. Save-time validation catches invalid mock references and malformed grader configs before anything runs.

### Deliverables

- Migration `0012_create_eval_tables.up.sql` / `.down.sql` — creates `eval_suites`, `eval_test_cases`, `eval_runs`, `eval_test_case_results`
- `store/store.go` — add all eval types (`EvalSuite`, `TestCase`, `NodeMock`, `GraderDef`, `EvalRun`, `EvalRunStatus`, `EvalRunFilter`, `EvalRunCounts`, `TestCaseResult`) and new Store interface methods
- `store/mysql/eval_store.go` — MySQL implementation of all eval store methods; application-layer cascade order for `DeleteEvalSuite`
- `eval/vault.go` — `GraderVault`: `EncryptGraders`, `DecryptGraders`, `MaskGraders` using `*crypto.Cipher`
- `eval/handler.go` — HTTP handlers for EvalSuite CRUD + TestCase CRUD + reorder; save-time validation (`validateMocks`, `validateGraderConfigs` — validates regex patterns and JSON Schema objects); `evalHandler` struct wired with `store`, `vault`, and `registry` (for mock node_id validation)
- `api/router.go` — register all eval routes (suite + test case endpoints only; run endpoints deferred to ME2); add `*crypto.Cipher` to `NewRouter` signature

### Testable Criteria

```bash
# --- Create an eval suite ---
SUITE=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Smoke Suite",
    "description": "Basic checks",
    "pass_threshold": 0.8,
    "max_concurrency": 1
  }' | jq -r '.id')

echo $SUITE   # → "es-uuid-..."

# --- Retrieve the suite ---
curl http://localhost:8080/v1/eval-suites/$SUITE | jq '{name, pass_threshold}'
# → {"name":"Smoke Suite","pass_threshold":0.8}

# --- List suites for a workflow ---
curl http://localhost:8080/v1/workflows/$WF_ID/eval-suites | jq '.eval_suites | length'
# → 1

# --- Create a test case with mocks and graders ---
TC=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Happy path",
    "initial_data": {"ticket": "billing question"},
    "mocks": [
      {"node_id": "'$NODE_ID'", "output": {"status_code": 200, "body": "ok"}}
    ],
    "graders": [
      {
        "id": "g1",
        "name": "Status is 200",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "'$NODE_ID'",
        "config": {"field_path": "status_code", "operator": "==", "threshold": 200}
      }
    ]
  }' | jq -r '.id')

echo $TC   # → "tc-uuid-..."

# --- api_key in LLM grader config is returned masked ---
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "LLM check",
    "initial_data": {},
    "mocks": [],
    "graders": [{
      "id": "g2", "name": "Quality", "type": "llm_judge", "scope": "workflow",
      "config": {"provider":"openai","model":"gpt-4o","api_key":"sk-real-key","rubric":"Is it helpful?"}
    }]
  }' | jq '.graders[0].config.api_key'
# → "***"

# --- Invalid mock node_id rejected ---
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bad mock",
    "initial_data": {},
    "mocks": [{"node_id": "does-not-exist", "output": {}}],
    "graders": []
  }' | jq '.error.code'
# → "VALIDATION_FAILED"

# --- Invalid regex in grader rejected ---
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bad regex",
    "initial_data": {},
    "mocks": [],
    "graders": [{
      "id":"g3","name":"Regex check","type":"string_match","scope":"workflow",
      "config":{"field_path":"completion","match_type":"regex","expected_value":"(broken"}
    }]
  }' | jq '.error.code'
# → "VALIDATION_FAILED"

# --- Reorder test cases ---
TC2=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -d '{"name":"Second","initial_data":{},"mocks":[],"graders":[]}' \
  -H 'Content-Type: application/json' | jq -r '.id')

curl -s -X PUT http://localhost:8080/v1/eval-suites/$SUITE/test-cases/order \
  -H 'Content-Type: application/json' \
  -d '{"case_ids":["'$TC2'","'$TC'"]}'
# → 204 No Content

curl http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  | jq '[.test_cases[].name]'
# → ["Second","Happy path"]  (TC2 is now first)

# --- Delete test case ---
curl -s -X DELETE http://localhost:8080/v1/eval-suites/$SUITE/test-cases/$TC2
# → 204 No Content

# --- Delete suite cascades ---
curl -s -X DELETE http://localhost:8080/v1/eval-suites/$SUITE
# → 204 No Content

curl http://localhost:8080/v1/eval-suites/$SUITE | jq '.error.code'
# → "NOT_FOUND"
```

### Dependencies
ME1 requires the base system (M1–M2 from PROJECT_PLAN.md) — MySQL running, migrations applying, workflows CRUD working. The `$WF_ID` and `$NODE_ID` used above come from an existing workflow.

---

## ME2: Eval Execution & Deterministic Graders

**Goal:** Eval suites can be triggered via API. Each test case executes the workflow with its initial data, intercepts any mocked nodes, captures node outputs, and evaluates string match, numeric threshold, and JSON schema graders. Results — including per-grader verdicts — are persisted and queryable. The suite's configurable pass threshold determines whether each test case passed.

### Deliverables

- `trigger/types.go` — add `NodeMocks map[string]map[string]any` to `RunRequest` (nil = no mocks; zero-change for all existing callers)
- `engine/runner.go` — mock interception in `executeNode`: if `req.NodeMocks[node.ID]` exists, skip `Execute()`, emit `node.succeeded` event with mock output + `"mocked": true`, store in `ExecutionContext`; output parsers are not applied to mock outputs
- `eval/graders/string_match.go` — GR-01: exact / contains / regex match on a gjson field path
- `eval/graders/numeric.go` — GR-02: `field_path` + operator + threshold numeric comparison
- `eval/graders/json_schema.go` — GR-04: JSON Schema draft-07 validation using `github.com/santhosh-tekuri/jsonschema/v5`
- `eval/grader.go` — `Grader` interface, `GraderResult` / `CriterionResult` types, `BuildGrader()` dispatcher (routes to the three grader types above; returns `error` for `llm_judge` / `checklist` until ME3)
- `eval/runner.go` — `EvalRunner.Execute()` + `executeTestCase()`: semaphore-controlled test case execution, EventBus subscription for output capture, grader evaluation loop, pass rate computation, `TestCaseResult` persistence, atomic `eval_runs` count updates
- `eval/handler.go` updated — `TriggerRun`, `ListRuns`, `GetRun`, `GetTestCaseResult` endpoints wired to runner and store
- `api/router.go` updated — register run endpoints; add `*engine.WorkflowEngine` to `NewRouter` signature; construct `EvalRunner`

### Testable Criteria

```bash
# --- Trigger an eval run ---
EVAL_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/runs \
  -H 'Content-Type: application/json' -d '{}' | jq -r '.id')

echo $EVAL_RUN   # → "er-uuid-..."

# --- Poll until completed (or sleep 10s for a simple workflow) ---
sleep 10
curl http://localhost:8080/v1/eval-runs/$EVAL_RUN | jq '{status, passed_count, failed_count}'
# → {"status":"completed","passed_count":1,"failed_count":0}

# --- Verify per-grader verdicts ---
curl http://localhost:8080/v1/eval-runs/$EVAL_RUN | jq '
  .test_case_results[0] | {
    test_case_name,
    passed,
    verdicts: [.grader_results[].verdict]
  }'
# → {"test_case_name":"Happy path","passed":true,"verdicts":["pass"]}

# --- Verify node mock was used ---
# The mocked node's output shows "mocked":true in the captured node_outputs
TCR_ID=$(curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN \
  | jq -r '.test_case_results[0].id')

curl http://localhost:8080/v1/eval-runs/$EVAL_RUN/test-case-results/$TCR_ID \
  | jq '.node_outputs | to_entries[0].value.mocked'
# → true

# --- Verify failing string match grader ---
# Create a suite with a grader that expects content that won't be present
FAIL_TC=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Should fail",
    "initial_data": {},
    "mocks": [],
    "graders": [{
      "id":"gf1","name":"Must contain XYZZY","type":"string_match","scope":"workflow",
      "config":{"field_path":"n1.completion","match_type":"contains","expected_value":"XYZZY"}
    }]
  }' | jq -r '.id')

FAIL_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/runs -d '{}' \
  -H 'Content-Type: application/json' | jq -r '.id')
sleep 10
curl http://localhost:8080/v1/eval-runs/$FAIL_RUN | jq '{passed_count, failed_count}'
# → {"passed_count":1,"failed_count":1}  (first case still passes, new one fails)

# --- Verify numeric threshold grader ---
# Grader: completion_tokens <= 500 — should pass for a short response
curl http://localhost:8080/v1/eval-runs/$EVAL_RUN \
  | jq '.test_case_results[0].grader_results[] | select(.grader_type == "numeric_threshold")'
# → {"verdict":"pass","explanation":"","actual_value":42,...}

# --- Verify JSON schema grader ---
# Grader checks that the workflow output contains a "completion" string field
# A properly structured LLM output should pass
curl http://localhost:8080/v1/eval-runs/$EVAL_RUN \
  | jq '.test_case_results[0].grader_results[] | select(.grader_type == "json_schema") | .verdict'
# → "pass"

# --- List eval runs for a suite ---
curl "http://localhost:8080/v1/eval-suites/$SUITE/runs?limit=5" \
  | jq '.eval_runs | length'
# → 2

# --- Triggered_by = "eval" on underlying workflow runs ---
curl http://localhost:8080/v1/eval-runs/$EVAL_RUN \
  | jq '.test_case_results[0].workflow_run_id' | xargs -I{} \
  curl http://localhost:8080/runs/{} | jq '.triggered_by'
# → "eval"
```

### Dependencies
ME1 (eval data model), plus M3 from PROJECT_PLAN.md (execution engine and workflow runs must work).

---

## ME3: LLM Graders

**Goal:** LLM-as-judge and checklist graders are fully operational. The judge grader sends a rubric + node output to an LLM and returns a binary pass/fail verdict with an explanation. The checklist grader evaluates N criteria independently and returns a partial score with per-criterion results. API keys for judge configs are stored encrypted and returned masked.

### Deliverables

- `eval/graders/llm_judge.go` — GR-03: constructs the judge system prompt + user prompt, calls `aiprovider.LLMClient.Complete()`, parses `{"verdict":"...","explanation":"..."}` from the completion; graceful fallback on parse failure → `error` verdict; `field_path` support for targeting a single field vs. full output JSON
- `eval/graders/checklist.go` — GR-05: sends all criteria in a single LLM call, parses `[{"criterion":"...","met":bool,"explanation":"..."}]` array, computes `score = met/total`, applies grader-level `pass_threshold`; `GraderResult` includes `score` and `criteria_results`
- `eval/grader.go` updated — `BuildGrader()` now constructs `llm_judge` and `checklist` graders; takes `llmFactory func(provider string) (aiprovider.LLMClient, error)` parameter
- `eval/runner.go` updated — `llmFactory` injected into `EvalRunner`; passed to `BuildGrader()` at evaluation time
- `eval/handler.go` updated — `llmFactory` wired through from `NewRouter`
- `api/router.go` updated — constructs the `llmFactory` closure (creates OpenAI or Anthropic client based on `provider` string); passes it into `NewEvalRunner`

### Testable Criteria

```bash
# --- LLM judge grader: binary pass/fail with explanation ---
# Suite with an llm_judge grader on an LLM node's completion
LLM_TC=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Response quality",
    "initial_data": {"ticket": "My order is late"},
    "mocks": [],
    "graders": [{
      "id":"gj1","name":"Is response helpful","type":"llm_judge","scope":"node",
      "node_id":"'$LLM_NODE_ID'",
      "config":{
        "provider":"openai","model":"gpt-4o-mini","api_key":"sk-...",
        "rubric":"The response acknowledges the delay, apologises, and offers a resolution",
        "field_path":"completion"
      }
    }]
  }' | jq -r '.id')

JUDGE_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/runs -d '{}' \
  -H 'Content-Type: application/json' | jq -r '.id')
sleep 20
curl http://localhost:8080/v1/eval-runs/$JUDGE_RUN \
  | jq '.test_case_results[] | select(.test_case_name=="Response quality")
        | .grader_results[0] | {verdict, explanation}'
# → {"verdict":"pass","explanation":"The response acknowledges the delay and offers to track the order."}

# --- LLM judge grader: bad api_key returns error verdict, not a crash ---
BAD_TC=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"Bad key test","initial_data":{},"mocks":[],
    "graders":[{
      "id":"gj2","name":"Bad key","type":"llm_judge","scope":"workflow",
      "config":{"provider":"openai","model":"gpt-4o","api_key":"sk-invalid","rubric":"any","field_path":""}
    }]
  }' | jq -r '.id')

BAD_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/runs -d '{}' \
  -H 'Content-Type: application/json' | jq -r '.id')
sleep 15
curl http://localhost:8080/v1/eval-runs/$BAD_RUN \
  | jq '.test_case_results[] | select(.test_case_name=="Bad key test")
        | .grader_results[0] | {verdict, explanation}'
# → {"verdict":"error","explanation":"openai: 401 Incorrect API key provided"}

# --- Checklist grader: partial score ---
CHECKLIST_TC=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"Completeness check","initial_data":{"ticket":"billing issue"},"mocks":[],
    "graders":[{
      "id":"gc1","name":"Response completeness","type":"checklist","scope":"node",
      "node_id":"'$LLM_NODE_ID'",
      "config":{
        "provider":"openai","model":"gpt-4o-mini","api_key":"sk-...",
        "criteria":[
          "Acknowledges the billing issue",
          "Provides next steps for resolution",
          "Includes a contact method for follow-up",
          "Maintains a professional tone",
          "Avoids technical jargon"
        ],
        "pass_threshold": 0.6,
        "field_path":"completion"
      }
    }]
  }' | jq -r '.id')

CL_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/runs -d '{}' \
  -H 'Content-Type: application/json' | jq -r '.id')
sleep 20
curl http://localhost:8080/v1/eval-runs/$CL_RUN \
  | jq '.test_case_results[] | select(.test_case_name=="Completeness check")
        | .grader_results[0] | {verdict, score, explanation, criteria_results}'
# → {
#     "verdict": "pass",
#     "score": 0.8,
#     "explanation": "4 of 5 criteria met",
#     "criteria_results": [
#       {"criterion":"Acknowledges the billing issue","met":true,"explanation":"Opening line confirms receipt"},
#       {"criterion":"Provides next steps","met":true,"explanation":"Three steps listed"},
#       {"criterion":"Includes a contact method","met":false,"explanation":"No phone/email mentioned"},
#       {"criterion":"Professional tone","met":true,"explanation":"Formal language throughout"},
#       {"criterion":"Avoids jargon","met":true,"explanation":"Plain language used"}
#     ]
#   }

# --- Verify Anthropic provider works for checklist ---
# Same test but provider="anthropic" and a claude model
# Should produce equivalent results

# --- api_key stored encrypted; never returned in plain text ---
curl http://localhost:8080/v1/eval-suites/$SUITE/test-cases/$CHECKLIST_TC \
  | jq '.graders[0].config.api_key'
# → "***"
```

### Dependencies
ME2 (eval runner infrastructure must exist), ME1 (store). Also requires M6 from PROJECT_PLAN.md (AI provider clients must be working — OpenAI and/or Anthropic).

---

## ME4: Frontend — Suite & Test Case Authoring

**Goal:** A browser UI provides full authoring of eval suites and test cases. Users can create suites, add test cases with initial data (using RJSF if a workflow schema is declared), define node mocks, and configure any of the five grader types — including LLM judge and checklist — without writing any JSON manually.

### Deliverables

- `src/types/eval.ts` — all eval TypeScript types
- `src/stores/useEvalStore.ts` — Zustand store: suite list, active suite, test cases, active run
- `src/hooks/useApi.ts` updated — typed wrappers for all eval CRUD endpoints
- `src/pages/EvalSuiteListPage.tsx` — rendered as a new **Eval Suites** tab on `WorkflowEditorPage` (alongside the existing Run History tab); lists suites with name, test case count, last run badge, last run timestamp; Create Suite button
- `src/pages/EvalSuiteDetailPage.tsx` — suite metadata header (name, description, pass threshold, edit, Run Suite button), ordered `TestCaseList`, collapsible `EvalRunHistory` panel
- `src/components/eval/EvalSuiteForm.tsx` — create/edit modal: name, description, pass_threshold slider (0.0–1.0), max_concurrency input
- `src/components/eval/TestCaseList.tsx` — ordered list with drag-to-reorder (`@dnd-kit`); each row shows name, grader count, last verdict badge, edit/delete actions
- `src/components/eval/TestCaseEditor.tsx` — slide-over panel: name, description, `InitialDataSection` (RJSF form or JSON textarea), `MockList`, `GraderList`
- `src/components/eval/MockEditor.tsx` — node selector dropdown (workflow nodes by label) + JSON textarea for mock output
- `src/components/eval/GraderEditor.tsx` — name input, type dropdown (String Match / Numeric / LLM Judge / JSON Schema / Checklist), scope toggle (Workflow / Node), conditional node selector, `GraderTypeFields`
- `src/components/eval/GraderTypeFields.tsx` — type-switched sub-forms:
  - `StringMatchFields` — field_path, match_type radio, expected_value
  - `NumericFields` — field_path, operator select, threshold number input
  - `LLMJudgeFields` — provider select, model input, api_key password input, rubric textarea, optional field_path
  - `JSONSchemaFields` — optional field_path, schema JSON textarea with syntax hint
  - `ChecklistFields` — dynamic list of criterion text inputs (add/remove), pass_threshold input, provider/model/api_key/field_path
- `src/App.tsx` updated — add `/eval-suites/:suite_id` and `/eval-suites/:suite_id/test-cases/:case_id/edit` routes
- Save-time validation errors surfaced inline per-field using the existing `VALIDATION_FAILED` pattern (same as workflow save errors)

### Testable Criteria

```
1. Open a workflow → click the "Eval Suites" tab (new, alongside Run History)
   → EvalSuiteListPage renders; "No eval suites yet" empty state shown

2. Click "New Suite"
   → EvalSuiteForm modal opens with name, description, pass threshold (default 1.0), concurrency
   → Fill in name "Regression Suite"; set pass_threshold to 0.8; save
   → Suite card appears in the list showing name and "0 test cases"

3. Click the suite card → EvalSuiteDetailPage opens
   → Header shows name, pass threshold; "Add Test Case" button present

4. Click "Add Test Case"
   → TestCaseEditor slide-over opens
   → Enter name "Happy path"; workflow declares an initial_data_schema
     → Initial Data section renders an RJSF form with labelled inputs per field
   → Fill in initial data values

5. In Mocks section: click "Add Mock"
   → Node selector dropdown shows the workflow's nodes by label
   → Select a node; JSON textarea appears for mock output
   → Enter {"status_code": 200, "body": "ok"}

6. In Graders section: click "Add Grader"
   → Default type is String Match; scope defaults to Workflow
   → Change scope to "Node" → node selector appears; select the mocked node
   → Fill in: field_path = "status_code", match_type = "exact", expected_value = "200"
   → Click "Add Grader" again; type = "LLM Judge"
   → Fill in provider, model, api_key (shows as password), rubric text
   → Click "Add Grader" again; type = "Checklist"
   → Add 3 criteria using the "+ Add Criterion" button; set pass_threshold to 0.6
   → Grader list shows three grader rows with their names

7. Save the test case
   → No validation errors; test case appears in the suite with "3 graders" chip

8. Reorder: drag the test case row to a different position → order updates

9. Validation errors surface correctly:
   → Add a grader with an invalid regex ("(broken") → inline error below pattern field
   → Add a mock referencing a non-existent node_id → inline error below node selector
   → Both errors appear simultaneously; Save is blocked

10. Edit an existing test case
    → Slide-over pre-fills all existing values; api_key field shows "***" (masked)
    → Update rubric text; save → changes persist on reload
```

### Dependencies
ME1 (CRUD API must be running). Also requires M11 from PROJECT_PLAN.md (React frontend scaffold, `useWorkflowStore`, `useNodeTypeStore` must exist).

---

## ME5: Frontend — Run & Observe

**Goal:** Users can trigger eval suite runs from the browser, watch run status update in real time (via polling), and drill into the results — seeing per-test-case pass/fail, per-grader verdicts with explanations, and the full per-criterion breakdown for checklist graders. A direct link to the underlying workflow run allows debugging at the node level.

### Deliverables

- `src/stores/useEvalStore.ts` updated — `triggerRun`, `pollRunStatus` (polls `GET /v1/eval-runs/{id}` every 2 s until `status === "completed"` or `"failed"`), `loadRun`
- `src/hooks/useApi.ts` updated — typed wrappers for run trigger + result endpoints
- `EvalSuiteDetailPage` updated — **Run Suite** button calls `triggerRun`; navigates to `EvalRunDetailPage`; `EvalRunHistory` panel lists past runs with status badges and "N/M passed" summary
- `src/pages/EvalRunDetailPage.tsx` — run summary header (status, started, duration, total/passed/failed/error counts); `EvalRunResultsTable`; polling spinner while `status !== "completed"`
- `src/components/eval/EvalRunHistory.tsx` — table of past runs with short run ID, started_at, duration, status badge, pass summary
- `src/components/eval/EvalRunResultsTable.tsx` — expandable rows per TestCase:
  - Collapsed: test case name, workflow run status chip, overall verdict badge (green/red, based on `passed`)
  - Expanded: per-grader result rows + checklist detail
- `src/components/eval/GraderResultRow.tsx` — verdict badge (green `pass` / red `fail` / amber `error`), grader type chip, explanation text, collapsible `actual_value` pre block
- `src/components/eval/ChecklistResultDetail.tsx` — renders when `grader_type === "checklist"`; table with columns: criterion text | met badge (✓/✗) | explanation; score summary row at bottom ("4 of 5 criteria met — 80%")
- `src/App.tsx` updated — add `/eval-runs/:run_id` route

### Testable Criteria

```
1. Open EvalSuiteDetailPage for a suite with at least 2 test cases
   → Click "Run Suite"
   → Page navigates to EvalRunDetailPage
   → Status badge shows "running"; spinner is visible
   → Every 2 seconds the page updates (polling)

2. When run completes (status = "completed"):
   → Status badge turns green (all passed) or red/amber (some failed)
   → Summary header: "2/2 passed" or "1/2 passed" etc.
   → TestCaseResultsTable shows one row per test case

3. Expand a passing test case row
   → Each grader row shows a green "pass" badge
   → explanation text appears next to the LLM judge verdict
   → actual_value is visible in a collapsible pre block

4. Expand a checklist grader row
   → ChecklistResultDetail renders below the GraderResultRow
   → Per-criterion table: each criterion shows ✓ or ✗ and the judge explanation
   → Score summary: "4 of 5 criteria met — 80%"

5. Expand a failing test case row
   → At least one grader shows a red "fail" badge with explanation

6. A test case where the workflow run itself failed (e.g., network error in a node)
   → Row shows red engine-failure chip (distinct from grader fail)
   → Grader rows that could not evaluate show amber "error" badges
   → "View Run" link is present

7. Click "View Run" on any test case result
   → Navigates to the existing RunDetailPage for the underlying workflow run
   → Node status colours match what the eval run captured

8. Navigate back to EvalSuiteDetailPage
   → EvalRunHistory panel at the bottom lists the completed run
   → Row shows run ID (short), timestamp, duration, status, pass summary badge
   → Click the run ID → navigates back to EvalRunDetailPage

9. Trigger a second run
   → New row appears in EvalRunHistory
   → Each run's results are independent (editing test cases between runs
      does not retroactively change old results — snapshots are preserved)
```

### Dependencies
ME3 (all grader types including LLM graders must work), ME4 (authoring UI must exist for creating suites and test cases to run against).

---

## Milestone Dependency Graph (Summary)

```
ME1  ──► ME2  ──► ME3  ──────────────────────┐
 │                                            │
 └───────────────► ME4  ──────────────────► ME5
```

| Milestone | Parallel with | Gate |
|-----------|--------------|------|
| ME1 | — | M1–M2 from main project plan |
| ME2 | ME4 | ME1 complete; M3 from main plan |
| ME3 | ME4 | ME2 complete; M6 from main plan (AI providers) |
| ME4 | ME2, ME3 | ME1 complete; M11 from main plan (frontend scaffold) |
| ME5 | — | ME3 + ME4 both complete |
```
