# Generic Event-Driven Automation Engine Boilerplate

A clean, production-ready, reusable backend boilerplate for event-driven automation systems built with **Go** and **PostgreSQL**.

The goal of this engine is to serve as a foundational template that can be cloned and adapted to any event-driven automation use case without modifying core logic:
- Telegram → Telegram
- Telegram → Discord
- RSS → Telegram
- Webhook → Telegram
- GitHub → Discord
- API → API
- Database → Webhook

---

## 1. Core Principle

The system follows a strict pipeline:

```text
SOURCE → INGEST → NORMALIZE → ROUTE → DELIVER → RECORD
```

**The core engine does not know or care what the source or destination service is.**

External services interact with the system strictly through **Source Adapters** and **Destination Adapters**. Service-specific implementations (e.g. Telegram, Discord, GitHub, Slack) are plugins on top of the generic core.

---

## 2. Architecture & Event Flow

### System Overview

```mermaid
flowchart LR
    subgraph ExternalSources["External Sources"]
        WebhookSrc["Webhook / HTTP Push"]
        PollSrc["Poller / API / RSS"]
    end

    subgraph Adapters["Adapters Layer"]
        SA["Source Adapters\n(Normalize to Generic Event)"]
        DA["Destination Adapters\n(Translate & Send)"]
    end

    subgraph CoreEngine["Core Engine"]
        Ingest["Ingestion Pipeline"]
        RouteEngine["Routing Engine"]
        Worker["Background Worker Pool"]
    end

    subgraph Storage["PostgreSQL"]
        DBEvents[("events\n(source_id + ext_id unique)")]
        DBRoutes[("routes\n(filters & bindings)")]
        DBJobs[("delivery_jobs\n(event_id + dest_id unique)")]
    end

    subgraph Targets["External Targets"]
        TgtA["Target Service A"]
        TgtB["Target Service B"]
    end

    ExternalSources --> SA
    SA --> Ingest
    Ingest --> DBEvents
    Ingest --> RouteEngine
    DBRoutes -.-> RouteEngine
    RouteEngine --> DBJobs
    Worker -. Claim with SKIP LOCKED .-> DBJobs
    Worker --> DA
    DA --> Targets
    DA -. Result / Error .-> Worker
    Worker -. Status update & Retry .-> DBJobs
```

### Event Lifecycle

1. **Ingest & Normalize**: A Source Adapter receives raw data from an external provider (webhook push or poll) and translates it into a generic `Event`.
2. **Persist (Idempotent)**: The event is stored in PostgreSQL. A database uniqueness constraint on `(source_id, external_event_id)` guarantees the same external event is never duplicated.
3. **Route**: The routing engine queries active routes matching the event's source ID and evaluates any optional filter criteria (such as `event_types`).
4. **Enqueue Deliveries**: For each matched route, a `DeliveryJob` is created with status `pending`. Uniqueness constraint on `(event_id, destination_id)` ensures idempotent job creation.
5. **Process Deliveries**: Background workers claim pending jobs atomically using `FOR UPDATE SKIP LOCKED`.
6. **Deliver & Record**: The worker looks up the destination's adapter, connects it with the destination's configuration, and delivers the generic event.
7. **Retry / Dead-Letter**:
   - If delivery succeeds, the job is marked `delivered`.
   - If delivery fails, the failure error is recorded, an exponential backoff is calculated, and status is set to `failed`.
   - If max retry attempts are reached, the job is permanently marked `dead_letter`.

### Step-by-Step Delivery and Retry Workflow

```text
Example Source
      ↓
   Event #123
      ↓
   PostgreSQL (Idempotency Check)
      ↓
   Route Lookup
      ↓
 ┌────┴─────┐
 ↓          ↓
Target A   Target B
 ↓          ↓
SUCCESS    FAILED
             ↓
           RETRY (Exponential Backoff)
             ↓
           SUCCESS (or DEAD_LETTER)
```

---

## 3. Project Structure

