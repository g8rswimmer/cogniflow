# DEMO_ME4 — Eval Suite Authoring UI

This demo walks through the ME4 frontend: creating eval suites, adding test cases with
mocks and graders of every type, reordering test cases, and triggering a run.

The backend must be running with a workflow that has at least one LLM node. Use the
existing ME3 backend from the `eval-planning` branch or later.

---

## Prerequisites

```bash
# Start the full stack
docker compose up --build

# In another terminal, start the frontend dev server
cd frontend && npm run dev
# → http://localhost:3000
```

You also need:
- An existing workflow with at least one LLM node (e.g. `llm_anthropic`)
- A valid Anthropic API key in your environment (`ANTHROPIC_KEY`)

---

## Step 1 — Create a workflow with an LLM node

If you don't have one already, create a workflow via the editor or the API.

```bash
# Create a minimal workflow with one LLM node
WF=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Support Bot",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "initial_data_schema": {
      "type": "object",
      "properties": {
        "ticket": {"type": "string", "title": "Support ticket", "description": "Customer issue description"},
        "priority": {"type": "string", "title": "Priority"}
      }
    },
    "nodes": [
      {
        "id": "llm-1",
        "type_id": "llm.anthropic",
        "label": "Draft reply",
        "position": {"x": 300, "y": 200},
        "config": {
          "model": "claude-haiku-4-5-20251001",
          "system_prompt": "You are a helpful customer support agent.",
          "user_prompt": "Respond to this support ticket: {{._initial.ticket}}"
        }
      }
    ],
    "edges": []
  }' | jq -r '.id')

echo "Workflow ID: $WF"
```

---

## Step 2 — Open the Eval Suites list

1. Open **http://localhost:3000** in your browser.
2. Find **Support Bot** in the workflow list and click it to open the editor.
3. In the top navbar, click the **⚗ Evals** button (appears once the workflow is saved).
4. You land on the **Eval Suites** page — currently empty with the message "No eval suites yet."

---

## Step 3 — Create an eval suite

1. Click **+ New Suite**.
2. In the modal, fill in:
   - **Name:** `Regression Suite`
   - **Description:** `Basic quality checks for the support bot`
   - **Pass threshold:** drag to 80%
   - **Max concurrency:** 1
3. Click **Create Suite**.
4. The suite card appears in the list showing "Regression Suite".
5. Click the suite card to open **EvalSuiteDetailPage**.

You should see the header with name, threshold, and the **+ Add Test Case** button.

---

## Step 4 — Add a test case: happy path

1. Click **+ Add Test Case**.
2. The **TestCaseEditor** slide-over opens from the right.
3. Fill in:
   - **Name:** `Happy path — billing question`
   - **Description:** `Customer asks about a late order`
4. In **Initial Data**:
   - Because the workflow declares `initial_data_schema`, you see a guided form.
   - **Support ticket:** `My order #12345 is 5 days late. When will it arrive?`
   - **Priority:** `high`

---

## Step 5 — Add a node mock

1. In the **Node Mocks** section, click **+ Add Mock**.
2. From the **Node to mock** dropdown, select `llm-1 (Draft reply)`.
3. In the JSON textarea, enter:
   ```json
   {"completion": "Thank you for reaching out. I apologize for the delay with order #12345. I have escalated this to our logistics team and you will receive an update within 24 hours."}
   ```
4. This mock bypasses the real LLM call during test execution so the test is deterministic.

---

## Step 6 — Add graders

### Grader 1 — String Match

1. In the **Graders** section, click **+ Add Grader**.
2. A new grader row appears (default: String Match, Workflow scope).
3. Expand it and fill in:
   - **Grader name:** `Response acknowledges order number`
   - **Type:** String Match
   - **Scope:** Workflow
   - **Field path:** `llm-1.completion`
   - **Match type:** Contains
   - **Expected value:** `#12345`
4. The grader row collapses showing the name and type chip.

### Grader 2 — LLM Judge

1. Click **+ Add Grader** again.
2. Fill in:
   - **Grader name:** `Response is professional and helpful`
   - **Type:** LLM Judge
   - **Scope:** Node → select `llm-1 (Draft reply)`
   - **Provider:** Anthropic
   - **Model:** `claude-haiku-4-5-20251001`
   - **API key:** your Anthropic key
   - **Rubric:** `The response acknowledges the issue, apologises, provides a clear next step, and maintains a professional tone.`
   - **Field path:** `completion`

