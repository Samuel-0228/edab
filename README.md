# Kerob — Production-Grade Event-Driven Automation Platform

> **Kerob** is an extensible, production-grade event-driven automation platform built in Go. It connects heterogeneous services, ingests normalized events, evaluates declarative workflows through safe condition and transformation engines, and reliably executes actions across connected targets.

```text
       ┌───────────────┐     ┌───────────────┐     ┌───────────────┐
       │   Telegram    │     │    Discord    │     │  RSS / Atom   │
       │(Bot & Scraper)│     │  (Webhooks)   │     │ (Feed Poller) │
       └───────┬───────┘     └───────┬───────┘     └───────┬───────┘
               │                     │                     │
               └───────────────┬─────┴─────────────────────┘
                               ▼
                    ┌─────────────────────┐
                    │    Kerob Engine     │
                    │  (Normalized Event) │
                    └──────────┬──────────┘
                               │
                WHEN Trigger Matches Criteria
                               │
               IF Conditions Evaluate to True
                               │
              THEN Transform Payload & Execute
                               │
               ┌───────────────┴───────────────┐
               ▼                               ▼
       ┌───────────────┐               ┌───────────────┐
       │   Telegram    │               │  Generic HTTP │
       │ (Verbatim Post│               │ (SSRF-Safe API│
       │    & Album)   │               │    Client)    │
       └───────────────┘               └───────────────┘
```

Kerob is designed around clean separation of concerns:
- **Core Engine**: Normalized events, AST condition engine, deterministic string templating, execution state machine, exponential backoff retries, and dead-letter failure queues.
- **Adapters & Integrations**: Decoupled integration model supporting Telegram, Discord, RSS, GitHub, Generic Inbound Webhooks, and Generic HTTP. Adding a new service is an adapter problem, not a core engine rewrite.
- **Security & Persistence**: AES-256-GCM encrypted credential store, proactive outbound SSRF defense, constant-time API token verification, and PostgreSQL domain persistence.
- **Observability & UI**: Real-time developer dashboard in dark technical aesthetic (black, dark gray, emerald accents), Prometheus `/metrics`, and granular step-by-step execution timeline auditing.

---

## 1. Supported Automations

Kerob easily powers workflows such as:
- **Telegram → Discord**: Monitor public Telegram channels and post formatted embed cards to Discord.
- **Telegram → Telegram**: Verbatim multi-channel and album mirroring without forwarded banners.
- **RSS → Telegram & Discord**: Poll tech news or blog feeds and distribute new articles to community channels.
- **GitHub → Discord**: Ingest repository events and notify engineering channels with issue details.
- **GitHub → Telegram**: Route pull request updates and deployment alerts to private channels.
- **Inbound Webhook → HTTP API**: Ingest external webhooks, transform JSON payloads, and forward to third-party REST APIs with SSRF protection.

---

## 2. Core Architectural Pillars

### 1. Declarative Workflow Architecture
Workflows are represented entirely as configuration and data:
```text
WHEN [Trigger Matches]
IF   [Conditions Evaluate to True]
THEN [Apply Transformations & Dispatch Actions]
```
- **Conditions**: Safe, deterministic AST evaluation (`equals`, `contains`, `starts_with`, `regex`, `greater_than`, `in`, `and/or/not`). Zero arbitrary code execution or eval.
- **Transformations**: Deterministic string templating (`{{event.payload.title}}`, `{{metadata.author | default: "System"}}`) and JSON field mapping.

### 2. Strict Deduplication & Idempotency
- **Event-Level**: Unique constraint on `(source_id, external_event_id)` ensures duplicate webhooks or messages are recorded once.
- **Execution-Level**: Unique constraint on `(workflow_id, event_id)` ensures that retried events or worker restarts never duplicate downstream action calls.

### 3. Proactive SSRF Protection
Outbound network requests (Generic HTTP Action, Webhooks, Discord, GitHub) are protected by a proactive multi-layer SSRF validator:
- Rejects loopback (`127.0.0.0/8`, `::1`), RFC1918 private subnets, link-local (`169.254.0.0/16`), and cloud metadata IP (`169.254.169.254`).
- Custom `net.Dialer` verifies all resolved IP addresses at connection dial time, defeating DNS rebinding attacks.
- Enforces safe HTTP redirect policies.

### 4. AES-256-GCM Credential Protection at Rest
- Sensitive bot tokens, webhook secrets, and API keys are encrypted at rest with AES-256-GCM using unique random nonces.
- Plaintext secrets are never returned in API responses, logs, or UI; all read operations return masked values (`••••••••1234`).

### 5. Granular Observability & Dead-Letter Queue
- Every execution records a step-by-step timeline: Trigger Ingestion → Condition Evaluation → Payload Transformation → Action Execution.
- Failed executions with exhausted retries are recorded in an inspectable Dead-Letter Queue with manual retry and discard controls.
- Real-time Prometheus metrics exposed at `/metrics`.

---

## 3. Quickstart with Docker Compose

