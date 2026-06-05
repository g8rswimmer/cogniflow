# Demo: IT Support Ticket Triage

A grounded end-to-end walkthrough of cogniflow. You will build a workflow that receives a support ticket, uses Claude (Anthropic) to classify urgency, branches into an escalation or standard-reply path, and merges the results — entirely configured through the browser UI.

---

## What you will build

```
Initial Data: { "ticket": "..." }
          │
          ▼
  ┌─────────────────┐
  │  Classify Ticket │  ← llm.anthropic
  │  (parse JSON)    │
  └────────┬────────┘
           │  output parser: urgent (boolean)
           ▼
  ┌─────────────────┐
  │  Urgency Check   │  ← conditional (CEL)
  └────────┬────────┘
     true ╱ ╲ false
          ╱   ╲
  ┌──────────┐ ┌──────────────┐
  │ Escalate │ │ Standard     │  ← llm.anthropic (one per branch)
  │  Ticket  │ │    Reply     │
  └────┬─────┘ └──────┬───────┘
       │              │
       └──────┬───────┘
              ▼
         ┌────────┐
         │  Merge │  ← merge
         └────────┘
```

**What this shows:**

- Multi-node DAG with a fan-out/fan-in pattern
- Anthropic LLM node with a structured JSON prompt
- Output parser extracting a typed field from the completion
- CEL conditional branching on that extracted value
- Template variables wiring upstream output into downstream prompts
- Live WebSocket run events and post-run detail view

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| Docker + Docker Compose | For running MySQL + backend |
| Go 1.22+ | Only needed if running backend locally without Docker |
| Node 20+ | For the Vite dev server |
| Anthropic API key | `sk-ant-...` — free tier is fine; Haiku is used |
| `.env` file at repo root | Copy from `.env.example`; set `COGNIFLOW_ENCRYPTION_KEY` |

### Environment variables (`.env`)

```
DB_DSN=cogniflow:cogniflow_pass@tcp(localhost:3306)/cogniflow?parseTime=true
COGNIFLOW_ENCRYPTION_KEY=<32 random bytes, base64-encoded>
PORT=8080
```

Generate a key:
```bash
openssl rand -base64 32
```

---

## Step 1 — Start the stack

```bash
# Terminal 1: database + backend
docker compose up --build backend mysql

# Terminal 2: frontend dev server
cd frontend
npm install
npm run dev
```

Wait until the backend logs `server listening :8080` and the Vite output shows `ready in ...ms`.

Verify:
```bash
curl http://localhost:8080/v1/node-types | jq '[.node_types[].type_id]'
# Should list http.request, llm.anthropic, conditional, merge, etc.
```

---

## Step 2 — Open the app

Navigate to **http://localhost:3000**. You should see the Workflow List page (empty on first run).

Click **+ New Workflow**.

---

## Step 3 — Name the workflow and set the trigger

1. In the top bar, click the workflow name field and type:
   `IT Support Ticket Triage`
2. Click the **▶ Manual** button (trigger settings).
3. Leave it on **Manual** — you will provide the ticket text when you run.
4. Click **Close**.

---

## Step 4 — Place the six nodes

The left palette shows all registered node types. Use the search box to find each one. **Drag** cards from the palette onto the canvas.

Arrange them roughly as shown — vertical spacing helps visualise the flow:

```
[Classify Ticket]                        ← centre-top
        |
[Urgency Check]                          ← centre, below Classify
       / \
[Escalate] [Standard Reply]              ← left and right of the conditional
       \ /
     [Merge]                             ← centre-bottom
```

**Nodes to place (in order):**

| Palette search | Label to set after placing |
|----------------|---------------------------|
| `llm.anthropic` | `Classify Ticket` |
| `conditional` | `Urgency Check` |
| `llm.anthropic` | `Escalate Ticket` |
| `llm.anthropic` | `Standard Reply` |
| `merge` | `Merge` |

**Rename a node:** click it to select, then edit the label field at the top of the right-hand Config Sidebar.

