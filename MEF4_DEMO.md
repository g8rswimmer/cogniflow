# MEF4 Demo — Frontend Trigger UI

Tests the trigger section in the eval suite create/edit form, the one-time
webhook secret modal, the "Rotate Secret" action, and trigger badges on the
suite list and detail pages.

---

## Prerequisites

```bash
# From repo root
docker compose up --build

# In a second terminal
npm install
npm run dev  # Vite dev server on http://localhost:5173
```

Create a workflow via the UI or:

```bash
WF_ID=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{"name":"MEF4 Demo","trigger":{"kind":"manual"},"timeout_seconds":60,"nodes":[],"edges":[]}' \
  | jq -r '.id')
echo "Workflow: $WF_ID"
```

Navigate to: `http://localhost:5173/workflows/$WF_ID/eval-suites`

---

## Scenario 1 — Create a suite with no trigger (default)

1. Click **+ New Suite**
2. The Trigger section appears at the bottom of the form with **None** selected
3. Fill in Name: `"Default Suite"`, leave Trigger = None
4. Click **Create Suite**
5. Suite appears in the list with no badge

**Expected:** Row shows no Cron or Webhook badge.

---

## Scenario 2 — Create a suite with a cron trigger

1. Click **+ New Suite**
2. Select **Cron Schedule** in the Trigger section
3. A cron expression input appears; enter: `* * * * *`
4. Help text shows: _"5-field cron: minute hour day month weekday"_
5. Fill in Name: `"Nightly Suite"` → click **Create Suite**

**Expected:**
- Suite row shows a blue **Cron** badge
- Row meta shows the cron expression `* * * * *`
- Suite detail page shows the cron badge in the header and the expression
  highlighted in blue

Verify via API:
```bash
curl -s http://localhost:8080/v1/eval-suites/<suite_id> | jq '{trigger_kind, cron_expr}'
# → {"trigger_kind":"cron","cron_expr":"* * * * *"}
```

---

## Scenario 3 — Create a suite with a webhook trigger

1. Click **+ New Suite**
2. Select **CI Webhook** in the Trigger section
3. Info box appears: _"A Bearer token will be generated on save — store it safely."_
4. Fill in Name: `"CI Gate"` → click **Create Suite**

**Expected:**
- Form closes
- **Webhook Secret modal** appears immediately:
  - Amber border and warning text: _"This secret will not be shown again."_
  - Webhook URL field (copyable)
  - Bearer Token field (copyable, styled in indigo)
  - curl example (copyable)
- Copy the token and URL before clicking **Done**

After closing:
- Suite row shows a purple **Webhook** badge
- Suite detail page shows the webhook URL in the header trigger row

Verify:
```bash
# GET shows secret masked
curl -s http://localhost:8080/v1/eval-suites/<suite_id> | jq '.webhook_secret'
# → "***"

# Webhook URL is present
curl -s http://localhost:8080/v1/eval-suites/<suite_id> | jq '.webhook_url'
# → "/v1/eval-webhooks/<suite_id>"
```

---

## Scenario 4 — Validate invalid cron expression

1. Click **+ New Suite** → select **Cron Schedule**
2. Enter: `bad-expression`
3. Try to save

**Expected:** The **Create Suite** button is disabled while the cron field is
non-empty with an invalid expression.

> The Save button is disabled when trigger_kind = cron and cron_expr is empty.
> Submitting an invalid expression returns a `VALIDATION_FAILED` error from the
> backend; the error appears in the form's red error box.

---

## Scenario 5 — Edit a cron suite to change expression

1. Open the "Nightly Suite" from Scenario 2
2. Click **Edit** (on the suite detail page or list page)
3. The form opens with Cron Schedule selected and `* * * * *` pre-filled
4. Change to `0 6 * * 1` (every Monday at 6 AM)
5. Click **Update**

**Expected:**
- Suite header shows updated expression `0 6 * * 1`
- Backend scheduler re-arms with new schedule

---

## Scenario 6 — Edit a cron suite → switch to None (disarm)

1. Open a cron suite → click **Edit**
2. Change Trigger from **Cron Schedule** to **None** → click **Update**

**Expected:**
- Blue Cron badge disappears from header and list
- No cron_expr visible
- Scheduler disarmed (no more automatic runs)

---

## Scenario 7 — Edit a non-webhook suite → switch to Webhook

1. Open any suite with Trigger = None → click **Edit**
2. Select **CI Webhook** → info box shows _"A Bearer token will be generated on save."_
3. Click **Update**

**Expected:**
- Form closes
- **Webhook Secret modal** appears with the new token (one-time reveal)
- Suite header now shows purple **Webhook** badge and the webhook URL

---

## Scenario 8 — Rotate the webhook secret

1. Navigate to the detail page of a webhook-triggered suite
2. In the trigger info row below the suite metadata, click **Rotate Secret**

**Expected:**
- Button shows "Rotating…" briefly
- **Webhook Secret modal** appears with the new token
- Old token is now invalid:

```bash
# Test with old token → 401
curl -s -X POST http://localhost:8080/v1/eval-webhooks/<suite_id> \
  -H "Authorization: Bearer <OLD_TOKEN>" | jq '.error.code'
# → "UNAUTHORIZED"

# New token works → 202
curl -s -X POST http://localhost:8080/v1/eval-webhooks/<suite_id> \
  -H "Authorization: Bearer <NEW_TOKEN>" | jq 'has("eval_run_id")'
# → true
```

---

## Scenario 9 — Run history shows triggered_by badge

1. Navigate to the detail page of a webhook or cron suite
2. Expand **Run History**

**Expected:**
- Runs triggered by webhook show a purple **webhook** badge
- Runs triggered by cron show a blue **cron** badge
- Manually triggered runs show no badge (it is the default)

```bash
# Verify triggered_by in list response
curl -s "http://localhost:8080/v1/eval-suites/<suite_id>/runs" \
  | jq '[.eval_runs[] | {triggered_by, status}]'
# Webhook runs show "triggered_by":"webhook"
# Cron runs show "triggered_by":"cron"
```

---

## Scenario 10 — Webhook suite with deleted workflow

1. Create a webhook suite, copy the token
2. Delete the underlying workflow
3. The suite detail page shows the **workflow deleted** badge alongside the Webhook badge
4. Rotate Secret still works (secret is rotated but token auth to trigger fails at the run level)

---

## Verifying via DB

```bash
docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SELECT id, name, trigger_kind, trigger_config FROM eval_suites\G"

docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SELECT id, suite_id, triggered_by, status FROM eval_runs ORDER BY created_at DESC LIMIT 10\G"
```
