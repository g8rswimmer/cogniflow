# cogniflow — Workflow Evaluation Requirements

> **Status:** Draft v0.3
> **Last Updated:** 2026-06-10
> **Depends on:** REQUIREMENTS.md v0.4

---

## 1. Overview

The Workflow Evaluation feature lets authors define automated quality tests against their workflows. An **EvalSuite** is a named collection of test cases. Each test case supplies fixed initial data, optional per-node mocks to stub out side-effectful nodes, and a set of graders that assert the correctness of either the workflow's final output or an individual node's output.

Running a suite triggers one real workflow execution per test case, captures intermediate node outputs, evaluates every grader, and persists a structured result record. The primary use cases are:

- Regression testing after changing prompt text or node configuration
- Confidence scoring of LLM pipelines before switching models
- Continuous quality monitoring triggered on a schedule or from CI

---

## 2. Goals

| # | Goal |
|---|------|
| EG1 | Allow authors to define suites of test cases with fixed inputs for a workflow |
| EG2 | Support five grader types: string match, numeric threshold, LLM-as-judge, JSON schema validation, and checklist (partial scoring) |
| EG3 | Grade at both the workflow level (final merged output) and the node level (a specific node's output) |
| EG4 | Persist eval suites and run results as first-class resources alongside workflows |
| EG5 | Provide a UI for authoring suites, running them, and inspecting results |
| EG6 | Support opt-in node mocking to avoid side effects (HTTP calls, DB writes) during evaluation |

---

## 3. Scope

### In Scope (v1)

- EvalSuite CRUD (name, description, linked to a workflow)
- TestCase CRUD with initial data, per-node mocks, and graders
- Five grader types (string match, numeric threshold, LLM-as-judge, JSON schema, checklist)
- Graders scoped to workflow-level output or a specific node's output
- Eval run trigger, async execution, result persistence
- Configurable pass-rate threshold per suite
- Per-TestCase result with per-grader verdict + explanation
- Web UI for suite authoring, run triggering, and result inspection

### Out of Scope (v1)

- Scheduled/automated eval runs (v2)
- CI webhook trigger for evals (v2)
- Dataset import (CSV / JSONL → test cases) (v2)
- Baseline comparison across eval runs (v2)
- Custom grader plugins via gRPC (v2)
- Eval result streaming via WebSocket (results are polled or page-loaded after completion)

---

## 4. Core Concepts

**EvalSuite** — A named, persisted collection of test cases associated with exactly one workflow. Contains a configurable `pass_threshold` (0.0–1.0, default 1.0 meaning all graders must pass).

**TestCase** — One scenario within an EvalSuite. Carries the `initial_data` fed to the workflow, zero or more NodeMocks, and one or more Graders.

**NodeMock** — An optional override for a single node in a TestCase. When a matching node is about to execute, its real `Execute()` is bypassed and the mock's `output` map is returned instead.

**Grader** — An assertion attached to either the workflow's final merged output (`scope: workflow`) or a specific node's output (`scope: node`). Each Grader produces a verdict of `pass`, `fail`, or `error`.

**GraderResult** — The outcome of one Grader evaluation: verdict, explanation (mandatory for LLM-as-judge), and the actual value inspected.

**EvalRun** — One execution of an EvalSuite. Triggers one workflow run per TestCase, evaluates all graders, and persists the aggregated results. Has a high-level status of `pending`, `running`, `completed`, or `failed`.

**TestCaseResult** — The outcome of one TestCase within an EvalRun. Holds the underlying workflow run ID, per-node output snapshots, and the list of GraderResults.

---

## 5. Functional Requirements

### 5.1 EvalSuite Management (EV)

| ID | Requirement |
|----|-------------|
| EV-01 | Users can create an EvalSuite linked to a specific workflow. Required field: `name`. Optional: `description`, `pass_threshold` (float 0.0–1.0, default 1.0). |
| EV-02 | Users can retrieve a single EvalSuite by ID, including its ordered list of TestCase summaries (ID, name, grader count, last verdict). |
| EV-03 | Users can list all EvalSuites for a given workflow, ordered by `created_at` descending. |
| EV-04 | Users can update an EvalSuite's `name`, `description`, and `pass_threshold`. |
| EV-05 | Users can delete an EvalSuite. At the application layer, deletion cascades to all TestCases and all EvalRun records for that suite in the correct dependency order. |
| EV-06 | If the linked workflow is deleted, the suite and its run history are retained but the suite is marked `workflow_deleted: true`. Triggering a new EvalRun on an orphaned suite returns an error. |
| EV-07 | Each EvalSuite records `created_at` and `updated_at` timestamps. |

### 5.2 TestCase Definition (TC)

| ID | Requirement |
|----|-------------|
| TC-01 | Users can add a TestCase to an EvalSuite. Required field: `name`. Optional: `description`. |
| TC-02 | Each TestCase carries an `initial_data` field (`map[string]any`) supplied to the workflow as the run's initial data. When the linked workflow declares an initial data schema (WF-09 in REQUIREMENTS.md), that schema is used to validate and render the TestCase's `initial_data` — the UI presents a RJSF form instead of a free-form JSON textarea (see UI-05). The schema is advisory: mismatched or missing fields do not block execution, consistent with WF-09. An empty map `{}` is always valid. |
| TC-03 | TestCases within a suite are ordered. Users can reorder them (display order only; does not affect execution). |
| TC-04 | Users can update any field of a TestCase (name, description, initial_data, mocks, graders). |
| TC-05 | Users can delete a TestCase. Historical EvalRun results retain a snapshot of the test case name at the time of the run and are unaffected. |
| TC-06 | A TestCase may have zero graders (smoke test — only verifies the workflow completes without error). In this case, the TestCase contributes a `pass` to the suite's pass rate if the workflow run succeeds. |

### 5.3 Node Mocking (MK)

| ID | Requirement |
|----|-------------|
| MK-01 | A NodeMock is defined per `(TestCase, node_id)` pair. When the eval engine encounters that node ID during execution, `Execute()` is bypassed and the mock's `output` map is returned as the node's output. |
| MK-02 | Mock `node_id` values are validated at save time against the current workflow's node list. If a referenced node ID does not exist in the workflow, the save is rejected with a `VALIDATION_FAILED` error. |
| MK-03 | The mock `output` must be a valid JSON object (`map[string]any`). Output parsers (ND-09 in REQUIREMENTS.md) are **not** applied to mock outputs — the value is used exactly as supplied, keeping mocks explicit and deterministic. |
| MK-04 | A TestCase may mock any number of nodes, including zero. |
| MK-05 | Mocked nodes still emit `node.pending`, `node.running`, and `node.succeeded` events on the EventBus, with `"mocked": true` included in the output map, so the real-time event feed remains consistent. |
| MK-06 | The mock interception layer is internal to the eval engine and does not modify the core `WorkflowEngine` or `NodeRegistry`. |

### 5.4 Grader Types (GR)

All graders operate on a `map[string]any` (either the workflow's final merged output or a specific node's output). A grader produces one of three verdicts:
- **`pass`** — assertion succeeded
- **`fail`** — assertion evaluated and was false
- **`error`** — the grader could not evaluate (missing field, LLM call failure, misconfiguration)

`field_path` in all grader configs uses gjson dot-path syntax — the same syntax used by existing output parsers (ND-09 in REQUIREMENTS.md).

#### GR-01 String Match Grader

| Sub-ID | Requirement |
|--------|-------------|
| GR-01a | Config: `field_path` (string, required), `match_type` (`exact` / `contains` / `regex`), `expected_value` (string, required). |
| GR-01b | `exact`: resolved value coerced to string must equal `expected_value` (case-sensitive). |
| GR-01c | `contains`: resolved value coerced to string must contain `expected_value` as a substring (case-sensitive). |
| GR-01d | `regex`: resolved value coerced to string must match `expected_value` compiled as a Go `regexp`. Pattern validated at save time; invalid pattern → `VALIDATION_FAILED`. |
| GR-01e | If `field_path` resolves to a missing or null value → verdict `error` with message "field not found". |
| GR-01f | Non-string resolved values are coerced with `fmt.Sprintf("%v", v)`. |

#### GR-02 Numeric Threshold Grader

| Sub-ID | Requirement |
|--------|-------------|
| GR-02a | Config: `field_path` (string, required), `operator` (`==` / `!=` / `>` / `>=` / `<` / `<=`), `threshold` (number, required). |
| GR-02b | The resolved value at `field_path` must be a JSON number (float64); other types → verdict `error` with message "field is not numeric". |
| GR-02c | The comparison `resolved_value <operator> threshold` is evaluated. `pass` if true, `fail` if false. |
| GR-02d | Intended for: token counts, scores, numeric fields extracted by output parsers. |

#### GR-03 LLM-as-Judge Grader

| Sub-ID | Requirement |
|--------|-------------|
| GR-03a | Config: `provider` (`openai` / `anthropic`), `model` (string), `api_key` (string, sensitive, stored encrypted), `rubric` (string, required — the evaluation criteria in natural language), `field_path` (string, optional — if provided, only the resolved field is sent to the judge; if absent, the entire output is serialised as JSON). |
| GR-03b | The engine constructs a judge prompt at evaluation time. The judge LLM is instructed via system prompt to respond with only: `{"verdict": "pass"|"fail", "explanation": "..."}`. |
| GR-03c | If the judge LLM call fails (network error, rate limit, etc.) → verdict `error` with the provider error message. |
| GR-03d | If the judge response cannot be parsed as the expected JSON → verdict `error` with message "judge response could not be parsed". |
| GR-03e | The `explanation` from the judge is stored in `GraderResult.explanation` regardless of verdict. |
| GR-03f | `api_key` is stored encrypted at rest using the same `ConfigVault` / AES-256-GCM pattern as node sensitive config. The key is never returned in plain text via the API (masked as `***`). |
| GR-03g | Provider and model are configurable per grader instance, allowing mixed providers within the same suite. |
| GR-03h | The judge uses the existing `aiprovider.LLMClient` interface. No new AI provider abstraction is required. |

#### GR-04 JSON Schema Validation Grader

| Sub-ID | Requirement |
|--------|-------------|
| GR-04a | Config: `field_path` (string, optional), `schema` (JSON Schema draft-07 object, required). |
| GR-04b | Target value (full output or resolved field) is validated against `schema`. `pass` if valid; `fail` if invalid with validation error messages in `explanation`. |
| GR-04c | `schema` is validated at save time; invalid JSON Schema → `VALIDATION_FAILED`. |
| GR-04d | Implementation uses `github.com/santhosh-tekuri/jsonschema/v5`. |

#### GR-05 Checklist Grader

| Sub-ID | Requirement |
|--------|-------------|
| GR-05a | Config: `provider` (`openai` / `anthropic`), `model` (string), `api_key` (string, sensitive, stored encrypted), `criteria` (array of strings, required — each string is one independently evaluated criterion), `pass_threshold` (float 0.0–1.0, default 1.0 — fraction of criteria that must be met for the grader to pass), `field_path` (string, optional — same semantics as GR-03a). |
| GR-05b | At evaluation time the engine sends the target content and the full criteria list to the LLM in a single call. The LLM is instructed to respond with a JSON array of per-criterion objects: `[{"criterion": "...", "met": true|false, "explanation": "..."}]`. |
| GR-05c | The grader computes `score = criteria_met / total_criteria`. The verdict is `pass` if `score >= pass_threshold`, otherwise `fail`. |
| GR-05d | `GraderResult` for a checklist grader includes a `score` field (float, e.g. `0.6`) and a `criteria_results` array (per-criterion met/explanation) in addition to the standard verdict and explanation. The top-level `explanation` summarises the count (e.g. "3 of 5 criteria met"). |
| GR-05e | If the LLM response cannot be parsed as the expected JSON array → verdict `error` with message "checklist response could not be parsed". |
| GR-05f | If the LLM call fails → verdict `error` with the provider error message. |
| GR-05g | `api_key` follows the same encryption and masking rules as GR-03f. |
| GR-05h | `criteria` must contain at least 1 item. Validated at save time; empty array → `VALIDATION_FAILED`. |
| GR-05i | The judge uses the existing `aiprovider.LLMClient` interface. No new AI provider abstraction is required. |

#### GR-06 Grader Scope

| ID | Requirement |
|----|-------------|
| GR-06a | Each grader carries a `scope` field: `workflow` or `node`. |
| GR-06b | `scope: workflow` — grader is applied to the run's final merged output (same as `runs.final_output`). |
| GR-06c | `scope: node` — grader requires a `node_id` field. Grader is applied to that node's output map as captured during the eval run. |
| GR-06d | Both scope types may coexist within the same TestCase, evaluated independently. |
| GR-06e | If `scope: node` and the specified node did not execute (e.g., pruned by conditional routing) → verdict `error` with message "node did not execute". |

### 5.5 Pass Rate Threshold (PR)

| ID | Requirement |
|----|-------------|
| PR-01 | Each EvalSuite has a `pass_threshold` field (float, 0.0–1.0, default 1.0). |
| PR-02 | A TestCase's **pass rate** is: `(number of graders with verdict "pass") / (total graders)`. A TestCase with zero graders that completed successfully has a pass rate of 1.0. |
| PR-03 | A TestCase is considered **passed** when its pass rate ≥ the suite's `pass_threshold`. |
| PR-04 | An EvalRun's summary includes: total test cases, passed count, failed count, error count (cases where the workflow run itself failed before graders could be evaluated). |
| PR-05 | An EvalRun does not itself have a single pass/fail verdict — only the per-TestCase passed/failed status and the aggregate counts. |

### 5.6 Eval Execution Engine (EX)

| ID | Requirement |
|----|-------------|
| EX-01 | `POST /v1/eval-suites/{suite_id}/runs` returns `201 Created` with the EvalRun ID immediately. Execution proceeds asynchronously in a background goroutine — identical to how `WorkflowEngine.Run()` works today. |
| EX-02 | Each TestCase produces exactly one workflow run. The run is created with `triggered_by = "eval"`. |
| EX-03 | **Per-node output capture:** The eval engine subscribes to the EventBus for each workflow run and extracts node output data from `node.succeeded` events. This captured data is stored in `eval_test_case_results.node_outputs` for use by node-scoped graders and the debug UI. The regular `runs` table and `WorkflowEngine` are unchanged. |
| EX-04 | After a workflow run completes (succeeded or failed), all graders on that TestCase are evaluated sequentially. |
| EX-05 | If the workflow run fails before a grader's target node produced output → verdict `error` for that grader with message "node did not execute". Graders targeting nodes that did complete are still evaluated. |
| EX-06 | TestCases within an EvalRun execute sequentially by default (`max_concurrency = 1`). A suite-level `max_concurrency` field (integer ≥ 1) allows parallel execution when the author is confident about provider rate limits. Default: 1. |
| EX-07 | The eval engine respects each workflow run's configured `timeout_seconds`. |
| EX-08 | Only per-node outputs referenced by at least one `scope: node` grader in the TestCase are captured and stored. If a TestCase has no node-scoped graders, `node_outputs` is stored as `{}`. |
| EX-09 | LLM-as-judge API calls are made using the `aiprovider` factory already wired into the server. The eval engine receives the factory as a constructor dependency. |
| EX-10 | The EvalRun's context is cancelled on server shutdown; outstanding workflow runs and judge calls respect Go context cancellation. |

### 5.7 Results Persistence (RS)

| ID | Requirement |
|----|-------------|
| RS-01 | All EvalRun results are persisted to the database. Results are never stored in memory only. |
| RS-02 | Users can list EvalRuns for a suite ordered by `started_at` descending, with `limit` + `offset` pagination. Filter by `status` is supported. |
| RS-03 | A single EvalRun retrieval returns the overall status, summary counts (total, passed, failed, error), and the list of TestCaseResults. |
| RS-04 | Each TestCaseResult includes: TestCase name (snapshot at run time), workflow run ID, workflow run status, and the list of GraderResults. |
| RS-05 | Each GraderResult includes: grader name (snapshot), grader type, verdict, explanation, and the actual value inspected (stored as JSON for display). |
| RS-06 | Users can retrieve a single TestCaseResult including full `node_outputs` (for debugging node-level grader failures). |
| RS-07 | EvalRun history is retained indefinitely in v1 (no expiration). |
| RS-08 | Deleting an EvalSuite cascades at the application layer to all TestCases, EvalRuns, and TestCaseResults in dependency order (consistent with the project's no-FK convention). |

### 5.8 Web Interface (UI)

| ID | Requirement |
|----|-------------|
| UI-01 | A per-workflow **Eval Suites** tab is added to the workflow detail view (alongside Run History). It shows all suites for the workflow: name, test case count, last run status badge, last run timestamp. |
| UI-02 | Users can create a new EvalSuite from the list view. |
| UI-03 | The EvalSuite detail page shows suite metadata (name, description, pass threshold) and an ordered list of TestCases with name, grader count, and last verdict summary. |
| UI-04 | Users can add, edit, reorder, and delete TestCases from the suite detail page. |
| UI-05 | The TestCase editor is a form with: name, description, initial data (RJSF form if the workflow has a declared initial data schema, else free-form JSON textarea — same logic as the existing Run modal), a Mocks section, and a Graders section. |
| UI-06 | The Mocks section allows adding node mocks. Each mock has a node selector (dropdown of the workflow's nodes) and a JSON textarea for the mock output. |
| UI-07 | The Graders section allows adding graders. Each grader has: name, type dropdown (String Match / Numeric Threshold / LLM-as-Judge / JSON Schema / Checklist), scope toggle (Workflow / Node), and when Node is selected a node selector. The form fields for each type render appropriately (see GR-01 through GR-05 for fields). The Checklist grader form renders a dynamic list of criterion text inputs and a `pass_threshold` field. |
| UI-08 | A **Run Suite** button on the suite detail page triggers a new EvalRun and navigates to the EvalRun result page (polling until `status = completed`). |
| UI-09 | The EvalRun history list (accessible from the suite detail) shows: run ID (short), started at, duration, status, summary badge (e.g., "4/5 passed"). |
| UI-10 | The EvalRun detail page shows per-TestCase results with expandable rows. Expanding a row reveals each GraderResult with a verdict badge (green/red/amber), grader type, and explanation. |
| UI-11 | If a TestCase's workflow run failed at the engine level, this is surfaced prominently in the row with the run error detail. A "View Run" link navigates to the existing run detail page. |
| UI-12 | Save-time validation errors (invalid regex, bad JSON schema, unknown node ID in mock) follow the existing `VALIDATION_FAILED` error format. |
| UI-13 | LLM-as-judge `api_key` fields in the UI are password inputs. The value is never returned from the API in plain text (masked as `***`). |

---

## 6. API Design

All endpoints under `/v1/`. Follows existing snake_case JSON convention and `{"error": {"code": "...", "message": "..."}}` error shape.

### EvalSuite

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/workflows/{workflow_id}/eval-suites` | List suites for a workflow |
| `POST` | `/v1/workflows/{workflow_id}/eval-suites` | Create suite |
| `GET` | `/v1/eval-suites/{suite_id}` | Get suite with TestCase summaries |
| `PUT` | `/v1/eval-suites/{suite_id}` | Update suite name / description / threshold |
| `DELETE` | `/v1/eval-suites/{suite_id}` | Delete suite and all data |

### TestCase

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/eval-suites/{suite_id}/test-cases` | List test cases |
| `POST` | `/v1/eval-suites/{suite_id}/test-cases` | Create test case |
| `GET` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | Get single test case |
| `PUT` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | Replace test case |
| `DELETE` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | Delete test case |
| `PUT` | `/v1/eval-suites/{suite_id}/test-cases/order` | Reorder test cases |

### EvalRun

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/eval-suites/{suite_id}/runs` | Trigger a new EvalRun |
| `GET` | `/v1/eval-suites/{suite_id}/runs` | List EvalRuns (`?status=&limit=&offset=`) |
| `GET` | `/v1/eval-runs/{eval_run_id}` | Get full EvalRun with all TestCaseResults |
| `GET` | `/v1/eval-runs/{eval_run_id}/test-case-results/{result_id}` | Get single TestCaseResult with `node_outputs` |

`runs.triggered_by` is already `VARCHAR(20)` — no migration needed to add the value `"eval"`.

---

## 7. Data Model

No `FOREIGN KEY` constraints. Referential integrity enforced at the application layer. All IDs are `VARCHAR(36)` (UUID). Timestamps are `DATETIME(3)`.

### `eval_suites`

| Column | Type | Notes |
|--------|------|-------|
| `id` | `VARCHAR(36)` | PK |
| `workflow_id` | `VARCHAR(36)` | Application-layer ref to `workflows.id` |
| `name` | `VARCHAR(255)` | Required |
| `description` | `TEXT` | Nullable |
| `pass_threshold` | `DECIMAL(4,3)` | 0.000–1.000, default 1.000 |
| `max_concurrency` | `TINYINT UNSIGNED` | 1–N, default 1 |
| `workflow_deleted` | `TINYINT(1)` | 0/1, updated when linked workflow is deleted |
| `created_at` | `DATETIME(3)` | |
| `updated_at` | `DATETIME(3)` | |

Index: `idx_es_workflow_id (workflow_id)`

### `eval_test_cases`

| Column | Type | Notes |
|--------|------|-------|
| `id` | `VARCHAR(36)` | PK |
| `suite_id` | `VARCHAR(36)` | Ref to `eval_suites.id` |
| `name` | `VARCHAR(255)` | Required |
| `description` | `TEXT` | Nullable |
| `position` | `INT UNSIGNED` | Ordering within suite |
| `initial_data` | `JSON` | `{}` valid |
| `mocks` | `JSON` | Array of `{node_id, output}` objects |
| `graders` | `JSON` | Array of grader definition objects (see shape below) |
| `created_at` | `DATETIME(3)` | |
| `updated_at` | `DATETIME(3)` | |

Index: `idx_etc_suite_id (suite_id, position)`

Grader JSON shape stored in the `graders` column:

```json
[
  {
    "id": "<uuid>",
    "name": "LLM output quality",
    "type": "llm_judge",
    "scope": "node",
    "node_id": "llm-1",
    "config": {
      "provider": "openai",
      "model": "gpt-4o",
      "api_key": "<encrypted>",
      "rubric": "Response is helpful and accurate",
      "field_path": "completion"
    }
  }
]
```

Grader configs are stored as a JSON column on the test case (same pattern as `output_parsers` on `workflow_nodes`). Sensitive `api_key` values within grader configs are encrypted by `ConfigVault` before write and decrypted on read.

### `eval_runs`

| Column | Type | Notes |
|--------|------|-------|
| `id` | `VARCHAR(36)` | PK |
| `suite_id` | `VARCHAR(36)` | Ref to `eval_suites.id` |
| `status` | `VARCHAR(20)` | `pending` / `running` / `completed` / `failed` |
| `total_cases` | `INT UNSIGNED` | Snapshot at trigger time |
| `passed_count` | `INT UNSIGNED` | Cases where pass rate ≥ threshold |
| `failed_count` | `INT UNSIGNED` | Cases where pass rate < threshold |
| `error_count` | `INT UNSIGNED` | Cases where workflow run itself failed |
| `started_at` | `DATETIME(3)` | Nullable until execution begins |
| `finished_at` | `DATETIME(3)` | Nullable until complete |
| `created_at` | `DATETIME(3)` | |

Indexes: `idx_er_suite_id (suite_id)`, `idx_er_suite_status (suite_id, status)`

### `eval_test_case_results`

| Column | Type | Notes |
|--------|------|-------|
| `id` | `VARCHAR(36)` | PK |
| `eval_run_id` | `VARCHAR(36)` | Ref to `eval_runs.id` |
| `test_case_id` | `VARCHAR(36)` | Snapshot ref to `eval_test_cases.id` |
| `test_case_name` | `VARCHAR(255)` | Snapshot of name at run time |
| `workflow_run_id` | `VARCHAR(36)` | Ref to `runs.id` |
| `workflow_run_status` | `VARCHAR(20)` | Snapshot after run completion |
| `node_outputs` | `JSON` | `{node_id → output_map}` for nodes with node-scoped graders only |
| `grader_results` | `JSON` | Array of GraderResult objects |
| `passed` | `TINYINT(1)` | 1 if pass rate ≥ suite threshold |
| `created_at` | `DATETIME(3)` | |

Indexes: `idx_etcr_eval_run_id (eval_run_id)`, `idx_etcr_workflow_run_id (workflow_run_id)`

GraderResult JSON shape (standard graders):

```json
[
  {
    "grader_id": "<uuid>",
    "grader_name": "LLM output quality",
    "grader_type": "llm_judge",
    "verdict": "pass",
    "explanation": "The response accurately answers the question and is polite.",
    "actual_value": "Hello! I can help you with that..."
  }
]
```

GraderResult JSON shape (checklist grader — includes `score` and `criteria_results`):

```json
[
  {
    "grader_id": "<uuid>",
    "grader_name": "Response completeness",
    "grader_type": "checklist",
    "verdict": "fail",
    "score": 0.6,
    "explanation": "3 of 5 criteria met",
    "criteria_results": [
      {"criterion": "Mentions the user's name",      "met": true,  "explanation": "Found 'Alice' in the response"},
      {"criterion": "Provides a step-by-step plan",  "met": true,  "explanation": "Three numbered steps present"},
      {"criterion": "Includes a timeline",           "met": false, "explanation": "No dates or durations mentioned"},
      {"criterion": "Lists required resources",      "met": true,  "explanation": "Resources section present"},
      {"criterion": "Addresses potential risks",     "met": false, "explanation": "No risk discussion found"}
    ],
    "actual_value": "Hi Alice, here is your plan..."
  }
]
```

---

## 8. Schema Migrations

Latest existing migration is `0011`. New migrations:

| Number | Name | Changes |
|--------|------|---------|
| `0012` | `create_eval_tables` | Create `eval_suites`, `eval_test_cases`, `eval_runs`, `eval_test_case_results` |

No migration needed for `runs.triggered_by` — it is already `VARCHAR(20)`.

---

## 9. Backend Package Structure

New package: `backend/internal/eval/`

```
backend/internal/eval/
├── handler.go            — HTTP handlers for EvalSuite CRUD + EvalRun endpoints
├── runner.go             — EvalRun orchestration: iterates TestCases, applies mocks, evaluates graders
├── grader.go             — Grader interface + dispatcher
└── graders/
    ├── string_match.go   — GR-01
    ├── numeric.go        — GR-02
    ├── llm_judge.go      — GR-03 (uses aiprovider.LLMClient)
    ├── json_schema.go    — GR-04 (uses github.com/santhosh-tekuri/jsonschema/v5)
    └── checklist.go      — GR-05 (uses aiprovider.LLMClient; returns score + per-criterion results)
```

New `store.Store` methods (added to the existing interface):

```go
// EvalSuites
CreateEvalSuite(ctx, EvalSuite) (EvalSuite, error)
GetEvalSuite(ctx, id string) (EvalSuite, error)
ListEvalSuites(ctx, workflowID string) ([]EvalSuiteSummary, error)
UpdateEvalSuite(ctx, EvalSuite) (EvalSuite, error)
DeleteEvalSuite(ctx, id string) error

// TestCases
CreateTestCase(ctx, TestCase) (TestCase, error)
GetTestCase(ctx, id string) (TestCase, error)
ListTestCases(ctx, suiteID string) ([]TestCase, error)
UpdateTestCase(ctx, TestCase) (TestCase, error)
DeleteTestCase(ctx, id string) error
ReorderTestCases(ctx, suiteID string, orderedIDs []string) error

// EvalRuns
CreateEvalRun(ctx, EvalRun) (EvalRun, error)
GetEvalRun(ctx, id string) (EvalRunDetail, error)
ListEvalRuns(ctx, EvalRunFilter) ([]EvalRunSummary, error)
UpdateEvalRunStatus(ctx, runID string, status EvalRunStatus, counts EvalRunCounts) error

// TestCase Results
CreateTestCaseResult(ctx, TestCaseResult) (TestCaseResult, error)
GetTestCaseResult(ctx, id string) (TestCaseResult, error)
ListTestCaseResults(ctx, evalRunID string) ([]TestCaseResult, error)
```

---

## 10. Design Decisions

The following decisions were made during requirements authoring and are reflected throughout this document.

| ID | Decision |
|----|----------|
| DD-01 | **LLM judge prompt format:** Single `rubric` field (natural language criteria). No separate `system_prompt` or `user_message_template`. Reflected in GR-03a. |
| DD-02 | **LLM judge structured output:** Instruction-following with graceful JSON parse fallback. Provider JSON mode deferred to v2. Reflected in GR-03b/GR-03d. |
| DD-03 | **`node_outputs` storage scope:** Only nodes referenced by a `scope: node` grader are captured and stored per TestCase execution. Limits storage growth. Reflected in EX-08. |
| DD-04 | **UI navigation placement:** Eval Suites surface as a tab within the per-workflow view, alongside Run History. Reflected in UI-01. |

---

## 11. Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| ENF-01 | EvalRun trigger is non-blocking — `POST /runs` returns `201` immediately; execution is async (same pattern as `WorkflowEngine.Run()`). |
| ENF-02 | LLM judge calls respect Go context cancellation (server shutdown or eval timeout). |
| ENF-03 | Sensitive grader config values (LLM `api_key`) are never returned in plain text via the API. |
| ENF-04 | EvalRun EventBus subscriptions are cleaned up immediately after each workflow run completes to avoid memory leaks. |
| ENF-05 | All DB queries use parameterised statements via `sqlx`, consistent with the existing store. |

---

## 12. Future Considerations (v2+)

| # | Item |
|---|------|
| ~~EF-01~~ | ~~Scheduled EvalRuns (cron-triggered, similar to workflow cron triggers)~~ |
| ~~EF-02~~ | ~~CI webhook trigger — call `POST /v1/eval-suites/{id}/runs` from a pipeline with a token~~ |
| ~~EF-03~~ | ~~Dataset import — generate TestCases from a CSV or JSONL file~~ |
| ~~EF-04~~ | ~~Baseline comparison — diff two EvalRun results to surface regressions~~ |
| EF-05 | Custom grader plugins via gRPC (mirrors node plugin protocol) |
| EF-06 | Eval result streaming via WebSocket for suites with many test cases |
