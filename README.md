# Kerob — Production-Grade Event-Driven Automation Platform

[![CI Pipeline](https://github.com/Samuel-0228/edab/actions/workflows/ci.yml/badge.svg)](https://github.com/Samuel-0228/edab/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Samuel-0228/edab?include_prereleases&style=flat-square)](https://github.com/Samuel-0228/edab/releases)
[![Go Version](https://img.shields.io/badge/go-1.26.10-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Svelte](https://img.shields.io/badge/svelte-5.x-FF3E00?style=flat-square&logo=svelte)](https://svelte.dev/)
[![Coverage](https://img.shields.io/badge/coverage-65.4%25-brightgreen?style=flat-square)](Makefile)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](LICENSE)

> **Kerob** is an extensible, production-grade event-driven automation platform built in Go. It connects heterogeneous services, ingests normalized events, evaluates declarative workflows through safe condition AST and deterministic transformation engines, and reliably dispatches actions across connected targets with end-to-end idempotency, exponential backoff retries, and dead-letter queues.

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

---

## Table of Contents

- [Core Capabilities](#core-capabilities)
- [Quick Start Guide](#quick-start-guide)
  - [Step 1: Download or Clone](#step-1-download-or-clone)
  - [Step 2: Configure Environment](#step-2-configure-environment)
  - [Step 3: Start the Engine](#step-3-start-the-engine)
  - [Step 4: Create Your First Automation](#step-4-create-your-first-automation)
  - [Step 5: Trigger & Observe Workflow](#step-5-trigger--observe-workflow)
- [Installation Options](#installation-options)
  - [Option A: Pre-compiled Binaries (Recommended)](#option-a-pre-compiled-binaries-recommended)
  - [Option B: Docker Compose](#option-b-docker-compose)
  - [Option C: Build from Source](#option-c-build-from-source)
- [Operations Dashboard](#operations-dashboard)
- [Supported Integrations](#supported-integrations)
- [Configuration Reference](#configuration-reference)
- [Database Architecture & Migrations](#database-architecture--migrations)
- [Local Development & Testing](#local-development--testing)
- [Production Deployment Hardening](#production-deployment-hardening)
- [Upgrade & Migration Notes](#upgrade--migration-notes)
- [Security & License](#security--license)

---

## Core Capabilities

- **Declarative Workflow Architecture**: Define automation pipelines as structured data (`WHEN [trigger] IF [conditions] TRANSFORM THEN [actions]`). Safe condition AST with zero arbitrary code execution (`equals`, `contains`, `matches`, `greater_than`, `in`, `and/or/not`).
- **Deterministic Templating**: Contextual JSON navigation (`{{event.payload.title}}`, `{{metadata.author | default: "System"}}`) without side effects.
- **Strict Idempotency & Deduplication**:
  - Event-level uniqueness on `(source_id, external_event_id)` avoids duplicate ingestion.
  - Execution-level uniqueness on `(workflow_id, event_id)` prevents duplicate action execution across restarts.
- **Proactive Outbound SSRF Defense**: All outbound requests (webhooks, HTTP actions, Discord, GitHub) route through a custom socket dialer that blocks loopback (`127.0.0.0/8`, `::1`), RFC1918 private subnets, link-local, cloud metadata (`169.254.169.254`), and defeats DNS rebinding.
- **AES-256-GCM Vault at Rest**: Third-party integration tokens, secrets, and API keys are encrypted at rest with AES-256-GCM using unique random nonces. Sensitive tokens are never rendered in logs or API responses (`••••••••1234`).
- **Resilient Execution Lifecycle**: Exponential backoff retry engine with configurable jitter, execution timeline tracking, and inspectable Dead-Letter Queue with manual retry and discard controls.
- **Embedded Control Plane**: Modern, high-density Svelte 5 + TypeScript dashboard embedded directly inside the compiled Go binary. Zero separate frontend server needed.
- **Prometheus Metrics & Probes**: Liveness (`/health`), readiness probe (`/ready`), and standard Prometheus format metrics at `/metrics`.

---

## Quick Start Guide

Go from zero to a running automation pipeline in 5 steps:

### Step 1: Download or Clone

#### Download Pre-compiled Binary
Download the pre-compiled binary for your architecture from the [GitHub Releases](https://github.com/Samuel-0228/edab/releases):

```bash
# Example for Linux (x86_64)
curl -sL https://github.com/Samuel-0228/edab/releases/download/v0.1.1/kerob-v0.1.1-linux-amd64.tar.gz | tar -xz
cd kerob-v0.1.1-linux-amd64
```

#### Or Clone Repository
```bash
git clone https://github.com/Samuel-0228/edab.git kerob
cd kerob
```

### Step 2: Configure Environment

Copy the example configuration:

```bash
cp .env.example .env
```

For development, default settings work out of the box with an existing PostgreSQL instance:
```env
DATABASE_URL=postgres://kerob:kerob_password@localhost:5432/kerob?sslmode=disable
PORT=8080
ENVIRONMENT=development
```

### Step 3: Start the Engine

#### Via Docker Compose (Quickest Full Stack)
```bash
docker compose up -d
```
This automatically boots PostgreSQL 16 and Kerob with migrations applied.

#### Via Standalone Binary
```bash
./kerob
```

Open [http://localhost:8080](http://localhost:8080) to access the operations dashboard.

### Step 4: Create Your First Automation

Create a webhook-triggered workflow that transforms incoming JSON data and prints or dispatches an action.

#### 1. Create an Inbound Webhook Endpoint
```bash
curl -X POST http://localhost:8080/api/v1/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GitHub CI Webhook",
    "secret_token": "my-secret-webhook-token",
    "rate_limit_per_minute": 60
  }'
```
Response contains the webhook endpoint details:
```json
{
  "id": "7fa84f7e-d309-4821-ba2a-f8fa266f83ab",
  "name": "GitHub CI Webhook",
  "secret_token": "my-secret-webhook-token"
}
```

#### 2. Create the Declarative Workflow Rule
```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Deploy Alert on Build Failure",
    "enabled": true,
    "trigger": {
      "type": "webhook",
      "event_types": ["webhook.received"]
    },
    "conditions": {
      "field": "payload.status",
      "operator": "equals",
      "value": "failed"
    },
    "transformations": {
      "type": "mapping",
      "field_maps": {
        "alert_text": "CI Failure in repo {{payload.repository}}: {{payload.commit_message}}"
      }
    },
    "actions": [{
      "integration": "http",
      "action_type": "request",
      "config": {
        "url": "https://httpbin.org/post",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json"
        },
        "body_template": "{\"message\": \"{{payload.alert_text}}\"}"
      }
    }]
  }'
```

### Step 5: Trigger & Observe Workflow

Trigger the webhook endpoint:
```bash
curl -X POST http://localhost:8080/webhooks/7fa84f7e-d309-4821-ba2a-f8fa266f83ab \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Token: my-secret-webhook-token" \
  -d '{
    "repository": "kerob-engine",
    "status": "failed",
    "commit_message": "Fix race condition in poller"
  }'
```

Verify the execution timeline in the UI ([http://localhost:8080](http://localhost:8080)) under **Executions**, or via the API:
```bash
curl http://localhost:8080/api/v1/executions
```

---

## Installation Options

### Option A: Pre-compiled Binaries (Recommended)

Self-contained, statically linked executables are published with every release. They include the complete backend and embedded dashboard with zero external runtime requirements other than a PostgreSQL database:

| Platform / Architecture | Download Archive |
| :--- | :--- |
| **Linux (x86_64 / amd64)** | `kerob-v0.1.1-linux-amd64.tar.gz` |
| **Linux (ARM64 / aarch64)** | `kerob-v0.1.1-linux-arm64.tar.gz` |
| **macOS (Apple Silicon / M1 / M2 / M3)** | `kerob-v0.1.1-darwin-arm64.tar.gz` |
| **macOS (Intel / x86_64)** | `kerob-v0.1.1-darwin-amd64.tar.gz` |
| **Windows (x86_64)** | `kerob-v0.1.1-windows-amd64.zip` |

Verify checksums:
```bash
sha256sum -c checksums.txt --ignore-missing
```

Query binary version:
```bash
./kerob --version
# Kerob Automation Engine v0.1.1 (commit: c4bddd5, built: 2026-10-30T17:54:00Z)
```

### Option B: Docker Compose

The standard Docker Compose configuration provisions both the database and the automation engine:

```bash
# 1. Start development stack
docker compose up -d

# 2. View streaming logs
docker compose logs -f engine

# 3. Stop containers
docker compose down
```

For production deployments, enforce authentication and encryption keys:
```bash
export ENVIRONMENT=production
export API_TOKEN="your-secure-production-api-token-minimum-16-chars"
export KEROB_ENCRYPTION_KEY="your-random-32-byte-secret-encryption-key!"
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### Option C: Build from Source

Requirements: Go 1.23+ and Node.js 20+ (only if modifying frontend).

```bash
# 1. Clone repository
git clone https://github.com/Samuel-0228/edab.git kerob
cd kerob

# 2. Compile standalone binary (includes embedded dashboard)
make build

# 3. Binary output is available at bin/kerob
./bin/kerob --version
```

If you modify files inside `frontend/src/`, recompile the embedded bundle:
```bash
make build-frontend
make build
```

---

## Operations Dashboard

Kerob features an embedded single-page control plane built with Svelte 5 and TypeScript. It is served directly from `/` and `/ui`:

- **Overview (`/`)**: Real-time operational metrics, uptime, active automations, success rates, and integration catalog.
- **Automations (`/workflows`)**: Declarative builder to create, edit, enable/disable, test, and delete workflows.
- **Schedules (`/schedules`)**: Periodic cron tasks (`*/5 * * * *`) and interval execution triggers.
- **Integrations (`/integrations`)**: Catalog of supported connectors and active configurations.
- **Credentials Vault (`/credentials`)**: AES-256-GCM encrypted token storage for external authorization.
- **Executions (`/executions`)**: Granular lifecycle audit log and step-by-step millisecond execution timing.
- **Dead-Letter Queue (`/failures`)**: Inspect unrecoverable executions, error stack traces, and trigger retries or discards.
- **Inbound Endpoints (`/webhooks`)**: Manage HTTP webhook ingestion endpoints with secret token authorization.
- **Normalized Events (`/events`)**: Audit stream of ingested event payloads.
- **Live Logs (`/logs`)**: High-density structured logs with severity filtering.
- **System Health (`/health`)**: Liveness probe, readiness state, active SSRF policy status, and Prometheus metrics.

---

## Supported Integrations

| Integration | Type | Capabilities & Notes |
| :--- | :--- | :--- |
| **Generic Inbound Webhooks** | Trigger | Ingest JSON payloads via `/webhooks/{id}` with secret token validation (`X-Webhook-Token`) and 1MB limit. |
| **Schedule / Cron** | Trigger | Standard 5-field cron syntax (`* * * * *`) and interval durations (`10s`, `5m`, `1h`). |
| **Telegram Bot & Scraper** | Trigger & Action | Bot polling, public channel web scraper (`t.me/s/<channel>`), media-group album buffering, rate limiting, and message posting. |
| **Discord** | Action | Send markdown text or rich embed cards to Discord webhooks with SSRF defense. |
| **GitHub** | Trigger & Action | Inbound webhooks with HMAC-SHA256 signature verification (`X-Hub-Signature-256`); issue creation action. |
| **RSS / Atom** | Trigger | Periodic feed polling with GUID and link deduplication. |
| **Generic HTTP API** | Action | Make arbitrary HTTP requests (GET, POST, PUT, PATCH, DELETE) with body templating and SSRF protection. |

---

## Configuration Reference

All settings are configured via environment variables or a `.env` file:

| Variable | Default | Required in Production | Description |
| :--- | :--- | :---: | :--- |
| `DATABASE_URL` | `postgres://localhost:5432/kerob` | **Yes** | PostgreSQL connection string (PostgreSQL 15+). |
| `PORT` | `8080` | No | HTTP server listening port. |
| `ENVIRONMENT` | `development` | No | Set to `production` to enforce security rules. |
| `API_TOKEN` | *(empty)* | **Yes** (≥16 chars) | Bearer token for Management API. Enforced in production. |
| `KEROB_ENCRYPTION_KEY` | *(default dev key)* | **Yes** (32 bytes) | AES-256-GCM secret key for encrypting credentials at rest. |
| `LOG_LEVEL` | `info` | No | Logging level: `debug`, `info`, `warn`, `error`. |
| `WORKER_CONCURRENCY` | `4` | No | Number of parallel delivery worker routines. |
| `WORKER_POLL_INTERVAL` | `1s` | No | Polling cadence for pending queue delivery jobs. |
| `MAX_RETRIES` | `3` | No | Maximum retry attempts before routing to Dead-Letter Queue. |
| `INITIAL_BACKOFF` | `2s` | No | Base exponential backoff duration for failed executions. |
| `MAX_BACKOFF` | `1m` | No | Ceiling duration for exponential retry backoff. |
| `SCHEDULER_INTERVAL` | `10s` | No | Cadence at which the schedule engine evaluates cron rules. |
| `TELEGRAM_ENABLED` | `false` | No | Enable Telegram bot polling and listening. |
| `TELEGRAM_BOT_TOKEN` | *(empty)* | Optional | Telegram Bot API token from `@BotFather`. |
| `TELEGRAM_RATE_GLOBAL` | `25` | No | Global message send rate limit per second. |
| `TELEGRAM_RATE_PER_CHAT` | `1` | No | Per-chat message send rate limit per second. |
| `TELEGRAM_PUBLIC_POLL_INTERVAL` | `15s` | No | Frequency for scraping public Telegram channels. |

---

## Database Architecture & Migrations

Kerob maintains strict schema integrity using automated database migrations stored in `migrations/` and embedded directly into the binary via `migrations/migrations.go`.

### Startup Migration Execution
When Kerob starts, it verifies the existence of the `schema_migrations` table and applies all pending `.up.sql` scripts in transactional order:

1. `000001_init.up.sql`: Base event bus schema (`sources`, `destinations`, `routes`, `events`, `delivery_jobs`, `telegram_update_state`, `public_channel_state`).
2. `000002_workflows_and_integrations.up.sql`: Automation engine schema (`credentials`, `workflows`, `executions`, `execution_steps`, `dead_letter`, `webhooks`).

### Idempotency Constraints
- **Event Deduplication**: `UNIQUE(source_id, external_event_id)` ensures duplicate webhook payloads or Telegram updates are never recorded twice.
- **Workflow Idempotency**: `UNIQUE(workflow_id, event_id)` ensures retried events never trigger duplicate action execution.

---

## Local Development & Testing

Kerob is engineered for complete offline testability. All core domain logic, workflows, conditions, transformations, and API endpoints run against **thread-safe in-memory stores without requiring PostgreSQL, Docker, or external network access**.

### Standard Test Commands

```bash
# 1. Run unit tests only (fast, pure memory mocks)
make test-unit

# 2. Run integration tests (in-memory multi-component pipelines)
make test-integration

# 3. Run end-to-end lifecycle and dead-letter retry tests
make test-e2e

# 4. Run Docker configuration validation (skips cleanly if Docker is unavailable)
make test-docker

# 5. Run frontend component tests (Vitest + JSDOM)
make test-frontend

# 6. Enforce Go test coverage threshold (minimum >=60.0% required)
make test-coverage

# 7. Enforce frontend test coverage threshold (minimum >=65.0% required)
make test-coverage-frontend

# 8. Run all test suites together with coverage enforcement
make test-all
```

### Dependency Health & Auditing
```bash
# Run full dependency health audit (integrity, audits, freshness)
make deps

# Verify module integrity and lockfile synchronization
make deps-verify

# Blocking vulnerability scans (govulncheck + npm audit)
make deps-audit

# Enforce freshness policies defined in .dependency-policy.json
make deps-freshness
```

---

## Production Deployment Hardening

When deploying Kerob to production environments, apply the following practices:

1. **Enforce `ENVIRONMENT=production`**:
   The engine refuses to boot if `API_TOKEN` is unset, shorter than 16 characters, or contains known placeholder values (`changeme`, `admin`, `password`).
2. **Provide a 32-Byte `KEROB_ENCRYPTION_KEY`**:
   Generate a cryptographic secret key:
   ```bash
   openssl rand -hex 16
   ```
3. **Run as Non-Root**:
   The official `Dockerfile` creates and runs as an unprivileged user `kerob:kerob` (UID 1000).
4. **Reverse Proxy & TLS**:
   Terminate TLS using Caddy, Nginx, or Traefik in front of Kerob (`PORT=8080`). Pass `X-Forwarded-For` and `X-Real-IP` headers.
5. **SSRF Defense Configuration**:
   The internal SSRF dialer rejects all private network addresses. If your workflows need to call services behind an internal proxy, configure outbound webhook proxies rather than disabling host boundaries.

---

## Upgrade & Migration Notes

### Upgrading from v0.1.0 to v0.1.1
- **Automatic Schema Migrations**: No manual SQL execution required. Database migrations are applied automatically at binary startup.
- **Embedded Frontend**: The single-file Svelte dashboard is embedded directly into the Go executable. If upgrading from source, run `make build-frontend` before `make build`.
- **Pre-Release Binary Support**: Pre-compiled binaries for Linux, macOS, and Windows can be dropped directly into systemd services or container volumes.
- **CLI Flags**: Binaries now support `--version`, `-v`, and `version` commands.
- **Validation Layer**: Inbound API requests are validated prior to execution. Ensure external webhook clients send valid JSON according to the OpenAPI schema.

---

## Security & License

- **Security Policy**: Read our vulnerability disclosure process in [SECURITY.md](SECURITY.md).
- **Contributing**: Development workflows and PR standards are in [CONTRIBUTING.md](CONTRIBUTING.md).
- **Architecture**: In-depth subsystem specifications are in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
- **API Schema**: Full OpenAPI 3.0 specification is in [docs/openapi.yaml](docs/openapi.yaml).
- **License**: Kerob is open-source software licensed under the [MIT License](LICENSE).
