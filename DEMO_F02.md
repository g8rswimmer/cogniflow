# F-02 Demo — Message Queue Triggers (Kafka & SQS)

This demo exercises the two new trigger kinds added in F-02: `kafka` (Kafka topic subscription) and `sqs` (AWS SQS long-poll consumer). Posting a message to the queue/topic fires a workflow run with the message body as the run's initial data, accessible via `{{._initial.*}}` template syntax in downstream nodes.

---

## Prerequisites

### Start cogniflow

```bash
docker compose up --build
```

```bash
BASE=http://localhost:8080/v1
```

### Kafka — start with the compose profile

The Kafka service is bundled in docker-compose under the `kafka` profile.
Start everything together:

```bash
docker compose --profile kafka up --build
```

Kafka exposes two listeners:

| Listener | Address | Used by |
|----------|---------|---------|
| `PLAINTEXT` | `kafka:9092` | backend container (use this in the UI) |
| `EXTERNAL` | `localhost:9094` | host-machine tools (producers, CLI) |

Topics are created automatically on first use (`auto.create.topics.enable=true`),
so no manual topic creation is needed.

### SQS — start LocalStack

```bash
docker run -d --name localstack \
  -p 4566:4566 \
  -e SERVICES=sqs \
  localstack/localstack
```

Create the queue:

```bash
AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
  aws --endpoint-url http://localhost:4566 sqs create-queue \
  --queue-name order-events --region us-east-1
```

LocalStack SQS queue URL: `http://localhost:4566/000000000000/order-events`

---

## Scenario 1 — Kafka trigger dispatches a run

Creates a workflow that echoes the incoming order data to an HTTP endpoint.

**Topology:**

```
[kafka topic: order-events] → (trigger) → [echo]
```

### 1a — Create the workflow with a kafka trigger

```bash
WF_KAFKA=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F02 Kafka Order Events",
    "trigger": {
      "kind": "kafka",
      "kafka_brokers": "kafka:9092",
      "kafka_topic": "order-events"
    },
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "echo",
        "type_id": "http.request",
        "label": "Echo order",
        "position": {"x": 0, "y": 0},
        "config": {
          "url": "https://httpbin.org/post",
          "method": "POST",
          "body": "{\"order_id\":\"{{._initial.order_id}}\",\"amount\":{{._initial.amount}}}"
        }
      }
    ],
    "edges": []
  }' | jq -r '.id')

echo "Workflow: $WF_KAFKA"
```

The server immediately starts a consumer goroutine subscribed to `order-events`.

### 1b — Publish a message to the topic

```bash
echo '{"order_id":"ORD-001","amount":99.99,"customer":"alice"}' | \
  docker compose exec -T kafka /opt/kafka/bin/kafka-console-producer.sh \
  --topic order-events --bootstrap-server kafka:9092
```

### 1c — Verify a run was triggered

```bash
sleep 3
curl -s "$BASE/workflows/$WF_KAFKA/runs" | \
  jq '[.[] | {run_id, status, triggered_by}]'
```

**Expected:**

```json
[
  {
    "run_id": "...",
    "status": "succeeded",
    "triggered_by": "kafka"
  }
]
```

### 1d — Inspect the run output

```bash
RUN_KAFKA=$(curl -s "$BASE/workflows/$WF_KAFKA/runs" | jq -r '.[0].run_id')
curl -s "$BASE/runs/$RUN_KAFKA" | jq '{status, triggered_by, echo_body: .node_results.echo.output.body}'
```

**Expected:** `echo_body` contains the forwarded order fields, with `order_id` and `amount` resolved from the message body via `{{._initial.*}}`.

---

## Scenario 2 — Multiple messages, one run each

Publish three messages in sequence and confirm three runs are created.

```bash
for i in 1 2 3; do
  echo "{\"order_id\":\"ORD-00$i\",\"amount\":$((i * 10)).00}" | \
    docker compose exec -T kafka /opt/kafka/bin/kafka-console-producer.sh \
    --topic order-events --bootstrap-server kafka:9092
done

sleep 5
curl -s "$BASE/workflows/$WF_KAFKA/runs" | jq 'length'
```

**Expected:** `4` (1 from Scenario 1 + 3 from this scenario).

---

## Scenario 3 — SQS trigger dispatches a run

Uses LocalStack SQS. Add the AWS env vars to your `.env` before starting:

```bash
# .env additions for LocalStack
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
AWS_ENDPOINT_URL=http://host.docker.internal:4566
```

Then start (or restart) the backend:

```bash
docker compose up -d --build backend
```

### 3a — Create the workflow with an sqs trigger

```bash
WF_SQS=$(curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "F02 SQS Order Events",
    "trigger": {
      "kind": "sqs",
      "sqs_queue_url": "http://localhost:4566/000000000000/order-events",
      "sqs_region": "us-east-1"
    },
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "echo",
        "type_id": "http.request",
        "label": "Echo order",
        "position": {"x": 0, "y": 0},
        "config": {
          "url": "https://httpbin.org/post",
          "method": "POST",
          "body": "{\"order_id\":\"{{._initial.order_id}}\"}"
        }
      }
    ],
    "edges": []
  }' | jq -r '.id')

echo "Workflow: $WF_SQS"
```

### 3b — Send a message to the SQS queue

