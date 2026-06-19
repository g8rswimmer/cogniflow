# EF-03 Dataset Import — Demo & Testing Guide

Use this file to exercise the dataset import feature end-to-end.
Copy any sample below into a file, then upload it from the **EvalSuite detail page → Import Dataset**.

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

Replace `<suite_id>` with an actual EvalSuite ID from your running instance.

```bash
# Basic CSV import
printf "name,description,score\nCase A,First,10\nCase B,,20\n,Missing name,30\n" > /tmp/test.csv
curl -s -X POST http://localhost:8080/v1/eval-suites/<suite_id>/test-cases/import \
  -F "file=@/tmp/test.csv" | jq .
# expect: {"created":2,"skipped":1,"errors":[{"row":4,"message":"name is required"}]}

# JSONL import with bad row
printf '{"name":"Good","initial_data":{"x":1}}\n{"name":"","initial_data":{}}\n{"bad":"json"\n' \
  > /tmp/test.jsonl
curl -s -X POST http://localhost:8080/v1/eval-suites/<suite_id>/test-cases/import \
  -F "file=@/tmp/test.jsonl" | jq .
# expect: {"created":1,"skipped":2,...}

# Row limit check (501 rows)
{ printf "name\n"; for i in $(seq 1 501); do echo "Row $i"; done; } > /tmp/big.csv
curl -s -X POST http://localhost:8080/v1/eval-suites/<suite_id>/test-cases/import \
  -F "file=@/tmp/big.csv" | jq .
# expect: 400 VALIDATION_FAILED — import exceeds maximum of 500 rows

# Unsupported file type
curl -s -X POST http://localhost:8080/v1/eval-suites/<suite_id>/test-cases/import \
  -F "file=@/tmp/test.txt" | jq .
# expect: 400 VALIDATION_FAILED — unsupported file type

# Suite not found
curl -s -X POST http://localhost:8080/v1/eval-suites/does-not-exist/test-cases/import \
  -F "file=@/tmp/test.csv" | jq .
# expect: 404 NOT_FOUND
```