```text
automation-engine/
├── cmd/
│   └── server/
│       └── main.go              # Application bootstrap & graceful shutdown
│
├── internal/
│   ├── core/
│   │   ├── event/               # Generic Event model and PostgreSQL store
│   │   ├── routing/             # Route entity, rule evaluation, and store
│   │   ├── delivery/            # DeliveryJob, atomic claim store, and background worker
│   │   └── workflow/            # Ingestion and processing pipeline orchestrator
│   │
│   ├── adapters/
│   │   ├── sources/             # SourceAdapter interface, registry, mock, webhook
│   │   └── destinations/        # DestinationAdapter interface, registry, mock, webhook, stdout
│   │
│   ├── database/                # Connection pooling and embedded SQL migration runner
│   ├── config/                  # Environment variable configuration loader
│   └── api/                     # REST API server and handlers (Go 1.22+ ServeMux)
│
├── migrations/                  # Pure SQL schema migrations (embedded in binary)
│   ├── 000001_init.up.sql
│   ├── 000001_init.down.sql
│   └── migrations.go
│
├── tests/                       # Unit and API integration tests
│   ├── memstore.go              # In-memory stores for isolated unit testing
│   ├── engine_test.go           # 10 core requirement test suite
│   └── api_test.go              # REST API integration test suite
│
├── docker-compose.yml           # Multi-container orchestration (PostgreSQL + Engine)
├── Dockerfile                   # Multi-stage lightweight Alpine container
├── .env.example                 # Configuration template
├── go.mod                       # Go module definition
└── README.md                    # Project documentation
```

---

## 4. Key Models

### Generic Event
```json
{
  "id": "c1f7a0b3-5776-47b8-89c0-9d0a6c6c7e2b",
  "source_id": "8bb38f82-a0ef-4f11-9277-3e4b786c527e",
  "external_event_id": "gh-push-892341",
  "event_type": "git.push",
  "payload": {
    "repository": "my-org/repo",
    "ref": "refs/heads/main",
    "commits": 3
  },
  "metadata": {
    "ip": "192.30.252.0",
    "user_agent": "GitHub-Hookshot/abc123"
  },
  "created_at": "2026-10-15T12:00:00Z"
}
```

### Source & Destination
Sources and destinations store flexible JSON configuration so individual adapters can declare their own settings without schema alterations:
```json
{
  "id": "8bb38f82-a0ef-4f11-9277-3e4b786c527e",
  "name": "Incoming GitHub Webhook",
  "adapter_type": "webhook",
  "config": {
    "header_secret": "X-Hub-Signature-256"
  },
  "enabled": true
}
```

### Route
Routes connect a Source to a Destination, with optional filtering criteria:
```json
{
  "id": "e229c9b4-64df-419b-a01d-55734fa09ab4",
  "source_id": "8bb38f82-a0ef-4f11-9277-3e4b786c527e",
  "destination_id": "51a37c98-1e43-4ce4-8933-bf97cf339230",
  "enabled": true,
  "filters": {
    "event_types": ["git.push", "release.published"]
  }
}
```

### Delivery Job
```json
{
  "id": "7616110a-ffec-4581-9b16-43ad07271e11",
  "event_id": "c1f7a0b3-5776-47b8-89c0-9d0a6c6c7e2b",
  "destination_id": "51a37c98-1e43-4ce4-8933-bf97cf339230",
  "status": "pending",
  "attempts": 0,
  "max_attempts": 3,
  "next_attempt_at": "2026-10-15T12:00:00Z",
  "last_error": null
}
```

---

## 5. Idempotency Guarantees

Idempotency is enforced at the database level:

1. **Event Ingestion**:
   ```sql
   CONSTRAINT uq_events_source_external UNIQUE (source_id, external_event_id)
   ```
   If the same external event arrives multiple times, the database rejects the duplicate insert (`ON CONFLICT DO NOTHING`), returns the existing event, and skips route processing and duplicate job scheduling.

2. **Delivery Job Enqueueing**:
   ```sql
   CONSTRAINT uq_delivery_jobs_event_dest UNIQUE (event_id, destination_id)
   ```
   Each destination is assigned exactly one delivery task per event. If Destination B fails while Destination A succeeds, retrying Destination B will never trigger a duplicate delivery to Target A.