```bash
AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
  aws --endpoint-url http://localhost:4566 sqs send-message \
  --queue-url http://localhost:4566/000000000000/order-events \
  --message-body '{"order_id":"SQS-001","amount":42.00}' \
  --region us-east-1
```

### 3c — Verify the run

```bash
sleep 25  # SQS long-poll has up to 20s wait; allow one full poll cycle
curl -s "$BASE/workflows/$WF_SQS/runs" | \
  jq '[.[] | {run_id, status, triggered_by}]'
```

**Expected:**

```json
[
  {
    "run_id": "...",
    "status": "succeeded",
    "triggered_by": "sqs"
  }
]
```

The message is deleted from the queue automatically after the run is dispatched successfully.

---

## Scenario 4 — Switching trigger kinds

A workflow can be re-saved to change its trigger. Active consumers are replaced atomically.

```bash
# Switch the Kafka workflow to manual
curl -s -X PUT "$BASE/workflows/$WF_KAFKA" \
  -H 'Content-Type: application/json' \
  -d "$(curl -s $BASE/workflows/$WF_KAFKA | jq '.trigger = {"kind":"manual"}')" | \
  jq '.trigger'
```

**Expected:**

```json
{"kind": "manual"}
```

The Kafka consumer goroutine is cancelled immediately. Publishing another message to `order-events` will no longer trigger a run for this workflow.

Switch back to Kafka:

```bash
curl -s -X PUT "$BASE/workflows/$WF_KAFKA" \
  -H 'Content-Type: application/json' \
  -d "$(curl -s $BASE/workflows/$WF_KAFKA | jq '
    .trigger = {
      "kind": "kafka",
      "kafka_brokers": "kafka:9092",
      "kafka_topic": "order-events"
    }')" | jq '.trigger'
```

A new consumer goroutine starts and resumes consuming from the same group offset.

---

## Scenario 5 — Validation errors

These return `400 VALIDATION_FAILED` without saving or starting a consumer.

### 5a — Missing kafka_topic

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "bad kafka",
    "trigger": {"kind": "kafka", "kafka_brokers": "localhost:9092"}
  }' | jq '{code: .error.code, message: .error.message}'
```

**Expected:**

```json
{
  "code": "VALIDATION_FAILED",
  "message": "kafka_topic is required when trigger.kind is \"kafka\""
}
```

### 5b — Missing kafka_brokers

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "bad kafka",
    "trigger": {"kind": "kafka", "kafka_topic": "my-topic"}
  }' | jq '{code: .error.code, message: .error.message}'
```

**Expected:**

```json
{
  "code": "VALIDATION_FAILED",
  "message": "kafka_brokers is required when trigger.kind is \"kafka\""
}
```

### 5c — Missing sqs_region

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "bad sqs",
    "trigger": {"kind": "sqs", "sqs_queue_url": "https://sqs.us-east-1.amazonaws.com/123/q"}
  }' | jq '{code: .error.code, message: .error.message}'
```

**Expected:**

```json
{
  "code": "VALIDATION_FAILED",
  "message": "sqs_region is required when trigger.kind is \"sqs\""
}
```

### 5d — Missing sqs_queue_url

```bash
curl -s -X POST $BASE/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "bad sqs",
    "trigger": {"kind": "sqs", "sqs_region": "us-east-1"}
  }' | jq '{code: .error.code, message: .error.message}'
```

**Expected:**

```json
{
  "code": "VALIDATION_FAILED",
  "message": "sqs_queue_url is required when trigger.kind is \"sqs\""
}
```

---

## Scenario 6 — Non-JSON message body (graceful fallback)

The consumer does not crash on a non-JSON message. It logs a warning and dispatches the run with an empty `initial_data` map.

```bash
# Publish a plain-text message
echo 'plain text — not JSON' | \
  docker compose exec -T kafka /opt/kafka/bin/kafka-console-producer.sh \
  --topic order-events --bootstrap-server kafka:9092

sleep 3
RUN_PLAIN=$(curl -s "$BASE/workflows/$WF_KAFKA/runs" | jq -r '.[0].run_id')
curl -s "$BASE/runs/$RUN_PLAIN" | jq '{status, triggered_by}'
```

**Expected:** `status: "succeeded"`, `triggered_by: "kafka"`. The `echo` node receives an empty `_initial` context but still runs (the URL template `{{._initial.order_id}}` resolves to an empty string).

---

## Scenario 7 — Consumer survives a restart (LoadAll)

Trigger configs are persisted in the database. On server restart, all kafka and sqs triggers are re-armed automatically before the HTTP port opens.

```bash
# Bounce the backend
docker compose restart backend

sleep 5  # wait for startup

# The kafka consumer is already running again; publish a message
echo '{"order_id":"POST-RESTART","amount":1.00}' | \
  docker compose exec -T kafka /opt/kafka/bin/kafka-console-producer.sh \
  --topic order-events --bootstrap-server kafka:9092

sleep 3
curl -s "$BASE/workflows/$WF_KAFKA/runs" | jq '[.[] | .triggered_by] | unique'
```

**Expected:** `["kafka"]` — all runs (including post-restart) were triggered by the Kafka consumer.

---

## Cleanup

```bash
curl -s -X DELETE "$BASE/workflows/$WF_KAFKA"
curl -s -X DELETE "$BASE/workflows/$WF_SQS"
docker compose --profile kafka down -v
docker stop localstack && docker rm localstack
echo "Done"
```