### Grader 3 — Checklist

1. Click **+ Add Grader** a third time.
2. Fill in:
   - **Grader name:** `Response completeness checklist`
   - **Type:** Checklist
   - **Scope:** Node → select `llm-1 (Draft reply)`
   - **Provider:** Anthropic
   - **Model:** `claude-haiku-4-5-20251001`
   - **API key:** your Anthropic key
   - **Criteria:** click **+ Add Criterion** three times and enter:
     - `Acknowledges the order number`
     - `Provides a resolution timeline`
     - `Maintains professional tone`
   - **Pass threshold:** `0.6`
   - **Field path:** `completion`

3. Click **Save**. The test case appears in the list with **3 graders** and **1 mock** chips.

---

## Step 7 — Add a second test case (no graders — smoke test)

1. Click **+ Add Test Case**.
2. **Name:** `Smoke — no initial data`
3. Leave Initial Data empty, no mocks, no graders.
4. Click **Save**.
5. Two test cases are now in the list: `Happy path — billing question` and `Smoke — no initial data`.

---

## Step 8 — Reorder test cases

1. In the test case list, click **▼** on the first row to move it below the smoke test.
2. The order immediately reflects: `Smoke — no initial data` is now first.
3. Click **▲** to move it back.

> Note: The project plan called for drag-to-reorder with @dnd-kit. ME4 uses up/down buttons
> instead because @dnd-kit is not in the current dependency set. Drag-to-reorder can be
> added in a follow-up without changing the backend API.

---

## Step 9 — Validation errors surface correctly

### Invalid regex

1. Click **+ Add Test Case**, name it `Validation test`.
2. Add a grader: **Type:** String Match, **Match type:** Regex, **Expected value:** `(broken`
3. Click **Save**.
4. The server returns `VALIDATION_FAILED`. The error banner appears at the top of the editor
   listing the field and message. The slide-over stays open.
5. Fix the regex to `broken` and save again — it succeeds.

### Invalid mock node ID (API-level test)

The node dropdown only shows actual workflow nodes so a bad node ID cannot be entered
through the UI. Attempt it via the API to verify server-side rejection:

```bash
SUITE=$(curl -s http://localhost:8080/v1/workflows/$WF/eval-suites | jq -r '.eval_suites[0].id')

curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bad mock",
    "initial_data": {},
    "mocks": [{"node_id": "does-not-exist", "output": {}}],
    "graders": []
  }' | jq '.error.code'
# → "VALIDATION_FAILED"
```

---

## Step 10 — Edit an existing test case

1. Hover over the **Happy path** row and click **Edit**.
2. The slide-over opens pre-filled with all values.
3. The LLM Judge **API key** field shows `***` (masked).
4. Update the rubric to: `The response acknowledges the issue and provides a concrete next step.`
5. Click **Save**. The updated test case appears in the list.

---

## Step 11 — Trigger a run (requires backend ME3)

1. Click **▶ Run Suite** in the suite header.
2. The UI triggers a new EvalRun and navigates to `/eval-runs/:run_id`.
3. The run summary shows status **running** with a polling indicator.
4. After the workflow executes (and mocked nodes return instantly), the results page shows:
   - Total: 2 cases
   - Passed/Failed counts depend on the LLM judge verdict
5. Expand any test case row to see per-grader verdicts with explanations.

> The full results drill-down (per-grader explanations, checklist breakdown, "View Run"
> link) is implemented as part of ME4's EvalRunDetailPage (basic version). The richer
> interactive view is ME5.

---

## Step 12 — API key masking verification

```bash
TC_ID=$(curl -s http://localhost:8080/v1/eval-suites/$SUITE/test-cases | jq -r '.test_cases[0].id')

curl -s http://localhost:8080/v1/eval-suites/$SUITE/test-cases/$TC_ID \
  | jq '.graders[] | select(.type == "llm_judge") | .config.api_key'
# → "***"
```

---

## Step 13 — Delete suite

```bash
# Via UI: hover the suite card on the list page and click Delete, confirm the prompt.
# Or via API:
curl -s -X DELETE http://localhost:8080/v1/eval-suites/$SUITE
# → 204 No Content

curl http://localhost:8080/v1/eval-suites/$SUITE | jq '.error.code'
# → "NOT_FOUND"
```
