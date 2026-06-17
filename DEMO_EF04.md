# EF-04 Demo — Baseline Comparison Across Eval Runs

This demo shows how to diff two completed eval runs to surface regressions and improvements. All scenarios use node mocks so no external APIs are required.

## Prerequisites

```bash
docker compose up --build
```

Set a convenience variable:

```bash
BASE=http://localhost:8080/v1
```

---

## Setup — Workflow and Eval Suite

### 1. Create a workflow

```bash
WF=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "EF04 Demo Workflow",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "scorer",
        "type_id": "http.request",
        "label": "Scorer",
        "position": {"x": 0, "y": 0},
        "config": {"url": "https://httpbin.org/get", "method": "GET"}
      }
    ],
    "edges": []
  }' | jq -r '.id')

echo "Workflow: $WF"
```

### 2. Create an eval suite

```bash
SUITE=$(curl -s -X POST $BASE/workflows/$WF/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Quality Gate",
    "description": "Tracks output quality across releases",
    "pass_threshold": 1.0
  }' | jq -r '.id')

echo "Suite: $SUITE"
```

### 3. Add test cases

Each test case mocks the `scorer` node so execution is instant and deterministic.

```bash
TC1=$(curl -s -X POST $BASE/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "High score passes",
    "mocks": [
      {"node_id": "scorer", "output": {"score": 95, "label": "excellent"}}
    ],
    "graders": [
      {
        "id": "g1",
        "name": "Score threshold",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "scorer",
        "config": {"field_path": "score", "operator": ">=", "threshold": 80}
      }
    ]
  }' | jq -r '.id')

TC2=$(curl -s -X POST $BASE/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Label check passes",
    "mocks": [
      {"node_id": "scorer", "output": {"score": 72, "label": "good"}}
    ],
    "graders": [
      {
        "id": "g2",
        "name": "Label match",
        "type": "string_match",
        "scope": "node",
        "node_id": "scorer",
        "config": {"field_path": "label", "match_type": "exact", "expected_value": "good"}
      }
    ]
  }' | jq -r '.id')

TC3=$(curl -s -X POST $BASE/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Edge case — low score",
    "mocks": [
      {"node_id": "scorer", "output": {"score": 45, "label": "poor"}}
    ],
    "graders": [
      {
        "id": "g3",
        "name": "Score threshold",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "scorer",
        "config": {"field_path": "score", "operator": ">=", "threshold": 40}
      }
    ]
  }' | jq -r '.id')

echo "Cases: $TC1  $TC2  $TC3"
```

---

## Scenario 1 — Baseline Run (all pass)

Trigger the first run — this will be the **baseline**.

```bash
BASE_RUN=$(curl -s -X POST $BASE/eval-suites/$SUITE/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.id')

echo "Baseline run: $BASE_RUN"
sleep 4
curl -s $BASE/eval-runs/$BASE_RUN | jq '{status, passed_count, failed_count, error_count}'
```

**Expected:**

```json
{
  "status": "completed",
  "passed_count": 3,
  "failed_count": 0,
  "error_count": 0
}
```

---

## Scenario 2 — Head Run with a Regression

Tighten the threshold on `TC1` so it now fails, then run the suite again.

```bash
curl -s -X PUT $BASE/eval-suites/$SUITE/test-cases/$TC1 \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "High score passes",
    "mocks": [
      {"node_id": "scorer", "output": {"score": 95, "label": "excellent"}}
    ],
    "graders": [
      {
        "id": "g1",
        "name": "Score threshold",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "scorer",
        "config": {"field_path": "score", "operator": ">=", "threshold": 100}
      }
    ]
  }' | jq '.name'
```

```bash
HEAD_RUN=$(curl -s -X POST $BASE/eval-suites/$SUITE/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.id')

echo "Head run: $HEAD_RUN"
sleep 4
curl -s $BASE/eval-runs/$HEAD_RUN | jq '{status, passed_count, failed_count, error_count}'
```

**Expected:**

```json
{
  "status": "completed",
  "passed_count": 2,
  "failed_count": 1,
  "error_count": 0
}
```

### Compare head run against baseline

```bash
curl -s "$BASE/eval-runs/$HEAD_RUN/compare?baseline_run_id=$BASE_RUN" | jq '{
  regressed_count,
  improved_count,
  unchanged_count,
  cases: [.cases[] | {test_case_name, change_type, head_passed, baseline_passed}]
}'
```

**Expected:**

```json
{
  "regressed_count": 1,
  "improved_count": 0,
  "unchanged_count": 2,
  "cases": [
    {
      "test_case_name": "High score passes",
      "change_type": "regressed",
      "head_passed": false,
      "baseline_passed": true
    },
    {
      "test_case_name": "Edge case — low score",
      "change_type": "unchanged",
      "head_passed": true,
      "baseline_passed": true
    },
    {
      "test_case_name": "Label check passes",
      "change_type": "unchanged",
      "head_passed": true,
      "baseline_passed": true
    }
  ]
}
```

