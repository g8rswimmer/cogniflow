# MEF3 Demo — CI Webhook Trigger

Tests that `POST /v1/eval-webhooks/{suite_id}` triggers an eval run when
the correct Bearer token is supplied, and rejects all invalid conditions.

---

## Prerequisites

```bash
# From repo root
docker compose up --build

# Create a workflow and export its ID
WF_ID=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{"name":"MEF3 Demo","trigger":{"kind":"manual"},"timeout_seconds":60,"nodes":[],"edges":[]}' \
  | jq -r '.id')
echo "Workflow: $WF_ID"
```

---

## Scenario 1 — Create a webhook-triggered suite and capture the secret

The secret is returned **only once** — on create. Store it immediately.

```bash
RESPONSE=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{"name":"CI Gate","trigger_kind":"webhook"}')

SUITE_ID=$(echo $RESPONSE | jq -r '.id')
SECRET=$(echo $RESPONSE | jq -r '.webhook_secret')
WEBHOOK_URL=$(echo $RESPONSE | jq -r '.webhook_url')

echo "Suite:  $SUITE_ID"
echo "URL:    $WEBHOOK_URL"
echo "Secret: $SECRET"
# Secret is a plain hex string — copy it, you won't see it again.
```

Add a smoke test case so the run has something to execute:

```bash
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{"name":"Smoke","initial_data":{}}' | jq '.id'
```

---

## Scenario 2 — Trigger via CI webhook (correct token)

```bash
curl -s -X POST http://localhost:8080$WEBHOOK_URL \
  -H "Authorization: Bearer $SECRET" \
  | jq '{eval_run_id}'
# → {"eval_run_id":"<uuid>"}  — 202 Accepted
```

Poll until complete and verify `triggered_by`:

```bash
RUN_ID=$(curl -s -X POST http://localhost:8080$WEBHOOK_URL \
  -H "Authorization: Bearer $SECRET" | jq -r '.eval_run_id')

sleep 3
curl -s http://localhost:8080/v1/eval-runs/$RUN_ID \
  | jq '{status, triggered_by}'
# → {"status":"completed","triggered_by":"webhook"}
```

---

## Scenario 3 — Wrong token → 401 Unauthorized

```bash
curl -s -X POST http://localhost:8080$WEBHOOK_URL \
  -H "Authorization: Bearer wrongtoken" \
  | jq '{status: .error.code}'
# → 401  {"status":"UNAUTHORIZED"}
```

---

## Scenario 4 — Missing Authorization header → 401

```bash
curl -s -X POST http://localhost:8080$WEBHOOK_URL \
  | jq '.error.code'
# → "UNAUTHORIZED"
```

---

## Scenario 5 — Suite with wrong trigger_kind → 400 INVALID_TRIGGER

```bash
# Create a manual (non-webhook) suite
MANUAL_SUITE=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{"name":"Manual Suite","trigger_kind":"none"}' | jq -r '.id')

curl -s -X POST http://localhost:8080/v1/eval-webhooks/$MANUAL_SUITE \
  -H "Authorization: Bearer anything" \
  | jq '.error.code'
# → "INVALID_TRIGGER"
```

---

## Scenario 6 — Secret rotation

Old token is immediately invalidated; new token is returned once.

```bash
ROTATE=$(curl -s -X PUT http://localhost:8080/v1/eval-suites/$SUITE_ID \
  -H 'Content-Type: application/json' \
  -d '{"rotate_webhook_secret":true}')

NEW_SECRET=$(echo $ROTATE | jq -r '.webhook_secret')
echo "New secret: $NEW_SECRET"

# Old token now rejected
curl -s -X POST http://localhost:8080$WEBHOOK_URL \
  -H "Authorization: Bearer $SECRET" \
  | jq '.error.code'
# → "UNAUTHORIZED"

# New token accepted
curl -s -X POST http://localhost:8080$WEBHOOK_URL \
  -H "Authorization: Bearer $NEW_SECRET" \
  | jq 'has("eval_run_id")'
# → true

# Update SECRET for remaining scenarios
SECRET=$NEW_SECRET
```

---

## Scenario 7 — Workflow-deleted suite → 400 WORKFLOW_DELETED

```bash
# Delete the underlying workflow
curl -s -X DELETE http://localhost:8080/v1/workflows/$WF_ID

# Attempt webhook trigger — suite is orphaned
curl -s -X POST http://localhost:8080$WEBHOOK_URL \
  -H "Authorization: Bearer $SECRET" \
  | jq '.error.code'
# → "WORKFLOW_DELETED"
```

---

## Scenario 8 — Run history confirms triggered_by

```bash
curl -s "http://localhost:8080/v1/eval-suites/$SUITE_ID/runs" \
  | jq '[.eval_runs[] | {triggered_by, status}]'
# All webhook-triggered runs show "triggered_by":"webhook"
# Any manually triggered run (from Scenario 2 polling) shows "triggered_by":"manual"
```

---

## Simulated CI pipeline script

```bash
#!/usr/bin/env bash
# ci-eval.sh — run after deploy; fail CI if eval suite doesn't pass
set -euo pipefail

API="http://localhost:8080"
SUITE_ID="<your-suite-id>"
TOKEN="<your-webhook-secret>"

RUN_ID=$(curl -sf -X POST "$API/v1/eval-webhooks/$SUITE_ID" \
  -H "Authorization: Bearer $TOKEN" | jq -r '.eval_run_id')

echo "Triggered eval run: $RUN_ID"

for i in $(seq 1 30); do
  STATUS=$(curl -sf "$API/v1/eval-runs/$RUN_ID" | jq -r '.status')
  echo "  [$i] $STATUS"
  if [[ "$STATUS" == "completed" ]]; then break; fi
  if [[ "$STATUS" == "failed" ]]; then echo "Eval run failed"; exit 1; fi
  sleep 5
done

PASSED=$(curl -sf "$API/v1/eval-runs/$RUN_ID" | jq '.passed_count')
TOTAL=$(curl -sf  "$API/v1/eval-runs/$RUN_ID" | jq '.total_cases')
echo "Result: $PASSED/$TOTAL test cases passed"
```

---

## Verifying via DB

```bash
docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SELECT id, triggered_by, status, passed_count, total_cases
      FROM eval_runs ORDER BY created_at DESC LIMIT 5\G"
# webhook-triggered runs show triggered_by = 'webhook'
```
