# EF-03 Dataset Import — Demo & Testing Guide

Use this file to exercise the dataset import feature end-to-end.
Copy any sample below into a file, then upload it from the **EvalSuite detail page → Import Dataset**.

---

## Prerequisites

Dataset import requires a workflow and an eval suite to already exist.
The import creates TestCases inside a suite; it does not create the suite or the workflow.

Run the setup script below before uploading any sample. It creates everything via the API and prints the IDs you need.

### Setup script — Samples 1–5

Samples 1–5 use a single HTTP GET node that calls `https://httpbin.org/anything` (a public echo endpoint).
The node ignores the initial data fields; the workflow always succeeds, making it easy to verify the import end-to-end without needing an LLM key or database.

```bash
#!/usr/bin/env bash
# Requirements: curl, jq, cogniflow backend on localhost:8080 (or set COGNIFLOW_URL).
set -euo pipefail
BASE="${COGNIFLOW_URL:-http://localhost:8080}"

echo "→ Creating demo workflow (Samples 1–5)..."
WORKFLOW=$(curl -sf -X POST "$BASE/v1/workflows" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "EF-03 Demo — Echo Workflow",
    "description": "Single HTTP GET node for dataset import testing. Accepts any initial data.",
    "trigger": { "kind": "manual" },
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "echo",
        "type_id": "http.request",
        "label": "Echo",
        "position": { "x": 400, "y": 200 },
        "config": {
          "method": "GET",
          "url": "https://httpbin.org/anything"
        }
      }
    ],
    "edges": []
  }')
WF_ID=$(echo "$WORKFLOW" | jq -r '.id')
echo "  Workflow ID : $WF_ID"

echo "→ Creating eval suite..."
SUITE=$(curl -sf -X POST "$BASE/v1/workflows/$WF_ID/eval-suites" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "EF-03 Import Demo Suite",
    "description": "Import any sample dataset from EF03_DEMO.md against this suite.",
    "pass_threshold": 1.0,
    "max_concurrency": 1
  }')
SUITE_ID=$(echo "$SUITE" | jq -r '.id')
echo "  Suite ID    : $SUITE_ID"

echo ""
echo "Copy and run these exports, then use \$SUITE_ID in any import command below:"
echo ""
echo "  export WF_ID=$WF_ID"
echo "  export SUITE_ID=$SUITE_ID"
```

---

### Setup script — Sample 6 (initial data schema)

Sample 6 matches a workflow that declares `customer_id` and `amount` as typed input fields.
With the schema set, the **Run** modal and TestCase editor render a typed form instead of a raw JSON textarea.

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="${COGNIFLOW_URL:-http://localhost:8080}"

echo "→ Creating schema workflow (Sample 6)..."
WORKFLOW=$(curl -sf -X POST "$BASE/v1/workflows" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "EF-03 Demo — Schema Workflow",
    "description": "Echo workflow with declared initial data schema (customer_id + amount).",
    "trigger": { "kind": "manual" },
    "timeout_seconds": 30,
    "initial_data_schema": {
      "type": "object",
      "properties": {
        "customer_id": {
          "type": "string",
          "title": "Customer ID",
          "description": "Unique identifier for the customer"
        },
        "amount": {
          "type": "number",
          "title": "Amount",
          "description": "Transaction amount in USD"
        }
      },
      "required": ["customer_id", "amount"]
    },
    "nodes": [
      {
        "id": "echo",
        "type_id": "http.request",
        "label": "Echo",
        "position": { "x": 400, "y": 200 },
        "config": {
          "method": "GET",
          "url": "https://httpbin.org/anything"
        }
      }
    ],
    "edges": []
  }')
WF_ID=$(echo "$WORKFLOW" | jq -r '.id')
echo "  Workflow ID : $WF_ID"

echo "→ Creating eval suite..."
SUITE=$(curl -sf -X POST "$BASE/v1/workflows/$WF_ID/eval-suites" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "EF-03 Schema Demo Suite",
    "description": "Suite for Sample 6 — schema-driven initial data form.",
    "pass_threshold": 1.0,
    "max_concurrency": 1
  }')