---

## Scenario 3 — Improvement After Fix

Relax the threshold back (simulating a fix) and run again.

```bash
curl -s -X PUT $BASE/eval-suites/$SUITE/test-cases/$TC1 \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "High score passes",
    "mocks": [
      {"node_id": "scorer", "output": {"score": 95, "label": "excellent"}}
    ],
    "graders": [
      {
        "id": "g1",
        "name": "Score threshold",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "scorer",
        "config": {"field_path": "score", "operator": ">=", "threshold": 80}
      }
    ]
  }' | jq '.name'
```

```bash
FIX_RUN=$(curl -s -X POST $BASE/eval-suites/$SUITE/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.id')

echo "Fix run: $FIX_RUN"
sleep 4
```

### Compare fix run against the regression run

```bash
curl -s "$BASE/eval-runs/$FIX_RUN/compare?baseline_run_id=$HEAD_RUN" | jq '{
  regressed_count,
  improved_count,
  unchanged_count,
  cases: [.cases[] | {test_case_name, change_type}]
}'
```

**Expected:**

```json
{
  "regressed_count": 0,
  "improved_count": 1,
  "unchanged_count": 2,
  "cases": [
    {
      "test_case_name": "High score passes",
      "change_type": "improved"
    },
    {
      "test_case_name": "Edge case — low score",
      "change_type": "unchanged"
    },
    {
      "test_case_name": "Label check passes",
      "change_type": "unchanged"
    }
  ]
}
```

---

## Scenario 4 — New and Missing Cases

Add a fourth test case **after** the baseline run, then compare to see the `new_case` classification.

```bash
TC4=$(curl -s -X POST $BASE/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Brand new case",
    "mocks": [
      {"node_id": "scorer", "output": {"score": 60, "label": "average"}}
    ],
    "graders": [
      {
        "id": "g4",
        "name": "Score threshold",
        "type": "numeric_threshold",
        "scope": "node",
        "node_id": "scorer",
        "config": {"field_path": "score", "operator": ">=", "threshold": 50}
      }
    ]
  }' | jq -r '.id')

NEW_RUN=$(curl -s -X POST $BASE/eval-suites/$SUITE/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.id')

echo "New run: $NEW_RUN"
sleep 4
```

### Compare new run against baseline (which did not have TC4)

```bash
curl -s "$BASE/eval-runs/$NEW_RUN/compare?baseline_run_id=$BASE_RUN" | jq '{
  new_case_count,
  unchanged_count,
  cases: [.cases[] | select(.change_type == "new_case") | {test_case_name, change_type, head_passed}]
}'
```

**Expected:**

```json
{
  "new_case_count": 1,
  "unchanged_count": 3,
  "cases": [
    {
      "test_case_name": "Brand new case",
      "change_type": "new_case",
      "head_passed": true
    }
  ]
}
```

---

## Validation errors

Both runs must be completed and belong to the same suite.

```bash
# Missing baseline_run_id → 400
curl -s "$BASE/eval-runs/$HEAD_RUN/compare" | jq '.error'

# Same run ID for head and baseline → 400
curl -s "$BASE/eval-runs/$HEAD_RUN/compare?baseline_run_id=$HEAD_RUN" | jq '.error'

# Run from a different suite → 400
OTHER_SUITE=$(curl -s -X POST $BASE/workflows/$WF/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{"name": "Other Suite"}' | jq -r '.id')
OTHER_RUN=$(curl -s -X POST $BASE/eval-suites/$OTHER_SUITE/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.id')
sleep 2
curl -s "$BASE/eval-runs/$HEAD_RUN/compare?baseline_run_id=$OTHER_RUN" | jq '.error'
```

All three return `"code": "VALIDATION_FAILED"` with a descriptive message.

---

## Verify in the UI

1. Open `http://localhost:3000` and navigate to any workflow.
2. Open **Eval Suites** → **Quality Gate** → click any completed run in the run history.
3. On the `EvalRunDetailPage`, a **"Compare to:"** dropdown appears (only when the run is completed and there are other completed runs to compare against).
4. Select an older run as the baseline:
   - A **delta stats banner** appears showing `+N improved`, `-N regressed`, `N unchanged`, etc.
   - Each test case row shows a colored badge: **regressed** (red), **improved** (green), **unchanged** (gray), **new** (indigo), **missing** (amber).
5. The URL updates to include `?baseline_run_id=<id>` — the comparison is deep-linkable.
6. Select **— no baseline —** to dismiss the comparison view.
