# Kerob Architecture Guide

This document describes the architectural design, execution pipeline, data model, and security mechanisms of **Kerob**, an event-driven automation platform.

---

## 1. High-Level System Architecture

Kerob connects heterogeneous services, listens for events, evaluates declarative rules, transforms payloads deterministically, and dispatches actions across destinations.

```text
 ┌──────────────────────────────────────────────────────────────────┐
 │                         Event Ingestion                          │
 │  Telegram Bot • Public Scraper • Discord • RSS • GitHub • Webhooks│
 └────────────────────────────────┬─────────────────────────────────┘
                                  │ Raw Event
                                  ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │                      Normalization Layer                         │
 │     Standardized Event: ID, SourceID, EventType, Payload, Meta   │
 └────────────────────────────────┬─────────────────────────────────┘
                                  │
                                  ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │                   Idempotency Gate (PostgreSQL)                  │
 │          uq_events_source_external: Deduplicates updates         │
 └────────────────────────────────┬─────────────────────────────────┘
                                  │
                                  ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │                        Workflow Engine                           │
 │     WHEN [Trigger Matches]                                       │
 │       ↓                                                          │
 │     IF [Conditions Evaluator (Safe AST)]                         │
 │       ↓                                                          │
 │     THEN [Transformations Engine (Deterministic Templates)]      │
 │       ↓                                                          │
 │     EXECUTE [Action Step 1 → Action Step 2]                      │
 └────────────────────────────────┬─────────────────────────────────┘
                                  │
                                  ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │                      Action Adapters Layer                       │
 │  Telegram API • Discord Webhook • GitHub Client • HTTP (SSRF Safe)│
 └────────────────────────────────┬─────────────────────────────────┘
                                  │
                                  ▼
 ┌──────────────────────────────────────────────────────────────────┐
 │                    Observability & Persistence                   │
 │   Executions • Granular Steps Timeline • Dead-Letter Queue       │
 │   Prometheus /metrics • Structured slog Logs • Developer UI      │
 └──────────────────────────────────────────────────────────────────┘
```

---

## 2. Core Domain Model

### 1. Event
The normalized representation of external updates.
- `id`: Unique UUID generated upon arrival.
- `source_id`: Identifier of the source producer (e.g. webhook ID, channel ID).
- `external_event_id`: Service-level unique identifier (e.g. `telegram:-100123:45`, `github:issue:101`).
- `event_type`: Domain dot-notation classification (e.g. `telegram.message`, `github.issue.opened`, `rss.item`).
- `payload`: Normalized JSON body containing actual event data.
- `metadata`: Service-specific context (headers, user agent, timestamps).

### 2. Workflow
Declarative automation rule:
- `trigger`: Configures source ID, integration type, and matched event types.
- `conditions`: Recursive condition tree evaluated against normalized event data.
- `transformations`: Field mappings, string templates, or structured JSON transforms.
- `actions`: Sequential action steps targeting connected integrations.
- `max_retries`: Maximum execution retry attempts on transient failure (default: 3).
- `timeout_seconds`: Action timeout in seconds (default: 30).

### 3. Execution & ExecutionStep
Audit trail recording the lifecycle of an automation run:
- **Status states**: `pending` → `running` → `success` | `failed` | `retrying` | `cancelled`.
- **Step Types**:
  - `trigger`: Recorded upon initial event ingestion.
  - `condition`: Records whether rule conditions passed or skipped.
  - `transformation`: Records transformed data output.
  - `action`: Records individual action execution results, external message/issue IDs, and durations.

### 4. Dead-Letter Failures
Unrecoverable errors where retries have been exhausted:
- Stores execution ID, workflow ID, event ID, error string, and raw payload.
- Operator actions: **Retry** (resets execution to pending with fresh attempt count) or **Discard**.

---

## 3. Condition Evaluation Engine

The condition engine in `internal/core/conditions` implements safe, deterministic AST evaluation without running unrestricted interpreted code.

Supported operators:
- `equals`, `not_equals`: Exact and type-coerced equality.
- `contains`, `not_contains`: Case-insensitive substring matching or array element inclusion.
- `starts_with`, `ends_with`: String prefix and suffix matching.
- `matches`: Safe regular expression matching bounded to 256 characters (preventing ReDoS).
- `exists`, `not_exists`: Property existence check.
- `greater_than`, `less_than`, `greater_or_equal`, `less_or_equal`: Numeric comparison.
- `in`, `not_in`: Membership check in list of values.

Compound expressions:
- `and`: All child rules must evaluate to true.
- `or`: At least one child rule must evaluate to true.
- `not`: Inverts child rule outcome.

---

## 4. Deterministic Transformation Engine

Located in `internal/core/transformations`, the transformation engine provides string templating and field mapping without arbitrary script evaluation.

Example template:
```text
URGENT ALERT: {{event.payload.title}} (by {{event.payload.author | default: "System"}})
Target URL: {{event.payload.url}}
```

Capabilities:
- Dot-notation path navigation (`payload.user.login`, `metadata.repo`).
- Fallback defaults (`{{payload.title | default: "Untitled"}}` or `{{payload.title || "Untitled"}}`).
- Recursive JSON template interpolation: scans arbitrary JSON payloads and renders nested template strings.

---

## 5. Security & SSRF Protection Architecture

Kerob protects external network boundaries through proactive security controls:

### 1. Outbound SSRF Defense
All outbound HTTP calls (Generic HTTP Action, Webhooks, Discord Webhooks, GitHub API) use `internal/adapters/ssrf`:
- **Protocol verification**: Strictly enforces `http` or `https`.
- **Blocked Hostnames**: Rejects `localhost`, `*.localhost`, `metadata.google.internal`, and cloud metadata hosts.
- **Blocked IP Ranges**:
  - Loopback: `127.0.0.0/8`, `::1`
  - RFC1918 Private Ranges: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
  - Link-Local: `169.254.0.0/16`, `fe80::/10`
  - Cloud Metadata Endpoint: `169.254.169.254`
  - Multicast and Unspecified IPs.
- **DNS Rebinding Prevention**: Custom `net.Dialer` transport resolves hostnames at TCP dial time and verifies all resolved IP addresses before opening sockets.
- **Safe HTTP Redirects**: Follows redirects only after re-validating the target URL against SSRF filters.

### 2. Credential Encryption at Rest
Credentials stored via `internal/core/credentials` use AES-256-GCM authenticated encryption:
- Unique 12-byte random nonces generated per credential update.
- Plaintext secrets are never stored in the database.
- Plaintext secrets are never returned via API responses or logs; all read operations return masked values (`••••••••1234`).

### 3. API Authentication
When `API_TOKEN` is configured, all management routes require constant-time `Authorization: Bearer <API_TOKEN>` validation, defeating timing attacks.
