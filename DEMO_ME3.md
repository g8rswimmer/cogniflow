# ME3 Demo — LLM Graders (LLM-as-Judge & Checklist)

> **Milestone:** ME3 from `PROJECT_PLAN_EVAL.md`
> **What you can show:** LLM-as-judge and checklist graders are fully operational. The judge grader sends a rubric + content to an Anthropic model and returns a binary `pass`/`fail` verdict with a written explanation. The checklist grader evaluates N criteria independently in a single call and returns a partial score with per-criterion results. API keys are stored encrypted and masked in API responses.

> **Model used throughout this demo:** `claude-haiku-4-5-20251001`
> This is Anthropic's fastest, lowest-cost model — well-suited for automated eval graders where throughput matters more than maximum reasoning depth.

---

## Prerequisites

1. An Anthropic API key (`sk-ant-...`). Set it in a shell variable — do **not** commit it:
   ```bash
   ANTHROPIC_KEY="sk-ant-your-key-here"
   ```

2. `.env` has `COGNIFLOW_ENCRYPTION_KEY` set:
   ```bash
   openssl rand -base64 32
   # paste into .env as COGNIFLOW_ENCRYPTION_KEY=<value>
   ```

3. Start the stack from the repo root:
   ```bash
   docker compose up --build -d
   ```

4. Verify healthy (~15 s after startup):
   ```bash
   curl -s http://localhost:8080/health | jq .
   # → {"status":"ok"}
   ```

---

## Step 1 — Create a workflow with an Anthropic LLM node

The workflow takes a `ticket` field in its initial data, passes it to Claude, and returns the completion. All graders in this demo evaluate the LLM's response.

```bash
WF=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Customer Support Bot",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [
      {
        "id": "llm-1",
        "type_id": "llm.anthropic",
        "label": "Support LLM",
        "position": {"x": 100, "y": 100},
        "config": {
          "model": "claude-haiku-4-5-20251001",
          "system_prompt": "You are a helpful customer support agent. Respond professionally and concisely.",
          "prompt": "Customer issue: {{.initial.ticket}}"
        }
      }
    ],
    "edges": []
  }')

WF_ID=$(echo $WF | jq -r '.id')
LLM_NODE_ID="llm-1"
echo "Workflow: $WF_ID"
```

> **Note:** The `llm.anthropic` node requires an `api_key` in its config (sensitive field, encrypted at rest). For this demo the node's API key is configured via the workflow config or you can mock the LLM node and supply the api_key only in the grader configs. We use mocks here to avoid needing the key in both places.

---

## Step 2 — Create an eval suite

```bash
SUITE=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "LLM Quality Suite",
    "description": "ME3 demo — llm_judge and checklist graders",
    "pass_threshold": 0.5,
    "max_concurrency": 1
  }')

SUITE_ID=$(echo $SUITE | jq -r '.id')
echo "Suite: $SUITE_ID"
```

---

## Step 3 — Create test case: LLM-as-judge grader (binary pass/fail)

The LLM node is mocked so no real Anthropic API call is made for the workflow run itself. The **judge grader** makes its own Anthropic API call to evaluate the canned response against the rubric.

The grader is scoped to the `llm-1` node's output and evaluates the `completion` field.

```bash
TC1=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Judge — helpful billing response",
    "initial_data": {"ticket": "My last invoice is wrong."},
    "mocks": [
      {
        "node_id": "llm-1",
        "output": {
          "completion": "I am sorry to hear about the billing issue. I have reviewed your account and can see the discrepancy. I will escalate this to our billing team immediately — you should receive a corrected invoice within 24 hours. Is there anything else I can help you with?",
          "prompt_tokens": 45,
          "completion_tokens": 55
        }
      }
    ],
    "graders": [
      {
        "id": "g-judge",
        "name": "Response is professional and resolves the issue",
        "type": "llm_judge",
        "scope": "node",
        "node_id": "llm-1",
        "config": {
          "provider": "anthropic",
          "model": "claude-haiku-4-5-20251001",
          "api_key": "'"$ANTHROPIC_KEY"'",
          "rubric": "The response acknowledges the billing issue, states a concrete next step or resolution, and maintains a professional tone. It must not be dismissive or vague.",
          "field_path": "completion"
        }
      }
    ]
  }')

TC1_ID=$(echo $TC1 | jq -r '.id')
echo "Test case 1: $TC1_ID"

# Verify api_key is masked in the response
echo $TC1 | jq '.graders[0].config.api_key'
# → "***"
```