3. **Concurrency-Safe Job Claiming**:
   ```sql
   UPDATE delivery_jobs
   SET status = 'processing', attempts = attempts + 1, updated_at = NOW()
   WHERE id = (
       SELECT id FROM delivery_jobs
       WHERE status IN ('pending', 'failed') AND next_attempt_at <= NOW()
       ORDER BY next_attempt_at ASC
       FOR UPDATE SKIP LOCKED
       LIMIT 1
   ) RETURNING ...;
   ```
   Multiple workers running across threads or containers can never claim the same job simultaneously.

---

## 6. Getting Started

### Prerequisites
- Docker & Docker Compose (or Go 1.22+ and PostgreSQL 14+)

### Running with Docker Compose

1. Clone or navigate to the repository:
   ```bash
   cd automation-engine
   ```

2. Copy environment configuration:
   ```bash
   cp .env.example .env
   ```

3. Start all services:
   ```bash
   docker compose up -d --build
   ```

4. Verify service health:
   ```bash
   curl http://localhost:8080/health
   # {"status":"healthy"}

   curl http://localhost:8080/ready
   # {"registered_destinations":["mock","webhook","stdout"],"registered_sources":["mock","webhook"],"status":"ready"}
   ```

---

## 7. Configuration & Environment Variables

| Variable | Description | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://postgres:postgres@localhost:5432/automation?sslmode=disable` |
| `PORT` | REST API HTTP port | `8080` |
| `WORKER_POLL_INTERVAL` | Polling frequency for pending delivery jobs | `1s` |
| `WORKER_CONCURRENCY` | Number of concurrent worker goroutines | `4` |
| `MAX_RETRIES` | Max attempts before marking job `dead_letter` | `3` |
| `INITIAL_BACKOFF` | Starting backoff duration for retries | `2s` |
| `MAX_BACKOFF` | Maximum capped backoff duration | `1m` |
| `LOG_LEVEL` | Structured logging level (`debug`, `info`, `warn`, `error`) | `info` |
| `ENVIRONMENT` | Environment name (`development`, `production`) | `development` |

---

## 8. REST API Reference

### Health
- `GET /health` - Basic liveness probe.
- `GET /ready` - Readiness probe returning registered adapter types.

### Sources
- `GET /sources` - List all configured sources.
- `POST /sources` - Create a new source.
  ```json
  {
    "name": "My Webhook",
    "adapter_type": "webhook",
    "config": {},
    "enabled": true
  }
  ```
- `GET /sources/{id}` - Get source details.
- `PUT /sources/{id}` - Update source configuration or toggle enabled state.
- `DELETE /sources/{id}` - Delete source and cascade routes.
- `POST /sources/{id}/ingest` - Push raw data into the source pipeline (useful for webhooks).

### Destinations
- `GET /destinations` - List all configured destinations.
- `POST /destinations` - Create a new destination.
  ```json
  {
    "name": "Webhook Sink",
    "adapter_type": "webhook",
    "config": {
      "target_url": "https://example.com/api/events",
      "timeout_seconds": 5
    },
    "enabled": true
  }
  ```
- `GET /destinations/{id}` - Get destination details.
- `PUT /destinations/{id}` - Update destination configuration.
- `DELETE /destinations/{id}` - Delete destination.

### Routes
- `GET /routes` - List all routes.
- `POST /routes` - Create a route linking a source to a destination:
  ```json
  {
    "source_id": "SOURCE_UUID",
    "destination_id": "DEST_UUID",
    "enabled": true,
    "filters": {
      "event_types": ["alert", "status.change"]
    }
  }
  ```
- `GET /routes/{id}` - Get route details.
- `PUT /routes/{id}` - Update route bindings or filters.
- `DELETE /routes/{id}` - Delete route.

### Events
- `GET /events?source_id=...&event_type=...&limit=50&offset=0` - List persisted events.
- `GET /events/{id}` - Get event details.
- `POST /events` - Ingest a generic event directly.