> **Tip:** The canvas supports scroll-to-zoom and drag-to-pan. Use the MiniMap (bottom-right) to orient yourself.

---

## Step 5 — Configure each node

Click a node to open its Config Sidebar on the right.

---

### 5a · Classify Ticket (first `llm.anthropic` node)

| Field | Value |
|-------|-------|
| API Key | Your `sk-ant-...` key |
| Model | `claude-haiku-4-5-20251001` |
| Max Tokens | `256` |
| Temperature | `0` (deterministic JSON output) |
| System Message | `You are a support ticket classifier. Always respond with a single valid JSON object and no other text.` |
| Prompt | See below |

**Prompt:**
```
Classify the following IT support ticket. Respond with this exact JSON structure and nothing else:
{"urgent": true_or_false, "category": "hardware|software|network|account|other", "summary": "one sentence describing the issue"}

Set urgent to true only if the issue is blocking the user from working right now.

Ticket:
{{._initial.ticket}}
```

> `{{._initial.ticket}}` is a template variable referencing the `ticket` key from the initial data you will provide at run time. Click the **Variables** section below the Prompt field to see available upstream fields.

---

### 5b · Add an Output Parser to Classify Ticket

Still on the **Classify Ticket** config sidebar, scroll down to the **Output Parsers** section.

Click **+ Add Extractor** and fill in:

| Field | Value |
|-------|-------|
| Name | `urgent` |
| Source field | `completion` |
| Type | `json_path` |
| Pattern | `urgent` |

Click **Add**.

Add a second extractor:

| Field | Value |
|-------|-------|
| Name | `summary` |
| Source field | `completion` |
| Type | `json_path` |
| Pattern | `summary` |

Click **Add**.

> These parsers run after the LLM completes. They read the `completion` text (which the model returns as JSON), extract the `urgent` boolean and `summary` string, and merge them into the node's output map — making them available to downstream nodes as `{{.CLASSIFY_ID.urgent}}` and `{{.CLASSIFY_ID.summary}}`.

---

### 5c · Find the Classify Ticket node ID

The Conditional node's CEL expression and the LLM prompts downstream need to reference the **Classify Ticket** node by its internal ID (not its label).

**How to find it:**

1. Click the **Urgency Check** (conditional) node to select it.
2. In the right-hand Config Sidebar, scroll down below the Expression field.
3. You will see an **↑ Upstream Nodes** section that lists every node connected upstream.
4. Find the **Classify Ticket** row. It shows:
   - The node label (`Classify Ticket`)
   - The raw node ID in small monospace text (e.g. `llm.anthropic-1748976543210`)
   - A `copy id` button
   - Clickable field chips: `completion`, `urgent`, `summary`
5. Click **copy id** next to Classify Ticket to copy the node ID to your clipboard.

> The field chips (e.g. clicking `urgent`) copy the full CEL reference `ctx["llm.anthropic-1748976543210"]["urgent"]` directly — paste it into the Expression field.

---

### 5d · Urgency Check (conditional node)

Click **Urgency Check** to select it.

| Field | Value |
|-------|-------|
| Expression | `ctx["CLASSIFY_NODE_ID"]["urgent"] == true` |

Replace `CLASSIFY_NODE_ID` with the ID you copied in 5c. Example:
```
ctx["llm.anthropic-1748976543210"]["urgent"] == true
```

> This is a CEL expression. `ctx` is the merged upstream data map, keyed by node ID. The output parser placed `urgent` (a boolean) into the Classify node's output, so this comparison is type-safe.
>
> The engine routes the **true** edge to Escalate Ticket and the **false** edge to Standard Reply.

---

### 5e · Escalate Ticket (second `llm.anthropic` node)

| Field | Value |
|-------|-------|
| API Key | Same Anthropic key |
| Model | `claude-haiku-4-5-20251001` |
| Max Tokens | `400` |
| Temperature | `0.7` |
| System Message | `You are a senior IT support agent handling urgent escalations. Be empathetic and action-oriented.` |
| Prompt | See below |