---

## Step 4 — Create test case: LLM-as-judge grader that should FAIL

A deliberately unhelpful response should not pass the rubric.

```bash
TC2=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Judge — unhelpful response should fail",
    "initial_data": {"ticket": "My account is locked."},
    "mocks": [
      {
        "node_id": "llm-1",
        "output": {
          "completion": "Please contact support.",
          "prompt_tokens": 20,
          "completion_tokens": 5
        }
      }
    ],
    "graders": [
      {
        "id": "g-judge-fail",
        "name": "Response must provide specific next steps",
        "type": "llm_judge",
        "scope": "node",
        "node_id": "llm-1",
        "config": {
          "provider": "anthropic",
          "model": "claude-haiku-4-5-20251001",
          "api_key": "'"$ANTHROPIC_KEY"'",
          "rubric": "The response must provide at least one specific next step or action the customer can take. Generic redirects like contact support without additional detail are not acceptable.",
          "field_path": "completion"
        }
      }
    ]
  }')

TC2_ID=$(echo $TC2 | jq -r '.id')
echo "Test case 2: $TC2_ID"
```

---

## Step 5 — Create test case: checklist grader (multi-criterion)

The checklist grader evaluates five independent criteria in a single LLM call. The `pass_threshold` inside the grader config (`0.6`) means at least 3 of 5 criteria must be met for a `pass` verdict.

The grader is scoped to the workflow's final output, evaluating the top-level `llm-1` output map.

```bash
TC3=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Checklist — response quality (3 of 5 required)",
    "initial_data": {"ticket": "I was double-charged this month."},
    "mocks": [
      {
        "node_id": "llm-1",
        "output": {
          "completion": "Thank you for reaching out about the double charge. I can see this on your account and I sincerely apologise for the inconvenience. Our billing team will process a full refund within 3-5 business days. You will receive a confirmation email once complete. Please do not hesitate to contact us if you have any further questions.",
          "prompt_tokens": 50,
          "completion_tokens": 70
        }
      }
    ],
    "graders": [
      {
        "id": "g-checklist",
        "name": "Customer support quality checklist",
        "type": "checklist",
        "scope": "node",
        "node_id": "llm-1",
        "config": {
          "provider": "anthropic",
          "model": "claude-haiku-4-5-20251001",
          "api_key": "'"$ANTHROPIC_KEY"'",
          "criteria": [
            "Acknowledges the specific issue (double charge)",
            "Apologises for the inconvenience",
            "States a concrete resolution or next step",
            "Provides a timeline for resolution",
            "Maintains a professional and empathetic tone"
          ],
          "pass_threshold": 0.6,
          "field_path": "completion"
        }
      }
    ]
  }')

TC3_ID=$(echo $TC3 | jq -r '.id')
echo "Test case 3: $TC3_ID"
```

---

## Step 6 — Create test case: bad API key returns error verdict, not a crash

Providing an invalid API key should produce a grader `error` verdict — the test case itself counts as `error` in the run summary, not as a crash.

```bash
TC4=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bad API key — should produce error verdict",
    "initial_data": {},
    "mocks": [
      {
        "node_id": "llm-1",
        "output": {"completion": "Some response", "prompt_tokens": 10, "completion_tokens": 5}
      }
    ],
    "graders": [
      {
        "id": "g-bad-key",
        "name": "Judge with invalid key",
        "type": "llm_judge",
        "scope": "node",
        "node_id": "llm-1",
        "config": {
          "provider": "anthropic",
          "model": "claude-haiku-4-5-20251001",
          "api_key": "sk-ant-invalid-key-for-demo",
          "rubric": "Is the response helpful?",
          "field_path": "completion"
        }
      }
    ]
  }')

TC4_ID=$(echo $TC4 | jq -r '.id')
echo "Test case 4: $TC4_ID"
```

---

## Step 7 — Trigger the eval run

Returns `201` immediately; all four test cases execute asynchronously.

```bash
EVAL_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  -H 'Content-Type: application/json' -d '{}')

EVAL_RUN_ID=$(echo $EVAL_RUN | jq -r '.id')
echo "Eval run: $EVAL_RUN_ID"
```

---

## Step 8 — Poll until completed

Each LLM judge call typically takes 2–5 s. Allow up to 60 s for all four test cases.

