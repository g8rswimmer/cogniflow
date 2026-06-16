# MEF2 Demo — Scheduled Eval Runs

Tests that `EvalScheduler` arms cron jobs on suite create/update, disarms on delete,
reloads from the DB on server restart, and actually fires runs on schedule.

---

## Prerequisites

```bash
# From repo root — start the full stack
docker compose up --build

# In a second terminal, export a workflow ID to use throughout
WF_ID=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{"name":"MEF2 Demo","trigger":{"kind":"manual"},"timeout_seconds":60,"nodes":[],"edges":[]}' \
  | jq -r '.id')
echo "Workflow: $WF_ID"
```

---

## Scenario 1 — Create a cron suite and verify the scheduler arms it

```bash
SUITE=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{"name":"Cron Demo","trigger_kind":"cron","cron_expr":"* * * * *"}')

echo $SUITE | jq '{id, trigger_kind, cron_expr}'
# → {"trigger_kind":"cron","cron_expr":"* * * * *"}

SUITE_ID=$(echo $SUITE | jq -r '.id')
```

The backend log should show no error — the scheduler silently armed the job.

---

## Scenario 2 — Wait for a cron fire (every-minute schedule)

> The cron expression `"* * * * *"` fires at the top of each minute.
> Either wait or use Scenario 2b for a faster test.

```bash
# Wait up to 90 s for the first automatic run
sleep 90

curl -s "http://localhost:8080/v1/eval-suites/$SUITE_ID/runs" \
  | jq '.eval_runs[] | {id, triggered_by, status}'
# → triggered_by: "cron"  status: "completed"
```

### Scenario 2b — Fast fire using an every-minute test suite

For a quicker demo, add a test case (smoke only — no graders) and manually
trigger to confirm the suite is wired, then watch the cron fire at the next minute.

```bash
# Add a no-grader smoke test case
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{"name":"Smoke","initial_data":{}}'

# Manually trigger now to confirm the suite works
MANUAL_RUN=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  | jq -r '.id')
sleep 3
curl -s http://localhost:8080/v1/eval-runs/$MANUAL_RUN \
  | jq '{status, triggered_by}'
# → {"status":"completed","triggered_by":"manual"}

# Then wait for the scheduler to fire at the next minute boundary
# (or check at :00 + a few seconds)
curl -s "http://localhost:8080/v1/eval-suites/$SUITE_ID/runs" \
  | jq '[.eval_runs[] | {triggered_by, status}]'
# → [...{"triggered_by":"cron","status":"completed"}]
```

---

## Scenario 3 — Update trigger kind: cron → none (disarms the job)

```bash
curl -s -X PUT http://localhost:8080/v1/eval-suites/$SUITE_ID \
  -H 'Content-Type: application/json' \
  -d '{"trigger_kind":"none"}' | jq '{trigger_kind}'
# → {"trigger_kind":"none"}

# Wait past the next minute boundary — no new cron runs should appear
sleep 90
curl -s "http://localhost:8080/v1/eval-suites/$SUITE_ID/runs" \
  | jq '[.eval_runs[] | select(.triggered_by == "cron")] | length'
# → count stays the same as before the update
```

---

## Scenario 4 — Change cron expression (re-arms with new schedule)

```bash
# Update to hourly
curl -s -X PUT http://localhost:8080/v1/eval-suites/$SUITE_ID \
  -H 'Content-Type: application/json' \
  -d '{"trigger_kind":"cron","cron_expr":"0 * * * *"}' \
  | jq '{trigger_kind, cron_expr}'
# → {"trigger_kind":"cron","cron_expr":"0 * * * *"}
# Only one scheduler entry exists — old every-minute job is gone.
```

---

## Scenario 5 — Delete suite disarms the scheduler

```bash
curl -s -X DELETE http://localhost:8080/v1/eval-suites/$SUITE_ID
# → 204 No Content

# The suite is gone and the scheduler entry is removed.
# No runs will appear for this suite after this point.
curl -s http://localhost:8080/v1/eval-suites/$SUITE_ID | jq '.error.code'
# → "NOT_FOUND"
```

---

## Scenario 6 — Scheduler reloads on server restart

```bash
# 1. Create a new cron suite
SUITE2=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{"name":"Restart Test","trigger_kind":"cron","cron_expr":"* * * * *"}')
SUITE2_ID=$(echo $SUITE2 | jq -r '.id')

# 2. Restart the backend
docker compose restart backend

# 3. Wait for restart to complete
sleep 10
curl -s http://localhost:8080/health | jq '.status'
# → "ok"

# 4. Wait for a cron fire — schedule should have been re-armed from DB
sleep 90
curl -s "http://localhost:8080/v1/eval-suites/$SUITE2_ID/runs" \
  | jq '[.eval_runs[] | select(.triggered_by == "cron")] | length'
# → 1 (or more — confirms reload and re-arm worked)
```

Startup log should contain:
```
level=INFO msg="database connected and migrations applied"
level=INFO msg="server starting" addr=:8080
```
No `WARN eval scheduler: could not arm suite at startup` lines for valid suites.

---

## Scenario 7 — Invalid cron expression rejected at create time

(Carried over from MEF1 — still applies)

```bash
curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{"name":"Bad","trigger_kind":"cron","cron_expr":"bad-expr"}' \
  | jq '.error.code'
# → "VALIDATION_FAILED"
# The scheduler is never reached — bad expressions are rejected by the handler.
```

---

## Verifying via DB

```bash
# Confirm trigger_kind and triggered_by are persisted correctly
docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SELECT id, name, trigger_kind, trigger_config FROM eval_suites\G"

docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SELECT id, suite_id, triggered_by, status FROM eval_runs ORDER BY created_at DESC LIMIT 10\G"
```
