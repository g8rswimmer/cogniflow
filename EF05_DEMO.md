# EF-05 Demo: Custom Grader Plugins via gRPC

This document walks through how to write, register, and use an out-of-process
grader plugin — an external process that implements the `GraderPlugin` gRPC
service contract and plugs into the eval engine at runtime.

---

## What changed

| Before | After |
|--------|-------|
| Grader type must be one of the five built-ins | Any process implementing the gRPC contract can be a grader type |
| Adding a new grader required modifying core eval code | New grader types register via `POST /v1/admin/grader-plugins` with no server restart |
| `GraderType` union was closed | `'plugin'` type routes through the `GraderRegistry` to the remote process |
| No admin UI for grader management | "Grader Plugins" link in the workflow list navbar → full CRUD page |

---

## Architecture

```
External process (any language)
   └── implements GraderPlugin gRPC service
         ├── Meta()  → type_id, display_name, description, config_schema
         └── Grade() → verdict ("pass"/"fail"/"error"), explanation, optional score

POST /v1/admin/grader-plugins  {"address":"host:port"}
   └── grader_plugin.RegisterOne()
         ├── dials gRPC
         ├── calls Meta() to validate + cache descriptor
         ├── registers grpcProxy in GraderRegistry
         └── persists to grader_registrations table (survives restart)

EvalRunner.executeTestCase()
   └── BuildGrader(def, llmFactory, graderRegistry)
         └── graderRegistry.Get(def.Type) → grpcProxy
               └── pluginGrader.Grade(ctx, data)
                     └── gRPC Grade(data_json, config_json) → GraderResult
```

---

## Step 1 — Write the example plugin

Create a file `example_grader_plugin/main.go` anywhere on your machine.
This plugin implements a **word-count threshold** grader: it extracts a text
field from the node output and passes if the word count meets a minimum.

```go
//go:build ignore

// Run with: go run main.go --port=9001
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	graderv1 "github.com/g8rswimmer/cogniflow/proto/grader/v1"
)

type wordCountServer struct {
	graderv1.UnimplementedGraderPluginServer
}

func (s *wordCountServer) Meta(_ context.Context, _ *graderv1.MetaRequest) (*graderv1.MetaResponse, error) {
	return &graderv1.MetaResponse{
		TypeId:      "example.word_count",
		DisplayName: "Word Count Threshold",
		Description: "Passes when a text field contains at least min_words words.",
		ConfigSchema: []byte(`{
			"type": "object",
			"required": ["field_path", "min_words"],
			"properties": {
				"field_path": {
					"type": "string",
					"title": "Field path",
					"description": "gjson dot-path into the graded data (e.g. completion)"
				},
				"min_words": {
					"type": "number",
					"title": "Minimum word count",
					"description": "Number of words required for a pass verdict"
				}
			}
		}`),
	}, nil
}

func (s *wordCountServer) Grade(_ context.Context, req *graderv1.GradeRequest) (*graderv1.GradeResponse, error) {
	var data map[string]any
	if err := json.Unmarshal(req.GetData(), &data); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal data: %v", err)
	}
	var cfg struct {
		FieldPath string  `json:"field_path"`
		MinWords  float64 `json:"min_words"`
	}
	if err := json.Unmarshal(req.GetConfig(), &cfg); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unmarshal config: %v", err)
	}
	if cfg.FieldPath == "" {
		return errorResult("field_path is required in config"), nil
	}

	val, ok := data[cfg.FieldPath]
	if !ok {
		return errorResult(fmt.Sprintf("field %q not found in data", cfg.FieldPath)), nil
	}
	text := fmt.Sprintf("%v", val)
	wordCount := len(strings.Fields(text))

	minWords := int(cfg.MinWords)
	if wordCount >= minWords {
		return &graderv1.GradeResponse{
			Result: &graderv1.GradeResponse_GradeResult{
				GradeResult: &graderv1.GradeResult{
					Verdict:     "pass",
					Explanation: fmt.Sprintf("%d words ≥ %d required", wordCount, minWords),
					ActualValue: []byte(fmt.Sprintf("%d", wordCount)),
				},
			},
		}, nil
	}
	return &graderv1.GradeResponse{
		Result: &graderv1.GradeResponse_GradeResult{
			GradeResult: &graderv1.GradeResult{
				Verdict:     "fail",
				Explanation: fmt.Sprintf("%d words < %d required", wordCount, minWords),
				ActualValue: []byte(fmt.Sprintf("%d", wordCount)),
			},
		},
	}, nil
}

func errorResult(msg string) *graderv1.GradeResponse {
	return &graderv1.GradeResponse{
		Result: &graderv1.GradeResponse_GradeResult{
			GradeResult: &graderv1.GradeResult{
				Verdict:     "error",
				Explanation: msg,
			},
		},
	}
}

func main() {
	port := flag.Int("port", 9001, "gRPC listen port")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	graderv1.RegisterGraderPluginServer(srv, &wordCountServer{})
	log.Printf("word-count grader plugin listening on %s", addr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
```

---

## Step 2 — Start the plugin and the stack

