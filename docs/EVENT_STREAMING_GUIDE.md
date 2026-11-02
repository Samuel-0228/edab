# Event Streaming and Schema Registry Subsystem

Kerob features a high-throughput, partitioned event streaming and schema registry architecture designed for deterministic event ingestion, strict schema governance, and safe historical replay.

---

## 1. Architecture Overview

```
                                  ┌─────────────────────────────┐
                                  │      Incoming Event         │
                                  │  (Webhook, REST, Connector) │
                                  └──────────────┬──────────────┘
                                                 │
                                                 ▼
                                  ┌─────────────────────────────┐
                                  │       Schema Registry       │
                                  │ (JSON Schema / Protobuf IDL)│
                                  └──────┬───────────────┬──────┘
                            Valid        │               │ Invalid (Poison Pill)
                                         ▼               ▼
                       ┌───────────────────┐    ┌────────────────────────┐
                       │ Partition Router  │    │ Stream DLQ Store       │
                       │ (FNV-1a Hashing)  │    │ (Quarantined for Triage│
                       └─────────┬─────────┘    └────────────────────────┘
                                 │
                                 ▼
                     ┌───────────────────────┐
                     │ Pluggable WAL / Stream│
                     │  (Durable WAL / Mem / │
                     │   Kafka / NATS)       │
                     └───────────┬───────────┘
                                 │
                                 ▼
                     ┌───────────────────────┐
                     │ Stream Worker Group   │
                     │ (Per-Partition Lock)  │
                     └───────────┬───────────┘
                                 │
                                 ▼
                     ┌───────────────────────┐
                     │ Engine & Idempotency  │
                     │ (Deduplication Check) │
                     └───────────────────────┘
```

The subsystem comprises three decoupled layers:
1. **Schema Registry**: Governs payload structure and evolutionary compatibility (Backward, Forward, Full, None).
2. **Streaming Broker & Driver Registry**: Implements partitioned message log storage, ordering key hashing, consumer offsets, and dead-letter handling.
3. **Stream Worker & Replay Engine**: Ingests partition records, handles at-least-once deliveries with idempotency deduplication, and allows historical replay across custom time ranges.

---

## 2. Pluggable Adapter Interface

Kerob decouples streaming abstractions from heavy third-party vendor dependencies:

```go
type StreamAdapter interface {
    Publish(ctx context.Context, record *Record) (*PublishResult, error)
    Subscribe(ctx context.Context, topic string, partition int, groupID string, handler MessageHandler) error
    Partitions(topic string) int
    Close() error
}
```

### Supported Adapters:
- **`durable` (Default Production Adapter)**:
  - Append-only Write-Ahead Log (WAL) partitioned on disk (`.stream` log files and `.meta` offset files).
  - Fully transactional, zero external broker dependency, survives engine restarts.
  - Automatically recovers high-water mark offsets and uncommitted records upon boot.
- **`memory` (Local / Testing Adapter)**:
  - Lock-free ring buffer supporting concurrent partition workers and immediate testing.
- **Driver Registry (`streaming.RegisterDriver`)**:
  - Allows external binary extensions to register Apache Kafka, NATS JetStream, AWS Kinesis, or GCP Pub/Sub without forcing large CGo or broker dependencies into the core Kerob binary.

---

## 3. Schema Registry & Evolution

Events can be registered with JSON Schema or Protobuf IDL definitions to prevent schema drift and protect workflow steps from unmarshaling panics.

### Compatibility Modes

| Mode | Allowed Changes | Guarantee |
| :--- | :--- | :--- |
| `BACKWARD` (Default) | Delete fields, add optional fields | Consumers with the new schema can read old payloads |
| `FORWARD` | Add required fields, delete optional fields | Old consumers can read new payloads |
| `FULL` | Add optional fields, delete optional fields | Both backward and forward compatibility |
| `NONE` | Any modification permitted | No evolution validation enforced |

### Schema Registration via API
```bash
curl -X POST http://localhost:8080/api/v1/schemas \
  -H "Authorization: Bearer <API_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "payment.processed",
    "schema_type": "json_schema",
    "compatibility": "BACKWARD",
    "description": "Payment authorization event",
    "definition": "{\"type\":\"object\",\"required\":[\"order_id\",\"amount\"],\"properties\":{\"order_id\":{\"type\":\"string\"},\"amount\":{\"type\":\"number\"}}}"
  }'
```

---

## 4. Partitioning & Ordering Guarantees

- **Deterministic Ordering**: By providing an `ordering_key` (e.g. `tenant_id`, `customer_id`, or `device_id`), records are routed via 32-bit FNV-1a hash modulo `numPartitions`.
- **In-Order Processing**: All events sharing the same ordering key arrive on the identical partition log, guaranteeing strict sequential execution.
- **Independent Failure Domains**: A failure or delay on partition 0 does not block or slow down execution on partition 1.

---

## 5. Poison Pill Quarantine (DLQ) & At-Least-Once Delivery

1. **Pre-Ingestion Validation**: If an incoming stream record violates its registered schema, it is quarantined immediately to `dead_letter_events`.
2. **Offset Commit**: The stream consumer advances the partition offset past the poison record, preventing partition lockup and endless retry crashes.
3. **DLQ Management**:
   - `GET /api/v1/streams/dlq`: List quarantined stream records and error reasons.
   - `POST /api/v1/streams/dlq/:id/retry`: Re-attempts validation and executes matched workflows once the schema or payload issue is addressed.

---

## 6. Historical Stream Replay

Kerob supports on-demand historical replay without losing audit logs or causing duplicate executions:

```bash
curl -X POST http://localhost:8080/api/v1/streams/replay \
  -H "Authorization: Bearer <API_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "kerob.events",
    "event_type": "payment.processed",
    "since": "2026-11-01T00:00:00Z",
    "bypass_idempotency": false
  }'
```

- When `bypass_idempotency` is `false`, events that already executed workflows will be safely skipped.
- When `bypass_idempotency` is `true`, past executions are re-triggered with fresh execution traces (useful for disaster recovery or testing workflow fixes).

---

## 7. Configuration Reference

Environment variables controlling streaming:

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `KEROB_STREAMING_ENABLED` | `true` | Enables background partitioned stream consumer worker |
| `KEROB_STREAM_ADAPTER` | `durable` | Broker implementation: `durable` or `memory` |
| `KEROB_STREAM_DATA_DIR` | `./data/streams` | Storage directory for durable WAL partition logs |
| `KEROB_STREAM_PARTITIONS` | `4` | Number of parallel partitions per topic |
| `KEROB_STREAM_GROUP_ID` | `kerob-core-worker` | Consumer group ID for committed offsets |