```bash
for i in $(seq 1 20); do
  STATUS=$(curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq -r '.status')
  echo "[$i] Status: $STATUS"
  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then break; fi
  sleep 3
done
```

---

## Step 9 — Inspect the run summary

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
#     "passed_count": 2,   ← TC1 (judge pass) + TC3 (checklist pass)
#     "failed_count": 1,   ← TC2 (judge fail — unhelpful response)
#     "error_count": 1     ← TC4 (invalid API key)
#   }
```

---

## Step 10 — Drill into the LLM judge verdicts

```bash
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq '
  .test_case_results[] | select(.test_case_name | startswith("Judge")) | {
    name: .test_case_name,
    passed: .passed,
    verdict: .grader_results[0].verdict,
    explanation: .grader_results[0].explanation
  }'
# → {
#     "name": "Judge — helpful billing response",
#     "passed": true,
#     "verdict": "pass",
#     "explanation": "The response acknowledges the billing issue and clearly escalates it..."
#   }
# → {
#     "name": "Judge — unhelpful response should fail",
#     "passed": false,
#     "verdict": "fail",
#     "explanation": "The response says 'contact support' without any specific guidance..."
#   }
```

---

## Step 11 — Inspect checklist criteria breakdown

```bash
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq '
  .test_case_results[] | select(.test_case_name | startswith("Checklist")) | {
    name: .test_case_name,
    passed: .passed,
    verdict: .grader_results[0].verdict,
    score: .grader_results[0].score,
    explanation: .grader_results[0].explanation,
    criteria: [.grader_results[0].criteria_results[] | {
      criterion,
      met,
      explanation
    }]
  }'
# → {
#     "name": "Checklist — response quality (3 of 5 required)",
#     "passed": true,
#     "verdict": "pass",
#     "score": 1.0,
#     "explanation": "5 of 5 criteria met",
#     "criteria": [
#       {"criterion":"Acknowledges the specific issue (double charge)","met":true,"explanation":"Opening line confirms the charge"},
#       {"criterion":"Apologises for the inconvenience","met":true,"explanation":"'I sincerely apologise' is present"},
#       {"criterion":"States a concrete resolution or next step","met":true,"explanation":"Refund mentioned"},
#       {"criterion":"Provides a timeline for resolution","met":true,"explanation":"3-5 business days stated"},
#       {"criterion":"Maintains a professional and empathetic tone","met":true,"explanation":"Polite and empathetic throughout"}
#     ]
#   }
```

---

## Step 12 — Verify the bad API key produces an error verdict, not a crash

```bash
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq '
  .test_case_results[] | select(.test_case_name | startswith("Bad API")) | {
    name: .test_case_name,
    verdict: .grader_results[0].verdict,
    explanation: .grader_results[0].explanation
  }'
# → {
#     "name": "Bad API key — should produce error verdict",
#     "verdict": "error",
#     "explanation": "anthropic: http 401: {\"type\":\"error\",\"error\":{\"type\":\"authentication_error\",...}}"
#   }
```

---

## Step 13 — Verify api_key is masked in GET responses

```bash
curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases/$TC1_ID \
  | jq '.graders[0].config.api_key'
# → "***"

curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases/$TC3_ID \
  | jq '.graders[0].config.api_key'
# → "***"
```

---

## What ME3 delivers

| Capability | Implementation |
|------------|----------------|
| `llm_judge` grader — binary pass/fail with explanation | `eval/graders/llm_judge.go` |
| `checklist` grader — multi-criterion partial score | `eval/graders/checklist.go` |
| `LLMFactory` — provider-keyed client lookup (openai / anthropic) | `eval/grader.go`, `api/router.go` |
| `Grader.Grade(ctx, data)` — context threaded for cancellation | All grader implementations |
| LLM judge preamble tolerance — extracts JSON from noisy completions | `extractJSON()` / `extractJSONArray()` in graders |
| `api_key` encrypted at rest, masked `"***"` in API responses | `eval/vault.go` (ME1) |
| Error verdict on bad API key — no crash, explicit explanation | `eval/graders/llm_judge.go Grade()` |
| `BuildGrader(def, factory)` — factory required for LLM graders | `eval/grader.go` |
| Server-lifetime context cancels in-flight LLM calls on shutdown | `eval/runner.go` (ME2) |

## Next: ME4

ME4 adds the browser UI for authoring eval suites, test cases, mocks, and all five grader types — including LLM judge and checklist fields.