**Terminal 1 — run the plugin** (from inside the `backend/` directory so the
proto import resolves):

```bash
cd backend
go run ../example_grader_plugin/main.go --port=9001
# → word-count grader plugin listening on :9001
```

**Terminal 2 — start cogniflow**:

```bash
docker compose up --build
```

> **Note:** Because Docker and the plugin run on the same machine, use
> `host.docker.internal:9001` (macOS/Windows) or the host IP on Linux when
> calling the admin API from inside Docker. The curl commands below run
> against the exposed backend port on localhost, so `localhost:9001` works
> as the plugin address from the API call perspective — the backend process
> running inside Docker needs to reach the plugin. On macOS with Docker
> Desktop, the backend container can reach the host via
> `host.docker.internal:9001`.

---

## Scenario 1 — Register the plugin

```bash
# Register the word-count grader plugin.
# Replace "localhost:9001" with "host.docker.internal:9001" if running in Docker.
curl -s -X POST http://localhost:8080/v1/admin/grader-plugins \
  -H 'Content-Type: application/json' \
  -d '{"address": "host.docker.internal:9001"}' | jq .
```

Expected response:

```json
{
  "type_id":      "example.word_count",
  "display_name": "Word Count Threshold",
  "description":  "Passes when a text field contains at least min_words words.",
  "address":      "host.docker.internal:9001",
  "config_schema": { ... },
  "registered_at": "2026-06-20T..."
}
```

**Verify it's listed:**

```bash
curl -s http://localhost:8080/v1/admin/grader-plugins | jq '.grader_plugins[] | {type_id, display_name}'
# → {"type_id": "example.word_count", "display_name": "Word Count Threshold"}
```

---

## Scenario 2 — Create a workflow and eval suite

**Create a workflow with a single LLM node** (or any node that produces text):

```bash
WF_ID=$(curl -s -X POST http://localhost:8080/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "EF-05 Demo Workflow",
    "trigger": {"kind": "manual"},
    "timeout_seconds": 60,
    "nodes": [
      {
        "id": "n1",
        "type_id": "http.request",
        "label": "Fetch quote",
        "position": {"x": 100, "y": 100},
        "config": {
          "url": "https://dummyjson.com/quotes/1",
          "method": "GET"
        }
      }
    ],
    "edges": []
  }' | jq -r '.id')
echo "Workflow: $WF_ID"
```

**Verify it runs:**

```bash
RUN_ID=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/runs \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')
sleep 4
curl -s http://localhost:8080/v1/runs/$RUN_ID \
  | jq '{status, body_preview: .final_output.n1.body[:80]}'
# → {"status": "succeeded", "body_preview": "{\"id\":1,\"quote\":\"Life is..."}
```

**Create an EvalSuite:**

```bash
SUITE_ID=$(curl -s -X POST http://localhost:8080/v1/workflows/$WF_ID/eval-suites \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "EF-05 Word Count Demo",
    "pass_threshold": 1.0
  }' | jq -r '.id')
echo "Suite: $SUITE_ID"
```

**Add a test case using the plugin grader:**

```bash
# The grader type is the plugin's type_id: "example.word_count"
# Config keys (field_path, min_words) match the plugin's config_schema.
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Response body has at least 5 words",
    "initial_data": {},
    "mocks": [],
    "graders": [
      {
        "id": "'"$(uuidgen | tr "[:upper:]" "[:lower:]")"'",
        "name": "word count check",
        "type": "example.word_count",
        "scope": "node",
        "node_id": "n1",
        "config": {
          "field_path": "body",
          "min_words": 5
        }
      }
    ]
  }' | jq '{id, name}'
```

---

## Scenario 3 — Run the suite and verify the plugin grader fires

```bash
EVAL_RUN_ID=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  -H 'Content-Type: application/json' | jq -r '.id')
echo "Eval run: $EVAL_RUN_ID"

# Poll until complete (usually < 10 s)
for i in $(seq 1 10); do
  STATUS=$(curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq -r '.status')
  echo "  status: $STATUS"
  [ "$STATUS" = "completed" ] && break
  sleep 2
done

# Inspect the grader result
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID | jq '
  .test_case_results[0].grader_results[] | {
    grader_name,
    grader_type,
    verdict,
    explanation,
    actual_value
  }'
```

Expected output:

```json
{
  "grader_name":  "word count check",
  "grader_type":  "example.word_count",
  "verdict":      "pass",
  "explanation":  "47 words ≥ 5 required",
  "actual_value": "47"
}
```

**Check the plugin terminal** — you should see gRPC calls logged there as each test case runs.

---

## Scenario 4 — Failing grader (threshold too high)

Add a second test case with an unreachable threshold:

```bash
curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/test-cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Body has at least 10000 words (should fail)",
    "initial_data": {},
    "mocks": [],
    "graders": [
      {
        "id": "'"$(uuidgen | tr "[:upper:]" "[:lower:]")"'",
        "name": "word count too high",
        "type": "example.word_count",
        "scope": "node",
        "node_id": "n1",
        "config": {
          "field_path": "body",
          "min_words": 10000
        }
      }
    ]
  }' | jq '{id, name}'

EVAL_RUN_ID2=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  -H 'Content-Type: application/json' | jq -r '.id')
sleep 10
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID2 \
  | jq '{status, passed_count, failed_count, error_count}'
# → {"status":"completed","passed_count":1,"failed_count":1,"error_count":0}

curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID2 | jq '
  .test_case_results[] | {
    name: .test_case_name,
    passed,
    verdict: .grader_results[0].verdict,
    explanation: .grader_results[0].explanation
  }'
```

---

## Scenario 5 — Update plugin address

```bash
# Restart the plugin on a different port:
# go run main.go --port=9002
# Then update:
curl -s -X PUT http://localhost:8080/v1/admin/grader-plugins/example.word_count \
  -H 'Content-Type: application/json' \
  -d '{"address": "host.docker.internal:9002"}' | jq '{type_id, address}'
# → {"type_id": "example.word_count", "address": "host.docker.internal:9002"}
```

The registry swaps atomically — in-flight requests finish against the old
connection; new eval runs use the new address.

---

## Scenario 6 — Deregister the plugin

```bash
curl -s -X DELETE http://localhost:8080/v1/admin/grader-plugins/example.word_count
# → 204 No Content

# Confirm it's gone
curl -s http://localhost:8080/v1/admin/grader-plugins \
  | jq '.grader_plugins | length'
# → 0

# Running the suite now produces an error verdict (grader not found)
EVAL_RUN_ID3=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  -H 'Content-Type: application/json' | jq -r '.id')
sleep 10
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID3 \
  | jq '.test_case_results[0].grader_results[0] | {verdict, explanation}'
# → {"verdict": "error", "explanation": "grader not available: grader plugin \"example.word_count\": not registered"}
```

---

## Scenario 7 — Restart survivability

Plugin registrations are persisted to `grader_registrations`. Restart the
backend and confirm the plugin is automatically re-connected:

```bash
# 1. Re-register the plugin (if deregistered above)
curl -s -X POST http://localhost:8080/v1/admin/grader-plugins \
  -H 'Content-Type: application/json' \
  -d '{"address": "host.docker.internal:9001"}' | jq '{type_id, registered_at}'

# 2. Restart the backend container
docker compose restart backend
sleep 10
curl -s http://localhost:8080/health | jq '.status'
# → "ok"

# 3. The plugin is still listed — reloaded from DB at startup
curl -s http://localhost:8080/v1/admin/grader-plugins \
  | jq '.grader_plugins[] | {type_id, address}'
# → {"type_id": "example.word_count", "address": "host.docker.internal:9001"}

# 4. A new eval run works without re-registering
EVAL_RUN_ID4=$(curl -s -X POST http://localhost:8080/v1/eval-suites/$SUITE_ID/runs \
  -H 'Content-Type: application/json' | jq -r '.id')
sleep 10
curl -s http://localhost:8080/v1/eval-runs/$EVAL_RUN_ID4 \
  | jq '{status, passed_count}'
# → {"status": "completed", "passed_count": 1}
```

**Startup logs should include:**

```
level=INFO msg="stored grader plugin registered" type_id=example.word_count address=host.docker.internal:9001
```

---

## Frontend walkthrough

1. Open `http://localhost:3000`.
2. Click **⚗ Grader Plugins** in the top navbar → `GraderPluginAdminPage`.
3. Enter `host.docker.internal:9001` and click **Register** — the plugin appears in the list.
4. Navigate to the EF-05 Demo Workflow → **⚗ Evals** → open the demo suite → click a test case.
5. In the Graders section, click **+ Add Grader** → Type dropdown shows **Custom Plugin**.
6. Select **Custom Plugin** → a second dropdown lists `Word Count Threshold (example.word_count)`.
7. Set `field_path = body`, `min_words = 5`, save.
8. Click **Run Suite** — the result row shows `example.word_count · pass · "47 words ≥ 5 required"`.

---

## DB verification

```bash
docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SELECT type_id, address, display_name, registered_at FROM grader_registrations\G"

docker compose exec mysql mysql -u cogniflow -pcogniflow_pass cogniflow \
  -e "SELECT grader_results FROM eval_test_case_results ORDER BY created_at DESC LIMIT 1\G" \
  | python3 -c "import sys,json; raw=sys.stdin.read(); \
    start=raw.index('['); print(json.dumps(json.loads(raw[start:].split('\n')[0]), indent=2))"
```

---

## Error cases

| Situation | Response |
|-----------|----------|
| Plugin process not running when registering | `502 GRADER_UNAVAILABLE` |
| Plugin at new address returns different `type_id` on update | `422 TYPE_ID_MISMATCH` |
| Registering `type_id` already in registry | `409 GRADER_ALREADY_REGISTERED` |
| Plugin crashes after registration | `error` verdict with RPC error message in `explanation` |
| Deregistered plugin used in eval run | `error` verdict with `"not registered"` explanation |
| Plugin returns invalid `config_schema` JSON at registration | Registration rejected with descriptive error |