**Prompt:**
```
A user has an URGENT IT support issue that is blocking them right now.

Issue summary: {{.CLASSIFY_NODE_ID.summary}}

Write a short, professional email response (3–4 sentences) that:
1. Acknowledges the urgency
2. Commits to immediate follow-up within 30 minutes
3. Asks for their phone number or best contact method
```

Replace `CLASSIFY_NODE_ID` with the same ID from step 5c.

> **How to insert the template variable without typing:** click the Prompt field to focus it, then click `CLASSIFY_NODE_ID → summary` in the Variables section below. It will insert `{{.CLASSIFY_NODE_ID.summary}}` at the cursor.

---

### 5f · Standard Reply (third `llm.anthropic` node)

| Field | Value |
|-------|-------|
| API Key | Same Anthropic key |
| Model | `claude-haiku-4-5-20251001` |
| Max Tokens | `400` |
| Temperature | `0.7` |
| System Message | `You are a friendly IT support agent. Be helpful and set clear expectations.` |
| Prompt | See below |

**Prompt:**
```
A user has submitted a routine IT support ticket.

Issue summary: {{.CLASSIFY_NODE_ID.summary}}

Write a professional acknowledgment email (3–4 sentences) that:
1. Confirms receipt of their request
2. Sets an expectation of 4–8 business hours for resolution
3. Offers a self-service suggestion if applicable (e.g. restart, clear cache)
```

Replace `CLASSIFY_NODE_ID` as before.

---

### 5g · Merge node

No configuration needed. The Merge node is a passthrough that waits for all its upstream predecessors to finish before proceeding. Since only one branch executes (the conditional routes to exactly one), Merge will receive the output of whichever LLM ran.

---

## Step 6 — Draw the edges

Click and drag from a node's **bottom handle** (source) to another node's **top handle** (target).

Draw these connections in order:

| From | To | Branch label |
|------|----|-------------|
| Classify Ticket | Urgency Check | *(none)* |
| Urgency Check | Escalate Ticket | `true` |
| Urgency Check | Standard Reply | `false` |
| Escalate Ticket | Merge | *(none)* |
| Standard Reply | Merge | *(none)* |

**Setting branch labels on Conditional edges:**

After drawing the edge from Urgency Check → Escalate Ticket, double-click the edge label area and type `true`. Repeat for Urgency Check → Standard Reply with `false`.

> Branch labels tell the engine which outgoing edge to follow based on the conditional's result. Edges without labels on a conditional node are ignored.

---

## Step 7 — Save

Click **Save** in the top-right of the navbar.

The URL will update from `/workflows/new` to `/workflows/<uuid>`. The "unsaved" indicator disappears.

If you see a validation error:
- `VALIDATION_FAILED` on the expression → check the CEL syntax and node ID in step 5d
- `VALIDATION_FAILED` on a template → check the `{{. ... }}` syntax in your prompts

---

## Step 8 — Run with a ticket

Click the green **▶ Run** button in the navbar.

A **RunStatusPanel** slides up from the bottom of the canvas. It shows each node with a grey dot (pending).

You will be asked for initial data. Paste this JSON into the initial data field:

```json
{
  "ticket": "My laptop screen has gone completely black and I cannot see anything. I have a board-level presentation in 90 minutes and nothing I try is working."
}
```

Click **Run**.

> This scenario should classify as `urgent: true` and route through the Escalate Ticket branch.

---

## Step 9 — Watch it live

As the run executes you will see the node status update in real time:

| Stage | What you see |
|-------|-------------|
| Run starts | All nodes: grey dot, `pending` |
| Classify Ticket executing | Amber pulsing dot, `running` |
| Classify Ticket done | Green dot, `succeeded` — click the row to expand and read the raw JSON completion + extracted `urgent` and `summary` fields |
| Urgency Check | Flips to `running` then `succeeded` instantly (CEL eval is fast) |
| Escalate Ticket | Amber pulsing, then `succeeded` — the email draft appears in the expanded output |
| Standard Reply | Stays grey — it never ran (conditional routed to the other branch) |
| Merge | `succeeded` immediately |
| Panel header | Changes to **"Run succeeded"** in green |

