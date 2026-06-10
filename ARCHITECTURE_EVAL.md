# cogniflow — Workflow Evaluation Architecture

> **Status:** Draft v0.1
> **Last Updated:** 2026-06-10
> **Implements:** REQUIREMENTS_EVAL.md v0.3
> **Depends on:** ARCHITECTURE.md v0.3

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Package Structure](#2-package-structure)
3. [Core Go Types & Interfaces](#3-core-go-types--interfaces)
4. [Eval Execution Engine](#4-eval-execution-engine)
5. [Grader System](#5-grader-system)
6. [Node Mocking Design](#6-node-mocking-design)
7. [API Key Encryption for Graders](#7-api-key-encryption-for-graders)
8. [Frontend Component Structure](#8-frontend-component-structure)
9. [MySQL Schema](#9-mysql-schema)
10. [REST API Contract](#10-rest-api-contract)
11. [Implementation Sequencing](#11-implementation-sequencing)

---

## 1. System Overview

The eval feature adds a new `eval` package to the Go backend and new pages/components to the React frontend. It integrates with three existing subsystems — the execution engine (to trigger workflow runs), the EventBus (to capture per-node outputs), and the store (to persist suites, test cases, and results) — without restructuring any of them.

### Component Diagram

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                          Browser (React SPA)                                  │
│                                                                               │
│  ┌────────────────────┐  ┌─────────────────────────────────────────────────┐ │
│  │  (existing)        │  │  Eval UI (new)                                  │ │
│  │  WorkflowCanvas    │  │  EvalSuiteListPage  EvalSuiteDetailPage         │ │
│  │  RunHistoryPage    │  │  TestCaseEditor     EvalRunDetailPage            │ │
│  │  …                 │  │  GraderEditor       ChecklistResultDetail        │ │
│  └────────────────────┘  └─────────────────────────────────────────────────┘ │
│                              REST (fetch)                                     │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                         Go Backend (single binary)                            │
│                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────┐    │
│  │                       api/router.go  (chi router)                    │    │
│  │   /eval-suites  /eval-runs  /workflows/:id/eval-suites               │    │
│  └────────────────────────────┬─────────────────────────────────────────┘    │
│                               │                                               │
│                               ▼                                               │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │                     eval (new package)                                 │  │
│  │                                                                        │  │
│  │  ┌─────────────┐   ┌──────────────┐   ┌──────────────────────────┐   │  │
│  │  │  handler.go │   │  runner.go   │   │  grader.go + graders/    │   │  │
│  │  │  (HTTP)     │──▶│  (EvalRun    │──▶│  (string_match, numeric, │   │  │
│  │  │             │   │   orchestr.) │   │   llm_judge, json_schema,│   │  │
│  │  └─────────────┘   └──────┬───────┘   │   checklist)             │   │  │
│  │                           │           └──────────────────────────┘   │  │
│  └───────────────────────────┼────────────────────────────────────────────┘  │
│                              │                                                │
│         ┌────────────────────┼──────────────────────┐                        │
│         │                   │                       │                        │
│         ▼                   ▼                       ▼                        │
│  ┌─────────────┐   ┌────────────────┐   ┌───────────────────────────┐        │
│  │  store      │   │  engine        │   │  aiprovider               │        │
│  │  (eval      │   │  WorkflowEngine│   │  LLMClient                │        │
│  │   tables)   │   │  EventBus      │   │  (judge + checklist calls) │        │
│  └─────────────┘   └────────────────┘   └───────────────────────────┘        │
└──────────────────────────────────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────┐
│  MySQL 9.0+      │
│  eval_suites     │
│  eval_test_cases │
│  eval_runs       │
│  eval_test_case_ │
│    results       │
└──────────────────┘
```

### Integration Points with Existing Code

| Existing Component | How Eval Uses It |
|-------------------|------------------|
| `engine.WorkflowEngine` | Calls `Run()` once per TestCase, passing `NodeMocks` in `trigger.RunRequest` |
| `engine.EventBus` | Subscribes via `bus.Subscribe(runID)` to capture per-node outputs from `node.succeeded` events |
| `trigger.RunRequest` | Extended with `NodeMocks map[string]map[string]any`; nil for non-eval runs (zero-change for existing callers) |
| `aiprovider.LLMClient` | Used by `llm_judge` and `checklist` graders to call the judge LLM |
| `crypto.Cipher` | Used by eval handler to encrypt/decrypt `api_key` in LLM grader configs before DB read/write |
| `store.Store` | Extended with eval CRUD methods (see §3) |
| `api/router.go` | `NewRouter` receives `evalHandler` deps and registers eval routes |

---

## 2. Package Structure

### New Backend Files

```
backend/internal/
└── eval/
    ├── handler.go          — HTTP handlers for all eval endpoints (struct + methods pattern)
    ├── runner.go           — EvalRunner: orchestrates EvalRun execution asynchronously
    ├── grader.go           — Grader interface, GraderResult/CriterionResult types, BuildGrader()
    ├── vault.go            — GraderVault: encrypts/decrypts api_key in grader configs using crypto.Cipher
    └── graders/
        ├── string_match.go — GR-01: exact / contains / regex string assertion
        ├── numeric.go      — GR-02: field_path + operator + threshold comparison
        ├── llm_judge.go    — GR-03: single-rubric LLM binary verdict
        ├── json_schema.go  — GR-04: JSON Schema draft-07 validation
        └── checklist.go    — GR-05: multi-criterion LLM partial scoring
```

### Modified Backend Files

| File | Change |
|------|--------|
| `backend/internal/trigger/types.go` | Add `NodeMocks map[string]map[string]any` to `RunRequest` |
| `backend/internal/engine/runner.go` | Check `req.NodeMocks` in `executeNode` before calling registry (see §6) |
| `backend/internal/store/store.go` | Add eval CRUD methods to `Store` interface |
| `backend/internal/store/mysql/eval_store.go` | New file — MySQL implementation of eval store methods |
| `backend/internal/store/mysql/migrations/0012_create_eval_tables.up.sql` | New migration |
| `backend/internal/store/mysql/migrations/0012_create_eval_tables.down.sql` | New migration |
| `backend/internal/api/router.go` | Register eval routes; pass additional deps to `NewRouter` |

### New Frontend Files

```
frontend/src/
├── pages/
│   ├── EvalSuiteListPage.tsx   — /workflows/:id/evals  (tab on workflow detail)
│   ├── EvalSuiteDetailPage.tsx — /eval-suites/:suite_id
│   └── EvalRunDetailPage.tsx   — /eval-runs/:run_id
├── components/eval/
│   ├── EvalSuiteCard.tsx       — summary card with last run badge
│   ├── EvalSuiteForm.tsx       — create/edit name, description, pass_threshold
│   ├── TestCaseList.tsx        — ordered list with drag-to-reorder
│   ├── TestCaseEditor.tsx      — full form: initial data + MockList + GraderList
│   ├── MockEditor.tsx          — node selector + JSON output textarea
│   ├── GraderList.tsx          — ordered list of grader rows
│   ├── GraderEditor.tsx        — type-switched form (renders one of the five sub-forms)
│   ├── GraderTypeFields.tsx    — per-type field groups (StringMatch / Numeric / LLMJudge / Schema / Checklist)
│   ├── EvalRunHistory.tsx      — table of past EvalRuns with summary badges
│   ├── EvalRunResultsTable.tsx — expandable per-TestCase rows
│   ├── GraderResultRow.tsx     — verdict badge + explanation + actual_value
│   └── ChecklistResultDetail.tsx — per-criterion breakdown table
├── stores/
│   └── useEvalStore.ts         — suite list, active suite, active test case, active run
└── types/
    └── eval.ts                 — EvalSuite, TestCase, NodeMock, GraderDef, EvalRun, TestCaseResult, GraderResult types
```

---

## 3. Core Go Types & Interfaces

### `trigger/types.go` — RunRequest extension

```go
// RunRequest is extended with NodeMocks for eval runs.
// All existing callers pass nil (zero value); no behaviour change.
type RunRequest struct {
    WorkflowID  string
    InitialData map[string]any
    TriggeredBy string  // "manual" | "webhook" | "cron" | "eval"

    // NodeMocks maps node instance ID → canned NodeOutput.Data.
    // When non-nil and a node ID is present, the engine skips Execute()
    // and returns the mock output directly. Set only by eval.EvalRunner.
    NodeMocks map[string]map[string]any
}
```

### `eval/grader.go` — Grader interface and result types

```go
package eval

import "context"

// GraderVerdict is the outcome of a single grader evaluation.
type GraderVerdict string

const (
    VerdictPass  GraderVerdict = "pass"
    VerdictFail  GraderVerdict = "fail"
    VerdictError GraderVerdict = "error"
)

// CriterionResult is the per-item outcome for GR-05 Checklist grader.
type CriterionResult struct {
    Criterion   string `json:"criterion"`
    Met         bool   `json:"met"`
    Explanation string `json:"explanation"`
}

// GraderResult is the outcome of one Grader evaluation within a TestCase.
type GraderResult struct {
    GraderID        string            `json:"grader_id"`
    GraderName      string            `json:"grader_name"`
    GraderType      string            `json:"grader_type"`
    Verdict         GraderVerdict     `json:"verdict"`
    Score           *float64          `json:"score,omitempty"`            // checklist only
    Explanation     string            `json:"explanation"`
    ActualValue     any               `json:"actual_value,omitempty"`
    CriteriaResults []CriterionResult `json:"criteria_results,omitempty"` // checklist only
}

// Grader evaluates a resolved output map and returns a GraderResult.
// target is the map[string]any to assert against (workflow final output or a node's output).
// The Grader does not need to know which scope it was resolved from.
type Grader interface {
    Type() string
    Evaluate(ctx context.Context, graderID, graderName string, target map[string]any) GraderResult
}

// GraderDef is the persisted definition stored in eval_test_cases.graders JSON column.
type GraderDef struct {
    ID     string         `json:"id"`
    Name   string         `json:"name"`
    Type   string         `json:"type"`             // "string_match" | "numeric_threshold" | "llm_judge" | "json_schema" | "checklist"
    Scope  string         `json:"scope"`            // "workflow" | "node"
    NodeID string         `json:"node_id,omitempty"`
    Config map[string]any `json:"config"`
}

// BuildGrader constructs the appropriate Grader from a GraderDef.
// Decrypted configs are passed in (vault decryption happens before this call).
func BuildGrader(def GraderDef, llmFactory func(provider string) (aiprovider.LLMClient, error)) (Grader, error)
```

### `store/store.go` — Eval types and new Store methods

```go
// --- Eval data types ---

type EvalSuite struct {
    ID              string    `db:"id"`
    WorkflowID      string    `db:"workflow_id"`
    Name            string    `db:"name"`
    Description     string    `db:"description"`
    PassThreshold   float64   `db:"pass_threshold"`
    MaxConcurrency  int       `db:"max_concurrency"`
    WorkflowDeleted bool      `db:"workflow_deleted"`
    CreatedAt       time.Time `db:"created_at"`
    UpdatedAt       time.Time `db:"updated_at"`
}

type EvalSuiteSummary struct {
    EvalSuite
    TestCaseCount int        `db:"test_case_count"`
    LastRunStatus *string    `db:"last_run_status"`
    LastRunAt     *time.Time `db:"last_run_at"`
}

type TestCase struct {
    ID          string         `db:"id"`
    SuiteID     string         `db:"suite_id"`
    Name        string         `db:"name"`
    Description string         `db:"description"`
    Position    int            `db:"position"`
    InitialData map[string]any // stored as JSON
    Mocks       []NodeMock     // stored as JSON
    Graders     []GraderDef    // stored as JSON; api_key values are encrypted ciphertext
    CreatedAt   time.Time      `db:"created_at"`
    UpdatedAt   time.Time      `db:"updated_at"`
}

type NodeMock struct {
    NodeID string         `json:"node_id"`
    Output map[string]any `json:"output"`
}

type EvalRun struct {
    ID          string        `db:"id"`
    SuiteID     string        `db:"suite_id"`
    Status      EvalRunStatus `db:"status"`
    TotalCases  int           `db:"total_cases"`
    PassedCount int           `db:"passed_count"`
    FailedCount int           `db:"failed_count"`
    ErrorCount  int           `db:"error_count"`
    StartedAt   *time.Time    `db:"started_at"`
    FinishedAt  *time.Time    `db:"finished_at"`
    CreatedAt   time.Time     `db:"created_at"`
}

type EvalRunStatus string

const (
    EvalRunPending   EvalRunStatus = "pending"
    EvalRunRunning   EvalRunStatus = "running"
    EvalRunCompleted EvalRunStatus = "completed"
    EvalRunFailed    EvalRunStatus = "failed"
)

type EvalRunFilter struct {
    SuiteID string
    Status  EvalRunStatus
    Limit   int
    Offset  int
}

type EvalRunCounts struct {
    PassedCount int
    FailedCount int
    ErrorCount  int
}

type TestCaseResult struct {
    ID                string                       `db:"id"`
    EvalRunID         string                       `db:"eval_run_id"`
    TestCaseID        string                       `db:"test_case_id"`
    TestCaseName      string                       `db:"test_case_name"`
    WorkflowRunID     string                       `db:"workflow_run_id"`
    WorkflowRunStatus string                       `db:"workflow_run_status"`
    NodeOutputs       map[string]map[string]any    // stored as JSON
    GraderResults     []eval.GraderResult          // stored as JSON
    Passed            bool                         `db:"passed"`
    CreatedAt         time.Time                    `db:"created_at"`
}

// --- New Store interface methods ---
// (appended to the existing Store interface in store/store.go)

// EvalSuites
CreateEvalSuite(ctx context.Context, s EvalSuite) (EvalSuite, error)
GetEvalSuite(ctx context.Context, id string) (EvalSuite, error)
ListEvalSuites(ctx context.Context, workflowID string) ([]EvalSuiteSummary, error)
UpdateEvalSuite(ctx context.Context, s EvalSuite) (EvalSuite, error)
DeleteEvalSuite(ctx context.Context, id string) error          // cascades at app layer

// TestCases
CreateTestCase(ctx context.Context, tc TestCase) (TestCase, error)
GetTestCase(ctx context.Context, id string) (TestCase, error)
ListTestCases(ctx context.Context, suiteID string) ([]TestCase, error)
UpdateTestCase(ctx context.Context, tc TestCase) (TestCase, error)
DeleteTestCase(ctx context.Context, id string) error
ReorderTestCases(ctx context.Context, suiteID string, orderedIDs []string) error

// EvalRuns
CreateEvalRun(ctx context.Context, r EvalRun) (EvalRun, error)
GetEvalRun(ctx context.Context, id string) (EvalRun, error)
ListEvalRuns(ctx context.Context, f EvalRunFilter) ([]EvalRun, error)
UpdateEvalRunStatus(ctx context.Context, runID string, status EvalRunStatus, counts EvalRunCounts) error

// TestCase Results
CreateTestCaseResult(ctx context.Context, r TestCaseResult) (TestCaseResult, error)
GetTestCaseResult(ctx context.Context, id string) (TestCaseResult, error)
ListTestCaseResults(ctx context.Context, evalRunID string) ([]TestCaseResult, error)
```

### `eval/handler.go` — Handler struct

```go
// evalHandler follows the same struct+methods pattern as all other handlers.
type evalHandler struct {
    store     store.Store
    engine    *engine.WorkflowEngine
    bus       *engine.EventBus
    vault     *GraderVault           // encrypts/decrypts grader api_key fields
    llmFactory func(provider string) (aiprovider.LLMClient, error)
}
```

---

## 4. Eval Execution Engine

### EvalRunner

`EvalRunner` is the central coordinator for an EvalRun. It is constructed from the same dependencies as `evalHandler` and is invoked in a background goroutine.

```go
// eval/runner.go

type EvalRunner struct {
    store      store.Store
    engine     *engine.WorkflowEngine
    bus        *engine.EventBus
    vault      *GraderVault
    llmFactory func(provider string) (aiprovider.LLMClient, error)
}

// Execute runs all TestCases in the suite and persists results.
// Called as `go runner.Execute(ctx, suiteID, evalRunID)`.
func (r *EvalRunner) Execute(ctx context.Context, suiteID, evalRunID string)
```

### Top-Level Orchestration

```
POST /v1/eval-suites/{suite_id}/runs
  │
  ├── store.CreateEvalRun({status: "pending", total_cases: len(testCases)})
  ├── go runner.Execute(ctx, suiteID, evalRunID)   ← non-blocking
  └── 201 Created {"eval_run_id": "..."}

runner.Execute():
  1. store.GetEvalSuite(suiteID)
  2. store.ListTestCases(suiteID)
  3. store.UpdateEvalRunStatus(evalRunID, "running", EvalRunCounts{})

  4. sem := make(chan struct{}, suite.MaxConcurrency)
     wg  := sync.WaitGroup{}

  5. for each testCase in testCases (in position order):
       sem ← struct{}{}
       wg.Add(1)
       go func(tc store.TestCase) {
           defer func() { <-sem; wg.Done() }()
           result, verdict := r.executeTestCase(ctx, tc, suite)
           store.CreateTestCaseResult(result)
           r.incrementCounts(evalRunID, verdict)  // atomic update
       }(testCase)

  6. wg.Wait()
  7. store.UpdateEvalRunStatus(evalRunID, "completed", finalCounts)
```

### Per-TestCase Execution

```
runner.executeTestCase(ctx, tc, suite):
  1. Build mock map from tc.Mocks:
       mockMap := map[string]map[string]any{}
       for _, m := range tc.Mocks:
           mockMap[m.NodeID] = m.Output

  2. Identify which node IDs need output capture:
       captureSet := set of node_ids referenced by scope:"node" graders in tc.Graders

  3. Subscribe to EventBus:
       eventCh, unsubscribe := bus.Subscribe("")    // will filter by runID after step 4
       defer unsubscribe()
       capturedOutputs := map[string]map[string]any{}

  4. Trigger workflow run:
       handle, err := engine.Run(ctx, trigger.RunRequest{
           WorkflowID:  suite.WorkflowID,
           InitialData: tc.InitialData,
           TriggeredBy: "eval",
           NodeMocks:   mockMap,
       })

  5. Drain EventBus subscription for this run:
       for event := range eventCh:
           if event.RunID != handle.RunID: continue
           if event.Type == "node.succeeded" && captureSet.has(event.NodeID):
               capturedOutputs[event.NodeID] = event.Output
           if event.Type == "run.succeeded" || event.Type == "run.failed":
               break

  6. Fetch final output (for workflow-scoped graders):
       run, _ := store.GetRun(ctx, handle.RunID)

  7. Decrypt grader api_key fields:
       graderDefs := vault.DecryptGraders(tc.Graders)

  8. Evaluate each grader:
       graderResults := []eval.GraderResult{}
       for _, def := range graderDefs:
           target := resolveTarget(def, run.FinalOutput, capturedOutputs)
           if target == nil:
               graderResults = append(graderResults, errorResult(def, "node did not execute"))
               continue
           grader, _ := BuildGrader(def, llmFactory)
           result := grader.Evaluate(ctx, def.ID, def.Name, target)
           graderResults = append(graderResults, result)

  9. Compute pass rate and verdict:
       passRate := float64(passCount) / float64(len(graderDefs))
       if len(graderDefs) == 0 && run.Status == "succeeded": passRate = 1.0
       passed := passRate >= suite.PassThreshold

  10. Return store.TestCaseResult{
          WorkflowRunID:     handle.RunID,
          WorkflowRunStatus: run.Status,
          NodeOutputs:       capturedOutputs,   // only captureSet entries
          GraderResults:     graderResults,
          Passed:            passed,
      }
```

### resolveTarget

```go
// resolveTarget returns the map to evaluate grader assertions against.
// Returns nil if a node-scoped grader's target node did not produce output.
func resolveTarget(def store.GraderDef, finalOutput map[string]any, captured map[string]map[string]any) map[string]any {
    switch def.Scope {
    case "workflow":
        return finalOutput
    case "node":
        out, ok := captured[def.NodeID]
        if !ok {
            return nil  // node did not execute
        }
        return out
    }
    return nil
}
```

---

## 5. Grader System

### Grader Interface Implementation Pattern

Each grader is a small, stateless struct in `eval/graders/`. All graders follow the same signature:

```go
type <Type>Grader struct {
    // parsed config fields
}

func New<Type>Grader(config map[string]any) (*<Type>Grader, error)  // validates config at construction

func (g *<Type>Grader) Type() string { return "<type_string>" }

func (g *<Type>Grader) Evaluate(ctx context.Context, graderID, graderName string, target map[string]any) eval.GraderResult
```

### GR-01 String Match (`graders/string_match.go`)

```go
type StringMatchGrader struct {
    FieldPath     string  // gjson dot-path
    MatchType     string  // "exact" | "contains" | "regex"
    ExpectedValue string
    compiled      *regexp.Regexp  // non-nil when match_type == "regex"
}
```

**Evaluate logic:**
1. Extract value via `gjson.Get(jsonTarget, g.FieldPath)` — if result is invalid → `error` verdict "field not found"
2. Coerce to string via `fmt.Sprintf("%v", val)`
3. Apply match: exact equality / `strings.Contains` / `regexp.MatchString`
4. Return `pass` or `fail`

### GR-02 Numeric Threshold (`graders/numeric.go`)

```go
type NumericGrader struct {
    FieldPath string
    Operator  string   // "==" | "!=" | ">" | ">=" | "<" | "<="
    Threshold float64
}
```

**Evaluate logic:**
1. Extract via gjson; if not a JSON number → `error` verdict "field is not numeric"
2. Compare `result.Float() <op> g.Threshold`
3. Return `pass` or `fail`

### GR-03 LLM-as-Judge (`graders/llm_judge.go`)

```go
type LLMJudgeGrader struct {
    Client    aiprovider.LLMClient
    Model     string
    APIKey    string   // decrypted at construction time
    Rubric    string
    FieldPath string   // empty = use full target JSON
}
```

**Judge prompt construction:**
```
System: You are an evaluator. Assess whether the following output meets the criteria below.
        Respond ONLY with valid JSON in this exact format: {"verdict":"pass","explanation":"..."}
        or {"verdict":"fail","explanation":"..."}

Criteria: {rubric}

Output to evaluate:
{target_content}
```

**Evaluate logic:**
1. Resolve `target_content`: if `FieldPath` is set, use gjson extraction; otherwise marshal full target to JSON
2. Call `client.Complete(ctx, LLMRequest{APIKey, Model, SystemMsg: judgeSystemPrompt, Prompt: targetContent, MaxTokens: 512})`
3. Attempt JSON unmarshal of `completion` into `{verdict, explanation}`
4. If unmarshal fails → `error` verdict "judge response could not be parsed"
5. If LLM call fails → `error` verdict with provider error message
6. Return `GraderResult{Verdict: parsed.verdict, Explanation: parsed.explanation, ActualValue: targetContent}`

### GR-04 JSON Schema Validation (`graders/json_schema.go`)

```go
type JSONSchemaGrader struct {
    FieldPath string
    Schema    *jsonschema.Schema  // compiled at construction using santhosh-tekuri/jsonschema/v5
}
```

**Evaluate logic:**
1. Resolve target value (field path or full map)
2. Marshal to JSON bytes, then `schema.Validate(jsonschema.NewStringInstance(jsonBytes))`
3. If valid → `pass`; if invalid → `fail` with validation errors concatenated in `explanation`

### GR-05 Checklist (`graders/checklist.go`)

```go
type ChecklistGrader struct {
    Client        aiprovider.LLMClient
    Model         string
    APIKey        string
    Criteria      []string
    PassThreshold float64   // grader-level threshold (independent of suite threshold)
    FieldPath     string
}
```

**Checklist prompt construction:**
```
System: You are an evaluator. For each criterion listed, determine if it is met by the output.
        Respond ONLY with a JSON array, one object per criterion, in this exact format:
        [{"criterion":"...","met":true,"explanation":"..."},...]

Criteria:
1. {criteria[0]}
2. {criteria[1]}
...

Output to evaluate:
{target_content}
```

**Evaluate logic:**
1. Resolve `target_content` (same as GR-03)
2. Call `client.Complete(...)` — MaxTokens sized to `len(criteria) * 100 + 200` (capped at 2048)
3. Attempt JSON unmarshal of completion into `[]CriterionResult`
4. If unmarshal fails → `error` verdict "checklist response could not be parsed"
5. Count `metCount`; compute `score = float64(metCount) / float64(len(criteria))`
6. `verdict = pass` if `score >= g.PassThreshold`, else `fail`
7. Return `GraderResult{Verdict, Score: &score, Explanation: "N of M criteria met", CriteriaResults: parsed}`

---

## 6. Node Mocking Design

### Mechanism

The `trigger.RunRequest` is extended with a `NodeMocks` field. Inside `engine/runner.go`, `executeNode()` checks this field before calling the node registry:

```go
// engine/runner.go — executeNode() (modified section only)

func (r *runner) executeNode(ctx context.Context, node store.WorkflowNode, req trigger.RunRequest, ec *ExecutionContext) {
    // --- NEW: check for eval mock ---
    if req.NodeMocks != nil {
        if mockOutput, ok := req.NodeMocks[node.ID]; ok {
            augmented := make(map[string]any, len(mockOutput)+1)
            for k, v := range mockOutput {
                augmented[k] = v
            }
            augmented["mocked"] = true
            ec.Set(node.ID, augmented)
            r.bus.Publish(engine.NodeEvent{
                RunID:     req.RunID,
                NodeID:    node.ID,
                Type:      engine.EventNodeSucceeded,
                Timestamp: time.Now(),
                Output:    augmented,
            })
            return
        }
    }
    // --- existing execution path unchanged below ---
    handler, err := r.registry.Lookup(node.TypeID)
    // ...
}
```

**Key properties of this design:**
- All existing callers of `engine.Run()` pass `NodeMocks: nil` implicitly (zero value of map) — no behaviour change
- Mock interception logic is 10 lines; the rest of the engine is untouched
- Mocked nodes still emit `node.succeeded` events (with `"mocked": true` in output) so the EventBus subscriber in the eval runner can capture them normally
- Output parsers are **not** applied to mock outputs (mock output is stored directly in ExecutionContext without going through `outputparser.Apply()`)

### Mock Validation (save time)

At `POST/PUT /eval-suites/{suite_id}/test-cases`, the handler validates all mock `node_id` values:

```go
// eval/handler.go — validateMocks()

func validateMocks(mocks []store.NodeMock, workflowNodes []store.WorkflowNode) []FieldValidationError {
    nodeIDs := set(workflowNodes, func(n store.WorkflowNode) string { return n.ID })
    var errs []FieldValidationError
    for i, m := range mocks {
        if !nodeIDs.has(m.NodeID) {
            errs = append(errs, FieldValidationError{
                Field:   fmt.Sprintf("mocks[%d].node_id", i),
                Message: fmt.Sprintf("node ID %q not found in workflow", m.NodeID),
            })
        }
    }
    return errs
}
```

---

## 7. API Key Encryption for Graders

### Problem

The `api_key` field in `llm_judge` and `checklist` grader configs is sensitive and must be stored encrypted. These configs live in the `graders` JSON column on `eval_test_cases`, not in `node_configs`, so the existing `ConfigVault` does not handle them.

### Solution — GraderVault (`eval/vault.go`)

```go
// GraderVault encrypts and decrypts api_key fields in grader configs.
// It uses the same crypto.Cipher as ConfigVault but operates on GraderDef slices.
type GraderVault struct {
    cipher *crypto.Cipher
}

func NewGraderVault(cipher *crypto.Cipher) *GraderVault

// EncryptGraders returns a copy of the grader slice with api_key values encrypted.
// Called before writing to the DB.
func (v *GraderVault) EncryptGraders(graders []GraderDef) ([]GraderDef, error)

// DecryptGraders returns a copy with api_key values decrypted.
// Called after reading from the DB, before passing to runner or returning via API.
func (v *GraderVault) DecryptGraders(graders []GraderDef) ([]GraderDef, error)

// MaskGraders returns a copy with api_key values replaced by "***".
// Called before serialising grader configs in API responses.
func (v *GraderVault) MaskGraders(graders []GraderDef) []GraderDef
```

**Sensitive grader types:** `llm_judge` and `checklist` (both have `api_key` in config). `string_match`, `numeric_threshold`, and `json_schema` have no sensitive fields.

**Encryption is the handler's responsibility:** `evalHandler` calls `vault.EncryptGraders` before calling any store write method, and `vault.DecryptGraders` after any store read. The store itself stores and retrieves the raw JSON — it does not perform encryption.

This mirrors the existing pattern where `ConfigVault` wraps the store, but here the eval handler performs the encryption inline (keeping `GraderVault` a thin helper, not a store wrapper).

---

## 8. Frontend Component Structure

### Route / Page Structure

```
/workflows/:id/evals              → EvalSuiteListPage  (new tab on workflow detail)
/eval-suites/:suite_id            → EvalSuiteDetailPage
/eval-suites/:suite_id/test-cases/:case_id/edit → TestCaseEditor (modal or page)
/eval-runs/:run_id                → EvalRunDetailPage
```

### Component Tree

```
App
├── WorkflowEditorPage  (existing)
│   └── [new] Eval Suites tab
│       └── EvalSuiteListPage
│           ├── EvalSuiteCard[]  (name, test case count, last run badge, run timestamp)
│           └── [Create Suite button → EvalSuiteForm modal]
│
├── EvalSuiteDetailPage
│   ├── SuiteHeader  (name, description, pass_threshold display, edit button, Run Suite button)
│   ├── TestCaseList  (ordered; drag-to-reorder via @dnd-kit or React Flow DnD)
│   │   └── TestCaseRow[]  (name, grader count, last verdict badge, edit/delete actions)
│   └── EvalRunHistory  (collapsible panel at bottom)
│       └── EvalRunRow[]  (short ID, started_at, duration, status, "N/M passed" badge)
│
├── TestCaseEditor  (slide-over panel or dedicated page)
│   ├── name + description inputs
│   ├── InitialDataSection
│   │   └── RJSF form (if workflow.initial_data_schema) or JSON textarea (fallback)
│   ├── MockList
│   │   └── MockEditor[]
│   │       ├── NodeSelector  (dropdown of workflow nodes by label)
│   │       └── JSON textarea  (mock output)
│   └── GraderList
│       └── GraderEditor[]
│           ├── name input
│           ├── type dropdown  (String Match / Numeric / LLM Judge / JSON Schema / Checklist)
│           ├── scope toggle   (Workflow / Node)
│           ├── NodeSelector   (shown when scope=Node)
│           └── GraderTypeFields  (type-switched sub-form)
│               ├── StringMatchFields   (field_path, match_type, expected_value)
│               ├── NumericFields       (field_path, operator, threshold)
│               ├── LLMJudgeFields      (provider, model, api_key password, rubric textarea, field_path)
│               ├── JSONSchemaFields    (field_path, schema JSON textarea)
│               └── ChecklistFields     (criteria list editor, pass_threshold, provider/model/api_key/field_path)
│
└── EvalRunDetailPage
    ├── RunSummary  (status, started_at, duration, N/M cases passed)
    └── EvalRunResultsTable
        └── TestCaseResultRow[]  (expandable)
            ├── collapsed: test case name, workflow run status, verdict summary badge
            └── expanded:
                ├── [View Run] link → RunDetailPage
                ├── GraderResultRow[]
                │   ├── verdict badge (green pass / red fail / amber error)
                │   ├── grader type chip
                │   ├── explanation text
                │   └── actual_value (collapsible pre block)
                └── [checklist graders only] ChecklistResultDetail
                    └── criterion table (criterion text | met badge | explanation)
```

### State Management — `useEvalStore`

```ts
// stores/useEvalStore.ts (Zustand)

interface EvalStore {
    // Suite list (per workflow)
    suites: EvalSuiteSummary[]
    loadSuites: (workflowId: string) => Promise<void>

    // Active suite
    activeSuite: EvalSuite | null
    testCases: TestCase[]
    loadSuite: (suiteId: string) => Promise<void>

    // Active eval run
    activeRunId: string | null
    runResults: EvalRunDetail | null
    triggerRun: (suiteId: string) => Promise<string>   // returns evalRunId
    pollRunStatus: (evalRunId: string) => Promise<void> // polls until completed
    loadRun: (evalRunId: string) => Promise<void>
}
```

Eval results are polled (not streamed via WebSocket) — `pollRunStatus` calls `GET /v1/eval-runs/{id}` every 2 seconds until `status === "completed"` or `"failed"`.

### TypeScript Types (`types/eval.ts`)

```ts
export interface EvalSuite {
    id: string
    workflow_id: string
    name: string
    description: string
    pass_threshold: number
    max_concurrency: number
    workflow_deleted: boolean
    created_at: string
    updated_at: string
}

export interface TestCase {
    id: string
    suite_id: string
    name: string
    description: string
    position: number
    initial_data: Record<string, unknown>
    mocks: NodeMock[]
    graders: GraderDef[]
}

export interface NodeMock {
    node_id: string
    output: Record<string, unknown>
}

export interface GraderDef {
    id: string
    name: string
    type: 'string_match' | 'numeric_threshold' | 'llm_judge' | 'json_schema' | 'checklist'
    scope: 'workflow' | 'node'
    node_id?: string
    config: Record<string, unknown>
}

export interface EvalRun {
    id: string
    suite_id: string
    status: 'pending' | 'running' | 'completed' | 'failed'
    total_cases: number
    passed_count: number
    failed_count: number
    error_count: number
    started_at: string | null
    finished_at: string | null
}

export interface CriterionResult {
    criterion: string
    met: boolean
    explanation: string
}

export interface GraderResult {
    grader_id: string
    grader_name: string
    grader_type: string
    verdict: 'pass' | 'fail' | 'error'
    score?: number
    explanation: string
    actual_value?: unknown
    criteria_results?: CriterionResult[]
}

export interface TestCaseResult {
    id: string
    eval_run_id: string
    test_case_id: string
    test_case_name: string
    workflow_run_id: string
    workflow_run_status: string
    node_outputs: Record<string, Record<string, unknown>>
    grader_results: GraderResult[]
    passed: boolean
}
```

---

## 9. MySQL Schema

```sql
-- ============================================================
-- eval_suites
-- One row per named evaluation suite, linked to one workflow.
-- No FOREIGN KEY to workflows — referential integrity at app layer.
-- ============================================================
CREATE TABLE eval_suites (
    id               VARCHAR(36)        NOT NULL,
    workflow_id      VARCHAR(36)        NOT NULL,
    name             VARCHAR(255)       NOT NULL,
    description      TEXT,
    pass_threshold   DECIMAL(4,3)       NOT NULL DEFAULT 1.000,
    max_concurrency  TINYINT UNSIGNED   NOT NULL DEFAULT 1,
    workflow_deleted TINYINT(1)         NOT NULL DEFAULT 0,
    created_at       DATETIME(3)        NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       DATETIME(3)        NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_es_workflow_id (workflow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- ============================================================
-- eval_test_cases
-- One row per test case. mocks and graders are JSON blobs.
-- Sensitive api_key values within graders are AES-256-GCM
-- encrypted (same cipher as node_configs.encrypted_value).
-- ============================================================
CREATE TABLE eval_test_cases (
    id           VARCHAR(36)      NOT NULL,
    suite_id     VARCHAR(36)      NOT NULL,
    name         VARCHAR(255)     NOT NULL,
    description  TEXT,
    position     INT UNSIGNED     NOT NULL DEFAULT 0,
    initial_data JSON             NOT NULL,
    mocks        JSON             NOT NULL DEFAULT (JSON_ARRAY()),
    graders      JSON             NOT NULL DEFAULT (JSON_ARRAY()),
    created_at   DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                  ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_etc_suite_position (suite_id, position)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- ============================================================
-- eval_runs
-- One row per triggered EvalRun.
-- passed_count / failed_count / error_count are updated
-- atomically as each TestCaseResult is persisted.
-- ============================================================
CREATE TABLE eval_runs (
    id           VARCHAR(36)      NOT NULL,
    suite_id     VARCHAR(36)      NOT NULL,
    status       VARCHAR(20)      NOT NULL DEFAULT 'pending',
    total_cases  INT UNSIGNED     NOT NULL DEFAULT 0,
    passed_count INT UNSIGNED     NOT NULL DEFAULT 0,
    failed_count INT UNSIGNED     NOT NULL DEFAULT 0,
    error_count  INT UNSIGNED     NOT NULL DEFAULT 0,
    started_at   DATETIME(3),
    finished_at  DATETIME(3),
    created_at   DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_er_suite_id (suite_id),
    INDEX idx_er_suite_status (suite_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- ============================================================
-- eval_test_case_results
-- One row per TestCase execution within an EvalRun.
-- node_outputs stores only outputs for nodes referenced by a
-- scope:"node" grader (EX-08 — storage minimisation).
-- grader_results JSON includes score + criteria_results for
-- checklist graders; standard fields only for others.
-- ============================================================
CREATE TABLE eval_test_case_results (
    id                   VARCHAR(36)  NOT NULL,
    eval_run_id          VARCHAR(36)  NOT NULL,
    test_case_id         VARCHAR(36)  NOT NULL,
    test_case_name       VARCHAR(255) NOT NULL,
    workflow_run_id      VARCHAR(36)  NOT NULL,
    workflow_run_status  VARCHAR(20)  NOT NULL,
    node_outputs         JSON         NOT NULL DEFAULT (JSON_OBJECT()),
    grader_results       JSON         NOT NULL DEFAULT (JSON_ARRAY()),
    passed               TINYINT(1)   NOT NULL DEFAULT 0,
    created_at           DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_etcr_eval_run_id (eval_run_id),
    INDEX idx_etcr_workflow_run_id (workflow_run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### Application-Layer Cascade Order for DeleteEvalSuite

```
DeleteEvalSuite(ctx, suiteID):
  1. DELETE FROM eval_test_case_results WHERE eval_run_id IN (SELECT id FROM eval_runs WHERE suite_id = ?)
  2. DELETE FROM eval_runs WHERE suite_id = ?
  3. DELETE FROM eval_test_cases WHERE suite_id = ?
  4. DELETE FROM eval_suites WHERE id = ?
```

### Atomic Count Updates

`UpdateEvalRunCounts` uses a single SQL statement to prevent race conditions when `max_concurrency > 1`:

```sql
UPDATE eval_runs
SET
    passed_count = passed_count + ?,
    failed_count = failed_count + ?,
    error_count  = error_count  + ?
WHERE id = ?
```

---

## 10. REST API Contract

All eval endpoints follow the existing `/v1/` prefix, snake_case JSON, and `{"error": {"code": "...", "message": "..."}}` error shape.

### Endpoint Summary

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/workflows/{workflow_id}/eval-suites` | List suites for a workflow |
| `POST` | `/v1/workflows/{workflow_id}/eval-suites` | Create suite |
| `GET` | `/v1/eval-suites/{suite_id}` | Get suite with TestCase summaries |
| `PUT` | `/v1/eval-suites/{suite_id}` | Update suite metadata |
| `DELETE` | `/v1/eval-suites/{suite_id}` | Delete suite and all data |
| `GET` | `/v1/eval-suites/{suite_id}/test-cases` | List test cases |
| `POST` | `/v1/eval-suites/{suite_id}/test-cases` | Create test case |
| `GET` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | Get single test case |
| `PUT` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | Replace test case |
| `DELETE` | `/v1/eval-suites/{suite_id}/test-cases/{case_id}` | Delete test case |
| `PUT` | `/v1/eval-suites/{suite_id}/test-cases/order` | Reorder test cases |
| `POST` | `/v1/eval-suites/{suite_id}/runs` | Trigger EvalRun |
| `GET` | `/v1/eval-suites/{suite_id}/runs` | List EvalRuns |
| `GET` | `/v1/eval-runs/{eval_run_id}` | Get full EvalRun with results |
| `GET` | `/v1/eval-runs/{eval_run_id}/test-case-results/{result_id}` | Get single TestCaseResult |

### Request / Response Examples

**`POST /v1/workflows/{id}/eval-suites`** — Request:

```json
{
  "name": "Customer Support Quality Suite",
  "description": "Validates LLM output quality for support tickets",
  "pass_threshold": 0.8,
  "max_concurrency": 1
}
```

Response (`201 Created`):

```json
{
  "id": "es-uuid-...",
  "workflow_id": "wf-uuid-...",
  "name": "Customer Support Quality Suite",
  "description": "Validates LLM output quality for support tickets",
  "pass_threshold": 0.8,
  "max_concurrency": 1,
  "workflow_deleted": false,
  "created_at": "2026-06-10T12:00:00.000Z",
  "updated_at": "2026-06-10T12:00:00.000Z"
}
```

---

**`POST /v1/eval-suites/{suite_id}/test-cases`** — Request:

```json
{
  "name": "Billing question — happy path",
  "description": "Customer asks about a charge",
  "initial_data": { "ticket_text": "Why was I charged $49 last month?" },
  "mocks": [
    {
      "node_id": "http-node-1",
      "output": { "status_code": 200, "body": "{\"account_status\":\"active\"}" }
    }
  ],
  "graders": [
    {
      "id": "gr-uuid-1",
      "name": "Response mentions billing",
      "type": "string_match",
      "scope": "node",
      "node_id": "llm-node-1",
      "config": {
        "field_path": "completion",
        "match_type": "contains",
        "expected_value": "charge"
      }
    },
    {
      "id": "gr-uuid-2",
      "name": "Response quality check",
      "type": "llm_judge",
      "scope": "node",
      "node_id": "llm-node-1",
      "config": {
        "provider": "openai",
        "model": "gpt-4o",
        "api_key": "sk-...",
        "rubric": "The response is polite, addresses the billing question, and offers next steps",
        "field_path": "completion"
      }
    }
  ]
}
```

Response (`201 Created`): same shape as request body, with `id`, `suite_id`, `position`, `created_at`, `updated_at` added; `api_key` values masked as `"***"`.

---

**`POST /v1/eval-suites/{suite_id}/runs`** — Request: `{}` (empty body)

Response (`201 Created`):

```json
{
  "id": "er-uuid-...",
  "suite_id": "es-uuid-...",
  "status": "pending",
  "total_cases": 3,
  "passed_count": 0,
  "failed_count": 0,
  "error_count": 0,
  "started_at": null,
  "finished_at": null,
  "created_at": "2026-06-10T12:05:00.000Z"
}
```

---

**`GET /v1/eval-runs/{eval_run_id}`** — Response (`200 OK`, after completion):

```json
{
  "id": "er-uuid-...",
  "suite_id": "es-uuid-...",
  "status": "completed",
  "total_cases": 3,
  "passed_count": 2,
  "failed_count": 1,
  "error_count": 0,
  "started_at": "2026-06-10T12:05:01.000Z",
  "finished_at": "2026-06-10T12:05:28.412Z",
  "test_case_results": [
    {
      "id": "tcr-uuid-...",
      "test_case_id": "tc-uuid-...",
      "test_case_name": "Billing question — happy path",
      "workflow_run_id": "run-uuid-...",
      "workflow_run_status": "succeeded",
      "passed": true,
      "grader_results": [
        {
          "grader_id": "gr-uuid-1",
          "grader_name": "Response mentions billing",
          "grader_type": "string_match",
          "verdict": "pass",
          "explanation": "",
          "actual_value": "I can see a $49 charge on your account..."
        },
        {
          "grader_id": "gr-uuid-2",
          "grader_name": "Response quality check",
          "grader_type": "llm_judge",
          "verdict": "pass",
          "explanation": "The response is polite, directly addresses the charge, and suggests contacting support.",
          "actual_value": "I can see a $49 charge..."
        }
      ]
    }
  ]
}
```

### Validation Error Format

Save-time validation errors for test cases follow the existing `VALIDATION_FAILED` structure:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Test case validation failed: 2 error(s)",
    "details": {
      "validation_errors": [
        { "field": "mocks[0].node_id",        "message": "node ID \"node-xyz\" not found in workflow" },
        { "field": "graders[1].config.pattern","message": "invalid regex: error parsing regexp: missing closing ): `(foo`" }
      ]
    }
  }
}
```

### Router Changes (`api/router.go`)

`NewRouter` receives two additional parameters and registers the eval routes:

```go
func NewRouter(
    db *sqlx.DB,
    st store.Store,
    registry *node.NodeRegistry,
    dispatcher trigger.Dispatcher,
    bus *engine.EventBus,
    eng *engine.WorkflowEngine,     // NEW — needed by eval runner
    cipher *crypto.Cipher,          // NEW — needed by GraderVault
    tm *trigger.Manager,
    level *slog.LevelVar,
) http.Handler {
    // ...existing handler setup...

    vault := eval.NewGraderVault(cipher)
    llmFactory := func(provider string) (aiprovider.LLMClient, error) { /* construct from provider string */ }
    runner := eval.NewEvalRunner(st, eng, bus, vault, llmFactory)
    eh := &eval.Handler{Store: st, Runner: runner, Vault: vault}

    mux.HandleFunc("GET /v1/workflows/{workflow_id}/eval-suites",                          eh.ListByWorkflow)
    mux.HandleFunc("POST /v1/workflows/{workflow_id}/eval-suites",                         eh.Create)
    mux.HandleFunc("GET /v1/eval-suites/{suite_id}",                                       eh.Get)
    mux.HandleFunc("PUT /v1/eval-suites/{suite_id}",                                       eh.Update)
    mux.HandleFunc("DELETE /v1/eval-suites/{suite_id}",                                    eh.Delete)
    mux.HandleFunc("GET /v1/eval-suites/{suite_id}/test-cases",                            eh.ListCases)
    mux.HandleFunc("POST /v1/eval-suites/{suite_id}/test-cases",                           eh.CreateCase)
    mux.HandleFunc("GET /v1/eval-suites/{suite_id}/test-cases/{case_id}",                  eh.GetCase)
    mux.HandleFunc("PUT /v1/eval-suites/{suite_id}/test-cases/{case_id}",                  eh.UpdateCase)
    mux.HandleFunc("DELETE /v1/eval-suites/{suite_id}/test-cases/{case_id}",               eh.DeleteCase)
    mux.HandleFunc("PUT /v1/eval-suites/{suite_id}/test-cases/order",                      eh.ReorderCases)
    mux.HandleFunc("POST /v1/eval-suites/{suite_id}/runs",                                 eh.TriggerRun)
    mux.HandleFunc("GET /v1/eval-suites/{suite_id}/runs",                                  eh.ListRuns)
    mux.HandleFunc("GET /v1/eval-runs/{eval_run_id}",                                      eh.GetRun)
    mux.HandleFunc("GET /v1/eval-runs/{eval_run_id}/test-case-results/{result_id}",        eh.GetTestCaseResult)
}
```

---

## 11. Implementation Sequencing

Build order respects inter-package dependencies:

| Step | Work | Notes |
|------|------|-------|
| 1 | **Migration `0012_create_eval_tables`** | DB schema first; all other steps depend on it |
| 2 | **`trigger/types.go`** — add `NodeMocks` field to `RunRequest` | Zero-change for existing callers (nil map) |
| 3 | **`engine/runner.go`** — mock interception in `executeNode` | 10-line addition; write unit test with mock nodes |
| 4 | **`store/store.go`** — add eval types + interface methods | Keeps store the single source of truth |
| 5 | **`store/mysql/eval_store.go`** — MySQL implementation | Can use SQLite in-memory for unit tests (same pattern as existing store tests) |
| 6 | **`eval/vault.go`** — `GraderVault` encrypt/decrypt/mask | Depends only on `crypto.Cipher` |
| 7 | **`eval/graders/string_match.go`** + **`numeric.go`** + **`json_schema.go`** | No external deps; easy to unit test in isolation |
| 8 | **`eval/graders/llm_judge.go`** + **`checklist.go`** | Depend on `aiprovider.LLMClient`; mock the client in tests |
| 9 | **`eval/grader.go`** — `BuildGrader()` dispatcher + `GraderResult` types | Wires all graders together |
| 10 | **`eval/runner.go`** — `EvalRunner.Execute()` + `executeTestCase()` | Integration test: runs a workflow with mocks, checks captured outputs |
| 11 | **`eval/handler.go`** — HTTP handlers + validation | Depends on store and runner |
| 12 | **`api/router.go`** — register eval routes, add deps to `NewRouter` | Final wiring |
| 13 | **Frontend** — types, store, API client, components, pages | Can begin against a mock API; integrate last |