### Deliveries
- `GET /deliveries?event_id=...&destination_id=...&status=...` - List delivery jobs.
- `GET /deliveries/{id}` - Get delivery job details, retry attempt count, and last error.
- `POST /deliveries/{id}/retry` - Manually reset a failed or dead-letter job for immediate retry.

---

## 9. End-to-End Walkthrough Example

### 1. Create a Source
```bash
SOURCE_ID=$(curl -s -X POST http://localhost:8080/sources \
  -H "Content-Type: application/json" \
  -d '{"name":"System Alerts","adapter_type":"webhook","enabled":true}' | grep -o '"id":"[^"]*' | cut -d'"' -f4)
echo "Created Source: $SOURCE_ID"
```

### 2. Create Two Destinations
```bash
# Destination 1: Stdout Logger
DEST_LOGGER_ID=$(curl -s -X POST http://localhost:8080/destinations \
  -H "Content-Type: application/json" \
  -d '{"name":"Stdout Logger","adapter_type":"stdout","enabled":true}' | grep -o '"id":"[^"]*' | cut -d'"' -f4)

# Destination 2: Mock target
DEST_MOCK_ID=$(curl -s -X POST http://localhost:8080/destinations \
  -H "Content-Type: application/json" \
  -d '{"name":"Mock Target","adapter_type":"mock","enabled":true}' | grep -o '"id":"[^"]*' | cut -d'"' -f4)
```

### 3. Bind Source to Destinations via Routes
```bash
curl -X POST http://localhost:8080/routes \
  -H "Content-Type: application/json" \
  -d "{\"source_id\":\"$SOURCE_ID\",\"destination_id\":\"$DEST_LOGGER_ID\",\"enabled\":true}"

curl -X POST http://localhost:8080/routes \
  -H "Content-Type: application/json" \
  -d "{\"source_id\":\"$SOURCE_ID\",\"destination_id\":\"$DEST_MOCK_ID\",\"enabled\":true}"
```

### 4. Push an External Event
```bash
curl -X POST http://localhost:8080/sources/$SOURCE_ID/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "id": "alert-9001",
    "type": "server.cpu_high",
    "cpu": 98.5,
    "host": "prod-worker-01"
  }'
```

### 5. Verify Deliveries
```bash
curl -s http://localhost:8080/deliveries | jq .
```
Both destinations will receive independent delivery jobs and transition to `delivered`.

---

## 10. Extending the Engine: Adding Adapters

The engine uses a clean **Adapter Registry / Factory Pattern**. Adding new integrations requires zero conditional checks in the core engine.

### How to Create a New Source Adapter

Implement the `SourceAdapter` interface in `internal/adapters/sources/`:

```go
package sources

import (
    "context"
    "encoding/json"
    "automation-engine/internal/core/event"
)

type RSSSourceAdapter struct {
    feedURL string
}

func (a *RSSSourceAdapter) Name() string { return "rss" }

func (a *RSSSourceAdapter) Connect(ctx context.Context, config json.RawMessage) error {
    var cfg struct { FeedURL string `json:"feed_url"` }
    _ = json.Unmarshal(config, &cfg)
    a.feedURL = cfg.FeedURL
    return nil
}

func (a *RSSSourceAdapter) Health(ctx context.Context) error { return nil }

func (a *RSSSourceAdapter) Poll(ctx context.Context) ([]RawEvent, error) {
    // Fetch RSS feed items and return []RawEvent
    return nil, nil
}

func (a *RSSSourceAdapter) Normalize(ctx context.Context, sourceID string, raw []byte) (*event.Event, error) {
    // Parse RSS item and construct generic Event
    return event.New(sourceID, "item-guid-123", "rss.article", raw, nil)
}
```

Register it in `cmd/server/main.go`:
```go
srcRegistry.Register("rss", func() sources.SourceAdapter {
    return &RSSSourceAdapter{}
})
```

---

### How to Create a New Destination Adapter

Implement the `DestinationAdapter` interface in `internal/adapters/destinations/`:

```go
package destinations

import (
    "context"
    "encoding/json"
    "automation-engine/internal/core/event"
)

type DiscordDestinationAdapter struct {
    webhookURL string
}

func (d *DiscordDestinationAdapter) Name() string { return "discord" }

func (d *DiscordDestinationAdapter) Connect(ctx context.Context, config json.RawMessage) error {
    var cfg struct { WebhookURL string `json:"webhook_url"` }
    _ = json.Unmarshal(config, &cfg)
    d.webhookURL = cfg.WebhookURL
    return nil
}

func (d *DiscordDestinationAdapter) Health(ctx context.Context) error { return nil }

func (d *DiscordDestinationAdapter) Deliver(ctx context.Context, ev *event.Event) error {
    // Translate generic Event to Discord webhook JSON format and execute HTTP POST
    return nil
}
```

Register it in `cmd/server/main.go`:
```go
destRegistry.Register("discord", func() destinations.DestinationAdapter {
    return &DiscordDestinationAdapter{}
})
```

---

## 11. Example: Future Telegram Integration

When adding Telegram in the future, the core engine remains untouched. Telegram will simply exist as an adapter:

```text
┌─────────────────┐
│ TelegramSource  │ (Receives Telegram Update via Webhook / Long-polling)
└────────┬────────┘
         │ Normalize(Update) -> Generic Event:
         │   ExternalEventID: Update.UpdateID
         │   EventType: "telegram.message"
         │   Payload: { "chat_id": 123, "text": "Hello", "media": ... }
         ▼
┌─────────────────┐
│   Core Engine   │ (Stores event, resolves routes, creates delivery jobs)
└────────┬────────┘
         │ Deliver(Generic Event)
         ▼
┌─────────────────────────┐
│   TelegramDestination   │ (Parses generic event payload, invokes SendMessage API)
└─────────────────────────┘
```

Secrets (bot tokens) can either be referenced via environment variable mappings (e.g., `{"token_env": "TELEGRAM_BOT_TOKEN"}`) or passed via adapter configuration.

---

## 12. Testing

The codebase includes an extensive automated test suite covering all core requirements:

1. **Event normalization**
2. **Event idempotency** (duplicate events ignored without error)
3. **Route resolution** (active vs inactive routes, filter evaluation)
4. **Delivery creation** (pending jobs enqueued for matched routes)
5. **Successful delivery** (worker claims job, updates status to delivered)
6. **Failed delivery** (worker records failure, schedules retry)
7. **Retry behavior** (exponential backoff, dead_letter transition after max retries)
8. **Multiple destinations** (one event dispatched to multiple targets)
9. **Destination isolation** (failure of one target does not block or affect another)
10. **Duplicate protection** (duplicate events do not generate duplicate deliveries)
11. **REST API handlers** (HTTP status codes, payload formatting, error handling)

### Run Tests Locally
```bash
go test -v ./...
```
Output:
```text
=== RUN   TestEventNormalization
--- PASS: TestEventNormalization (0.00s)
=== RUN   TestEventIdempotency
--- PASS: TestEventIdempotency (0.00s)
=== RUN   TestRouteResolution
--- PASS: TestRouteResolution (0.00s)
=== RUN   TestDeliveryCreation
--- PASS: TestDeliveryCreation (0.00s)
=== RUN   TestSuccessfulDelivery
--- PASS: TestSuccessfulDelivery (0.00s)
=== RUN   TestFailedDelivery
--- PASS: TestFailedDelivery (0.00s)
=== RUN   TestRetryBehaviorAndDeadLetter
--- PASS: TestRetryBehaviorAndDeadLetter (0.00s)
=== RUN   TestMultipleDestinations
--- PASS: TestMultipleDestinations (0.00s)
=== RUN   TestDestinationIsolation
--- PASS: TestDestinationIsolation (0.00s)
=== RUN   TestDuplicateEventsDoNotCreateDuplicateDeliveries
--- PASS: TestDuplicateEventsDoNotCreateDuplicateDeliveries (0.00s)
=== RUN   TestAPIEndpoints
--- PASS: TestAPIEndpoints (0.01s)
PASS
ok      automation-engine/tests 0.012s
```

---

## 13. License

MIT