SUITE_ID=$(echo "$SUITE" | jq -r '.id')
echo "  Suite ID    : $SUITE_ID"

echo ""
echo "  export WF_ID=$WF_ID"
echo "  export SUITE_ID=$SUITE_ID"
```

---

## Sample 1 — Basic CSV (`sample_basic.csv`)

Minimal case: name column only, no initial data fields.

```csv
name,description
Smoke test — empty input,Verify workflow handles empty initial data gracefully
Smoke test — whitespace only,Input is a single space character
Smoke test — very long name,Input is a 500-character string
```

**Expected result:** 3 test cases created, 0 skipped.

---

## Sample 2 — CSV with initial data fields (`sample_with_fields.csv`)

Each extra column becomes a field in `initial_data`. Values are strings.

```csv
name,description,customer_id,query,priority
Happy path — known customer,Standard lookup for an existing customer,cust-001,What is my account balance?,high
Happy path — premium tier,Premium customer with elevated context,cust-002,Upgrade my subscription,high
Edge case — unknown customer,Customer ID does not exist in the system,cust-999,Cancel my account,low
Edge case — empty query,Customer sends a blank message,cust-003,,medium
Edge case — special characters,Query contains quotes and newlines,cust-004,"Hello, ""world""",low
Stress test — long query,Query is near the LLM context limit,cust-005,Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ut enim ad minim veniam,high
```

**Expected result:** 6 test cases created, 0 skipped. Each test case will have `initial_data` with keys `customer_id`, `query`, and `priority`.

---

## Sample 3 — CSV with a bad row (`sample_with_errors.csv`)

Row 3 has an empty name — it will be skipped.

```csv
name,description,score
Case A,First valid case,10
,This row has no name so it will be skipped,20
Case C,Third valid case,30
Case D,Fourth valid case,40
```

**Expected result:** 3 test cases created, 1 skipped. The response `errors` array will show row 3 with message `name is required`.

---

## Sample 4 — JSONL with typed initial data (`sample_typed.jsonl`)

JSONL preserves JSON types (numbers, booleans) unlike CSV.

```jsonl
{"name":"Numeric threshold test","description":"Score should be above 0.8","initial_data":{"score":0.92,"model":"gpt-4o","retries":0}}
{"name":"Boolean flag test","description":"Flagged response should be rejected","initial_data":{"is_flagged":true,"severity":"high","user_id":"u-123"}}
{"name":"Nested object test","description":"Workflow receives a structured payload","initial_data":{"user":{"id":"u-456","tier":"premium"},"context":{"locale":"en-US","timezone":"UTC"}}}
{"name":"Array field test","description":"Initial data includes an array of items","initial_data":{"items":["apple","banana","cherry"],"count":3}}
{"name":"Null field test","description":"Optional field is absent","initial_data":{"required_field":"present"}}
```

**Expected result:** 5 test cases created, 0 skipped. `initial_data` will have native types (numbers as float64, booleans as bool, nested objects as maps).

---

## Sample 5 — JSONL with mixed valid/invalid rows (`sample_mixed.jsonl`)

Lines 2 and 4 are malformed or missing the required name field.

```jsonl
{"name":"Valid case 1","initial_data":{"x":1}}
{bad json — this line will be skipped}
{"name":"Valid case 2","initial_data":{"x":2}}
{"description":"No name field — will be skipped","initial_data":{"x":3}}
{"name":"Valid case 3","initial_data":{"x":4}}
```

**Expected result:** 3 test cases created, 2 skipped. The response `errors` array will show row 2 (invalid JSON) and row 4 (name is required).

---

## Sample 6 — Workflow with initial data schema

If your workflow declares an initial data schema (Workflow Settings → Workflow Inputs), the schema drives the grader field pickers. Use this JSONL to match a schema with fields `customer_id` (string) and `amount` (number):

```jsonl
{"name":"Below threshold","description":"Amount is under the alert threshold","initial_data":{"customer_id":"cust-001","amount":49.99}}
{"name":"At threshold","description":"Amount is exactly at the alert boundary","initial_data":{"customer_id":"cust-002","amount":100.00}}
{"name":"Above threshold","description":"Amount triggers the high-value alert","initial_data":{"customer_id":"cust-003","amount":250.75}}
{"name":"Zero amount","description":"Edge case: zero-value transaction","initial_data":{"customer_id":"cust-004","amount":0}}
{"name":"Large amount","description":"Stress test with very large value","initial_data":{"customer_id":"cust-005","amount":999999.99}}
```

---

## Client-side validation to exercise

| Scenario | How to trigger | Expected behaviour |
|---|---|---|
| Wrong file type | Upload a `.txt` or `.json` file | Error shown immediately, no preview |
| File > 5 MB | Upload a large file | Error shown immediately, no preview |
| > 500 rows | Generate a CSV with 501 data rows | Error shown immediately, no preview |
| All rows invalid | Upload a CSV with only an empty name column | Preview shows all rows as warnings; Import button disabled |
| Header-only CSV | Upload a CSV with only a header row and no data | Preview shows "0 rows ready"; Import creates 0 cases |

---

## Backend curl tests

Run the setup script first and `export SUITE_ID=...` from its output. Then paste any command below.

```bash
BASE="${COGNIFLOW_URL:-http://localhost:8080}"