Click **View details** in the panel header to open the RunDetailPage.

---

## Step 10 — Explore run history and detail

### Run History (`/workflows/<id>/runs`)

Click **← Workflows** then the **Runs** button on your workflow card. You will see:

- A row per run with status badge, timestamp, triggered-by, and elapsed time
- Click any row to open RunDetailPage

### Run Detail (`/runs/<run_id>`)

- **Graph snapshot** at the top: nodes that produced output show in green; others in grey (unknown — the API only surfaces sink-node output in `final_output`)
- **Node Results** list below: expand `Merge` to see the final merged output, which contains the escalation email draft from the Escalate Ticket node

---

## Try a different ticket

Go back to the editor (`← Back to Editor`) and click **Run** again with a low-urgency ticket:

```json
{
  "ticket": "Hi, could you update my email signature template? No rush, whenever you get a chance."
}
```

This should classify as `urgent: false`, route through Standard Reply, and produce a polite acknowledgment email instead.

---

## Observations and UX notes

These are things worth noticing as you use the system — useful for evaluating what to build next.

### What works well

- **Schema-driven forms:** dragging a new node type onto the canvas immediately shows the correct config fields — no per-node form code needed. `x-sensitive` fields mask the API key automatically.
- **Template variable picker:** clicking upstream output fields inserts the snippet at the cursor. No memorising node IDs.
- **Output parsers:** extracting typed fields from an LLM completion and making them available downstream is a clean pattern that avoids a dedicated "JSON parse" node.
- **Live events:** watching each node flip from amber to green during execution makes the DAG scheduling visible.
- **CEL branching:** the conditional node correctly skips the Standard Reply branch entirely — skipped nodes never appear in the run panel's output.

### Current friction points

| Area | Observation |
|------|-------------|
| **Node IDs in CEL** | The conditional expression requires the raw auto-generated node ID (e.g. `llm.anthropic-1748976543210`). Users must fish this out from the template variable picker. A named-reference system or node aliases would eliminate this. |
| **Branch labels on edges** | Typing `true`/`false` on conditional edges is not discoverable — a dropdown on the edge would be clearer. |
| **Run detail graph** | Nodes without output (skipped branch, non-sink nodes) are all grey — the detail page cannot distinguish "ran and succeeded" from "never ran" without per-node status persisted in the DB. |
| **No initial-data modal** | The Run button fires with empty `{}` unless the user edits code. A small "Initial data" dialog on the Run button would make parameterised runs self-service. |
| **CEL expression validation feedback** | Save-time CEL errors return `VALIDATION_FAILED` but the UI shows the error in the navbar — it would be more helpful to highlight the specific Conditional node's field inline. |
| **No retry-on-failure UI** | Retry policy fields (`max_retries`, `backoff_ms`) exist in the backend schema but are not exposed in the config sidebar forms yet. |

### Suggested follow-up experiments

1. **Add an HTTP Request node before Classify** — fetch a ticket from a URL (e.g. a mock API or a local JSON server) to simulate a real intake pipe.
2. **Add a DB Write node after Merge** — persist the generated response to a database row. The `{{.MERGE_NODE_ID.completion}}` variable carries the final text.
3. **Switch trigger to Webhook** — change the trigger to `webhook` in the trigger panel, save, and fire it with `curl -X POST http://localhost:8080/webhooks/<id> -d '{"ticket": "..."}'` to see the same workflow run without touching the UI.
4. **Change the model** — swap `claude-haiku-4-5-20251001` to `claude-sonnet-4-6` on the Classify node and compare response quality vs. latency in the run panel.
5. **Test failure handling** — intentionally break the Classify node (set the model to `invalid-model`) and re-run. The failed node shows an error in the run panel; downstream nodes are never scheduled.
