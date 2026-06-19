# EF-06 Demo: Eval Result Streaming via WebSocket

This document walks through how to verify that EvalRun results stream live to the browser via WebSocket instead of polling.

---

## What changed

| Before | After |
|--------|-------|
| `EvalRunDetailPage` polled `GET /v1/eval-runs/{id}` every 2 s while running | Page subscribes to `GET /v1/eval-runs/{id}/events` (WebSocket) |
| "Polling every 2s…" status indicator | "Live · N/M complete" green pulse indicator |
| Results appeared all-at-once after the run finished | Each test case result appears as soon as its workflow run + graders finish |
| Summary counts (passed/failed/error) stayed at zero until completion | Counts update progressively as each result streams in |

---

## Architecture

```
EvalRunner.runAsync()
   └── goroutine per TestCase
         ├── bus.Publish(eval.test_case.started)     ← fires when TC goroutine begins
         ├── executeTestCase()                        ← workflow run + graders
         ├── store.CreateTestCaseResult()
         └── bus.Publish(eval.test_case.completed)   ← fires immediately after persist
   └── (after all TCs done)
         └── bus.Publish(eval.run.completed)         ← summary with final counts

EvalEventBus (same fan-out pattern as engine.EventBus)

Handler.StreamEvalRunEvents — GET /v1/eval-runs/{id}/events
   ├── Terminal run fast path: fetch from DB, burst all results + terminal event, close
   └── Live run: subscribe → upgrade → stream → close on terminal event

useEvalRunEvents (frontend hook)
   ├── eval.test_case.completed → upsert into liveResults state
   └── eval.run.completed       → set isTerminal, close socket

EvalRunDetailPage
   ├── uses liveResults when non-empty (streaming)
   └── falls back to run.test_case_results (REST, for already-terminal pages)
```

---

## Manual test: live streaming

### Prerequisites

- Stack running: `docker-compose up --build`
- A workflow that makes an HTTP call (e.g., `GET https://httpbin.org/get`)
- An EvalSuite with at least 3 test cases on that workflow, each with a string-match grader

### Steps

**1. Trigger the run and open DevTools before results appear**

```bash
# In one terminal — trigger a run and capture the ID
SUITE_ID="<your-suite-id>"
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs | jq .
# → {"id": "<eval-run-id>"}
```

Open the EvalRun detail page in the browser:
```
http://localhost:3000/eval-runs/<eval-run-id>
```

**2. Observe the streaming indicator**

In the page header you should see:
```
Eval Run  [running]  Live · 0/3 complete   ← green pulse
```

**3. Watch results stream in**

Open DevTools → Network → WS → select the `/v1/eval-runs/.../events` connection.

As each test case finishes, you'll see frames like:

```json
{
  "eval_run_id": "...",
  "type": "eval.test_case.completed",
  "timestamp": "2026-06-19T12:00:01Z",
  "test_case_name": "Happy path",
  "result": {
    "id": "...",
    "test_case_name": "Happy path",
    "workflow_run_status": "succeeded",
    "passed": true,
    "grader_results": [{ "verdict": "pass", ... }]
  }
}
```

The result row appears in the table immediately after the frame arrives, without waiting for all test cases.

**4. Observe the terminal frame**

After all test cases finish:

```json
{
  "eval_run_id": "...",
  "type": "eval.run.completed",
  "timestamp": "2026-06-19T12:00:05Z",
  "summary": {
    "total_cases": 3,
    "passed_count": 2,
    "failed_count": 1,
    "error_count": 0
  }
}
```

The WebSocket closes, the header badge changes to `[completed]`, and the summary counts update.

---

## Manual test: fast path for completed runs

When navigating to an EvalRun that already finished (status = `completed`):

**1.** Open DevTools → Network → WS before loading the page.

**2.** Navigate to:
```
http://localhost:3000/eval-runs/<completed-eval-run-id>
```

**3.** In the WS frames panel you'll see a burst of `eval.test_case.completed` frames (one per stored result) followed immediately by `eval.run.completed`, then the connection closes.

**4.** The page renders with all results visible — no REST polling occurs.

---

## Raw WebSocket test (wscat)

```bash
npm install -g wscat   # if not installed

# Live run (trigger first, then connect immediately):
EVAL_RUN_ID="<id>"
wscat -c "ws://localhost:8080/v1/eval-runs/$EVAL_RUN_ID/events"

# Expected output (one frame per event):
# < {"eval_run_id":"...","type":"eval.test_case.started","timestamp":"...","test_case_name":"TC 1"}
# < {"eval_run_id":"...","type":"eval.test_case.completed","timestamp":"...","result":{...}}
# < {"eval_run_id":"...","type":"eval.test_case.started","timestamp":"...","test_case_name":"TC 2"}
# < {"eval_run_id":"...","type":"eval.test_case.completed","timestamp":"...","result":{...}}
# < {"eval_run_id":"...","type":"eval.run.completed","timestamp":"...","summary":{...}}
# (connection closed by server)
```

---

## Event reference

| Type | When emitted | Payload fields |
|------|-------------|----------------|
| `eval.test_case.started` | When the test case goroutine begins (before the workflow run) | `test_case_name` |
| `eval.test_case.completed` | After `executeTestCase()` + `CreateTestCaseResult()` | `test_case_name`, `result` (full `TestCaseResult`) |
| `eval.run.completed` | After all test cases finish and final counts are persisted | `summary` (`total_cases`, `passed_count`, `failed_count`, `error_count`) |
| `eval.run.failed` | If `runAsync` exits via the failed status path | `summary` |

---

## Concurrency note

When `max_concurrency > 1` on the EvalSuite, multiple test cases execute in parallel. `eval.test_case.completed` events arrive in completion order (not definition order). The frontend upserts by `test_case_id` so late-arriving events from reconnects don't create duplicate rows.
