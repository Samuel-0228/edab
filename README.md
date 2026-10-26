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

### 4. Open the Kerob Control Dashboard
Open [http://localhost:8080](http://localhost:8080) (or `/ui`) in your browser to access the technical Svelte 5 + TypeScript control plane:
- **Overview**: Real operational metrics, success rate, and active automations.
- **Automations**: Create, edit, enable/disable, test, and delete workflow definitions.
- **Schedules**: Periodic cron routines and interval execution controller.
- **Integrations**: Catalog of Telegram, Discord, RSS, GitHub, Webhooks, and HTTP API.
- **Executions**: Audit logs and granular step-by-step lifecycle outputs.
- **Dead-Letter**: Review unrecoverable failures, inspect errors, and retry/discard.
- **Events**: Stream of normalized incoming events and payloads.
- **Inbound Endpoints**: Create and manage HTTP webhook ingestion endpoints.
- **Credentials Vault**: AES-256-GCM encrypted tokens for external API authorizations.
- **Live Logs**: Terminal-style structured execution, event, and failure log stream.
- **System Health**: Health probes, Prometheus metrics gauges, and SSRF policies.
- **Settings**: Bearer API token configuration and connection diagnostics.

#### Frontend Development & Build
The frontend is a dedicated Svelte 5 + TypeScript application located in `frontend/`:
```bash
# Run tests
cd frontend && npm test

# Typecheck
npm run check

# Dev server with proxy to backend
npm run dev

# Production build (bundles and updates embedded backend dashboard)
npm run build
```

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

Kerob is designed for genuine testability immediately after a fresh clone **without requiring Docker, PostgreSQL, Telegram, Discord, GitHub, or any external services** for the standard test suites.

### Testing Without External Services (Fresh Clone)

On any machine with Go 1.23+ installed, all core tests run out-of-the-box using lightweight in-memory storage implementations and dependency injection:

```bash
# Download dependencies
go mod download

# Run the complete offline test suite with race detector
go test ./...
```

### Make Test Targets

Kerob provides standard `make` targets to separate test tiers:

```bash
# 1. Run unit tests only (fast, completely isolated, no external dependencies)
make test-unit

# 2. Run integration tests (in-memory multi-component pipelines & API server)
make test-integration

# 3. Run end-to-end lifecycle, failure, and dead-letter retry tests
make test-e2e

# 4. Run Docker Compose validation tests (cleanly skipped if Docker is not installed)
make test-docker

# 5. Run frontend unit and component tests (Vitest + JSDOM, no backend needed)
make test-frontend

# 6. Run all test tiers together
make test-all
```

### Infrastructure Requirements & Test Matrix

| Test Suite | Command | External Infrastructure Required? | Behavior Without Infrastructure |
| :--- | :--- | :--- | :--- |
| **Unit Tests** | `make test-unit` | **None** (pure in-memory stores & mocks) | Always passes |
| **Integration Tests** | `make test-integration` | **None** (in-memory pipeline & HTTP test servers) | Always passes |
| **E2E Automation Tests** | `make test-e2e` | **None** (in-memory engine & action spies) | Always passes |
| **Docker / Compose Tests** | `make test-docker` | **Docker CLI & Compose** | **Skips cleanly** (`t.Skip`) if Docker unavailable |
| **Frontend Tests** | `make test-frontend` | **Node.js 20+** (mocked fetch in JSDOM) | Always passes independently without backend |
| **Standard Suite** | `go test ./...` | **None** | Always passes on a clean clone |

### Storage & Dependency Injection

The platform defines the `workflow.Store`, `workflow.ExecutionStore`, and `workflow.DeadLetterStore` interfaces:
- **`MemoryStore`**: Thread-safe in-memory store with full deep-copy isolation and CRUD operations, used in all unit, integration, and E2E test suites without touching a database.
- **`PostgresStore`**: Production PostgreSQL implementation utilized when running the live server binary or docker containers.

### Building & Code Quality

```bash
# Format Go code
gofmt -w .

# Run static analysis
go vet ./...

# Build statically-linked server binary
make build
# or: CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/kerob ./cmd/server
```

### Dependency Health & Maintenance

Kerob enforces a strict, reproducible dependency maintenance process across both Go and frontend components. Dependency freshness and security audits are blocking in CI (`dependency-health` job) and fail if dependencies require attention.

```bash
# Run complete dependency health verification (integrity, audits, freshness)
make deps

# Verify module hashes and lockfile synchronization
make deps-verify

# Run blocking vulnerability audits (govulncheck + npm audit)
make deps-audit

# Enforce Go and frontend dependency freshness policies
make deps-freshness

# Standalone cross-platform script
./scripts/check-dependencies.sh
```

See [Dependency Policy & Maintenance Guide](docs/DEPENDENCY_POLICY.md) for full policy specifications, framework compatibility boundaries, and Dependabot automation details.

---

## 6. Project Documentation

- [Architecture Guide](docs/ARCHITECTURE.md): Detailed architectural design, domain models, and execution lifecycle.
- [Dependency Health Policy](docs/DEPENDENCY_POLICY.md): Enforced dependency security, freshness, and audit process.
- [Integration Developer Guide](docs/INTEGRATION_GUIDE.md): Guide on adding new trigger and action adapters.
- [OpenAPI Specification](docs/openapi.yaml): Full OpenAPI 3.0 API schema.
- [Changelog](CHANGELOG.md): Release version history.
- [Contributing Guidelines](CONTRIBUTING.md): PR rules, coding standards, and testing procedures.

---

## 7. License

Kerob is licensed under the MIT License. See [LICENSE](LICENSE) for details.