# Sample 2 — CSV with initial data fields
printf 'name,description,customer_id,query,priority\n' > /tmp/sample2.csv
printf 'Happy path — known customer,Standard lookup,cust-001,What is my balance?,high\n' >> /tmp/sample2.csv
printf 'Edge case — empty query,Blank message,cust-003,,medium\n' >> /tmp/sample2.csv
curl -s -X POST "$BASE/v1/eval-suites/$SUITE_ID/test-cases/import" \
  -F "file=@/tmp/sample2.csv" | jq .
# expect: {"created":2,"skipped":0,"errors":[]}

# Sample 3 — CSV with a bad row
printf "name,description,score\nCase A,First valid case,10\n,Skipped row,20\nCase C,Third valid case,30\n" \
  > /tmp/sample3.csv
curl -s -X POST "$BASE/v1/eval-suites/$SUITE_ID/test-cases/import" \
  -F "file=@/tmp/sample3.csv" | jq .
# expect: {"created":2,"skipped":1,"errors":[{"row":3,"message":"name is required"}]}

# Sample 5 — JSONL with mixed valid/invalid rows
printf '{"name":"Valid case 1","initial_data":{"x":1}}\n' > /tmp/sample5.jsonl
printf '{bad json}\n' >> /tmp/sample5.jsonl
printf '{"name":"Valid case 2","initial_data":{"x":2}}\n' >> /tmp/sample5.jsonl
printf '{"description":"No name field","initial_data":{"x":3}}\n' >> /tmp/sample5.jsonl
printf '{"name":"Valid case 3","initial_data":{"x":4}}\n' >> /tmp/sample5.jsonl
curl -s -X POST "$BASE/v1/eval-suites/$SUITE_ID/test-cases/import" \
  -F "file=@/tmp/sample5.jsonl" | jq .
# expect: {"created":3,"skipped":2,"errors":[{"row":2,...},{"row":4,...}]}

# Row limit check (501 rows → rejected)
{ printf "name\n"; for i in $(seq 1 501); do echo "Row $i"; done; } > /tmp/big.csv
curl -s -X POST "$BASE/v1/eval-suites/$SUITE_ID/test-cases/import" \
  -F "file=@/tmp/big.csv" | jq .
# expect: 400 VALIDATION_FAILED — import exceeds maximum of 500 rows

# Wrong file type
printf "some content" > /tmp/data.txt
curl -s -X POST "$BASE/v1/eval-suites/$SUITE_ID/test-cases/import" \
  -F "file=@/tmp/data.txt" | jq .
# expect: 400 VALIDATION_FAILED — unsupported file type ".txt"

# Suite not found
curl -s -X POST "$BASE/v1/eval-suites/does-not-exist/test-cases/import" \
  -F "file=@/tmp/sample3.csv" | jq .
# expect: 404 NOT_FOUND
```