### 1. Clone & Configure
```bash
git clone https://github.com/Samuel-0228/edab.git kerob
cd kerob
cp .env.example .env
```

`API_TOKEN` is optional in development. Optionally edit `.env` if you wish to configure custom credentials, ports, or integration tokens:
```env
# Optional in development; required in production
API_TOKEN=
KEROB_ENCRYPTION_KEY=
```

### 2. Launch Services (Development)
```bash
docker compose up -d
```
This spins up:
- **PostgreSQL 16** with automated database migrations applied at startup.
- **Kerob Engine & Dashboard** (`http://localhost:8080`).

### 3. Production Deployment
For production deployments, `API_TOKEN` (minimum 16 characters) is strictly mandatory:
```bash
export ENVIRONMENT=production
export API_TOKEN="your-strong-production-api-token"
export KEROB_ENCRYPTION_KEY="your-random-32-byte-secret-key!!"
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### 4. Open the Developer Dashboard
Open [http://localhost:8080](http://localhost:8080) in your browser:
- **Overview**: Real operational metrics, success rate, and active automations.
- **Automations**: Create, enable/disable, and inspect workflow definitions.
- **Integrations**: Catalog of Telegram, Discord, RSS, GitHub, Webhooks, and HTTP API.
- **Executions**: Inspect execution runs and view granular step timelines.
- **Dead-Letter**: Review unrecoverable failures, inspect errors, and retry.
- **Events**: Stream of normalized incoming events and payloads.

---

## 4. REST API Reference (v1)

When `API_TOKEN` is configured (or in production mode), send requests with `Authorization: Bearer <API_TOKEN>`. In development mode when `API_TOKEN` is unset, management endpoints can be accessed without authentication headers.

### Overview & Health
| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/health` | Liveness health check |
| `GET` | `/ready` | Readiness probe (database and worker pool ready) |
| `GET` | `/metrics` | Prometheus metrics format |
| `GET` | `/api/v1/overview` | Platform operational statistics and recent runs |

### Automations / Workflows
| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/workflows` | List all workflows |
| `POST` | `/api/v1/workflows` | Create a new workflow |
| `GET` | `/api/v1/workflows/{id}` | Get workflow definition |
| `PUT` | `/api/v1/workflows/{id}` | Replace workflow definition |
| `PATCH`| `/api/v1/workflows/{id}` | Update workflow name, description, or enabled state |
| `DELETE`| `/api/v1/workflows/{id}`| Delete workflow |

#### Example: Create a Workflow via API
```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Urgent Telegram Jobs to Discord",
    "enabled": true,
    "trigger": {
      "type": "telegram",
      "event_types": ["telegram.message"]
    },
    "conditions": {
      "field": "payload.text",
      "operator": "contains",
      "value": "hiring"
    },
    "transformations": {
      "type": "mapping",
      "field_maps": {
        "alert": "OPPORTUNITY: {{payload.text}}"
      }
    },
    "actions": [{
      "integration": "discord",
      "action_type": "send_message",
      "config": {
        "webhook_url": "https://discord.com/api/webhooks/...",
        "content": "{{payload.alert}}"
      }
    }]
  }'
```

### Inbound Generic Webhooks
| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/webhooks` | List configured inbound webhook endpoints |
| `POST` | `/api/v1/webhooks` | Create new webhook endpoint with secret token |
| `POST` | `/webhooks/{id}` | Ingest external webhook (secret validated via `X-Webhook-Token`) |

### Executions & Failures
| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/executions` | List execution history with filters |
| `GET` | `/api/v1/executions/{id}` | Get execution details and step timeline |
| `POST`| `/api/v1/executions/{id}/retry` | Reset execution for immediate retry |
| `GET` | `/api/v1/failures` | List dead-letter failures |
| `POST`| `/api/v1/failures/{id}/retry` | Retry failed dead-letter execution |
| `POST`| `/api/v1/failures/{id}/discard` | Discard dead-letter failure record |

---

## 5. Local Development & Testing

```bash
# 1. Format code according to Go standards
gofmt -w .

# 2. Run static analysis
go vet ./...

# 3. Run complete test suite with race detector
go test -v -count=1 -race ./...

# 4. Build statically-linked server binary
CGO_ENABLED=0 go build -ldflags="-w -s" -o kerob ./cmd/server
```

---

## 6. Project Documentation

- [Architecture Guide](docs/ARCHITECTURE.md): Detailed architectural design, domain models, and execution lifecycle.
- [Integration Developer Guide](docs/INTEGRATION_GUIDE.md): Guide on adding new trigger and action adapters.
- [OpenAPI Specification](docs/openapi.yaml): Full OpenAPI 3.0 API schema.
- [Changelog](CHANGELOG.md): Release version history.
- [Contributing Guidelines](CONTRIBUTING.md): PR rules, coding standards, and testing procedures.

---

## 7. License

Kerob is licensed under the MIT License. See [LICENSE](LICENSE) for details.
