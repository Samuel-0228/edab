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

---

## 6. API Boundary Validation Layer

Untrusted request payloads are strictly validated in `internal/api/validation.go` before reaching the core domain or database operations.

### Validation Principles
1. **Pre-Domain Execution**: Validation intercepts requests immediately after JSON decoding. Invalid payloads never reach stores or workflow engines.
2. **Consistent Structured 400 Responses**: Malformed requests return structured JSON with request correlation:
   ```json
   {
     "error": "validation failed: one or more fields are invalid",
     "code": "VALIDATION_ERROR",
     "request_id": "a5c0f188-7e77-4b68-9a48-5c4e1352e46b",
     "details": [
       {"field": "name", "message": "workflow name is required"},
       {"field": "trigger.type", "message": "unsupported trigger type \"unknown\""}
     ]
   }
   ```
3. **Validated Entities**:
   - **Workflows (`POST /api/v1/workflows`, `PUT /api/v1/workflows/{id}`)**: Name presence and length (<= 128 chars), supported trigger types (`webhook`, `telegram`, `github`, `rss`, `schedule`), cron expression validity, non-empty action list, required integration and action type, and action URL SSRF safety.
   - **Webhooks (`POST /api/v1/webhooks`, `PUT /api/v1/webhooks/{id}`)**: Name presence and length (<= 128 chars), non-negative rate limit.
   - **Credentials (`POST /api/v1/credentials`, `PUT /api/v1/credentials/{id}`)**: Name presence, supported integration (`discord`, `telegram`, `github`, `http`, `custom`), non-empty secret map.
   - **Sources & Destinations (`POST /sources`, `POST /destinations`)**: Name presence, valid registered adapter type names.
   - **Routes (`POST /routes`)**: Valid UUID format for `source_id` and `destination_id`.

---

## 7. Error Tracking & Production Observability

Kerob isolates error tracking behind a clean provider interface in `internal/core/errtracker`:

```go
type Provider interface {
    CaptureError(ctx context.Context, err error, scope Scope)
    Flush(timeout time.Duration)
}
```

### Key Capabilities
- **Scrubbing & Sanitization**: Proactively strips sensitive data from error messages, stack traces, and metadata tags before capture:
  - Bearer tokens (`Bearer [REDACTED]`)
  - GitHub personal access tokens (`[REDACTED_GITHUB_TOKEN]`)
  - Telegram bot tokens (`[REDACTED_TELEGRAM_TOKEN]`)
  - PostgreSQL connection strings (`postgres://user:[REDACTED]@host...`)
  - Sensitive scope keys (`password`, `secret`, `token`, `auth`, `credential`, `key`)
- **Structured Error Context**: Captures `request_id`, `workflow_id`, `execution_id`, `event_id`, and step metadata.
- **Provider Implementations**:
  - `NoopProvider`: Disables error reporting.
  - `LogProvider`: Formats sanitized errors into structured `slog` error records.
  - `MemoryProvider`: In-memory recording for unit and integration testing assertions.
  - `SentryProvider`: Production integration with Sentry via standard HTTP ingestion without coupling the core engine to third-party SDKs.

---

## 8. Distributed Execution Foundation & Durable Job Leases

Kerob supports horizontal scaling where multiple stateless server nodes and worker instances process workflow executions and delivery jobs concurrently against a shared PostgreSQL database.

```text
 ┌────────────────────────────────────────────────────────┐
 │                   PostgreSQL Database                  │
 │  • executions: locked_by, lease_expires_at, SKIP LOCKED│
 │  • delivery_jobs: locked_by, lease_expires_at          │
 └─────────────┬───────────────────────────┬──────────────┘
               │                           │
       FOR UPDATE SKIP LOCKED      FOR UPDATE SKIP LOCKED
               │                           │
               ▼                           ▼
 ┌───────────────────────────┐ ┌───────────────────────────┐
 │       Worker Node 1       │ │       Worker Node 2       │
 │  ID: worker-node-1        │ │  ID: worker-node-2        │
 │  • Workflow Concurrency=4 │ │  • Workflow Concurrency=4 │
 │  • Delivery Concurrency=4 │ │  • Delivery Concurrency=4 │
 │  • Periodic Heartbeats    │ │  • Periodic Heartbeats    │
 └───────────────────────────┘ └───────────────────────────┘
```

### 1. Durable Leases & Zero Duplicate Claims
Each claim operation uses atomic row locking with `FOR UPDATE SKIP LOCKED` over `executions` and `delivery_jobs`:
- **Lock Owner**: Recorded in `locked_by` (configured `WORKER_ID` or auto-generated `<hostname>-<uuid8>`).
- **Lease Duration**: Configurable via `WORKER_LEASE_DURATION` (default: 30s).
- **Atomic State Transition**: Rows transitioning to `running` or `processing` claim the lease in a single atomic SQL transaction. Other workers immediately skip locked rows without lock contention or deadlocks.

```sql
UPDATE executions
SET status = 'running',
    locked_by = $1,
    lease_expires_at = NOW() + ($2 || ' seconds')::interval,
    updated_at = NOW()
WHERE id = (
    SELECT id FROM executions
    WHERE (
        (status IN ('pending', 'retrying', 'waiting') AND next_attempt_at <= NOW())
        OR
        (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at <= NOW())
    )
    ORDER BY priority DESC, next_attempt_at ASC, created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING ...;
```

### 2. Renewal Heartbeats & Safe Crash Takeover
- **Heartbeat Goroutine**: Active jobs/executions run background heartbeats every `WORKER_HEARTBEAT_INTERVAL` (default: 10s, recommended 1/3 of lease duration) to extend `lease_expires_at = NOW() + lease_duration`.
- **Safe Crash Takeover**: If a worker node crashes, terminates abruptly (OOM/SIGKILL), or loses database connectivity, its lease expires automatically. Any available worker will safely claim the expired row on its next polling cycle.
- **Lease Fencing**: If a worker takes longer than its lease duration and another worker takes over the lease, renewals by the zombie worker fail (`"execution lease lost or execution not running"`), preventing split-brain writes.

### 3. Delayed, Retrying, and Scheduled Jobs Recovery
- **Delayed Steps (`waiting`)**: Paused workflows with scheduled resumption (`StepTypeDelay`) store `next_attempt_at` in the future without holding database connections or worker goroutines.
- **Exponential Backoff (`retrying`)**: Failed workflows calculate backoff and update `next_attempt_at`. Once `next_attempt_at <= NOW()`, the execution is eligible to be claimed by any available worker.
- **Scheduled Triggers (`schedule`)**: Scheduler triggers use deterministic time-window bucketing (e.g. truncated to the minute/interval) when generating events, ensuring multi-node schedulers generate the exact same idempotent event key (`schedule:<workflow_id>:<bucket_timestamp>`), deduplicated by PostgreSQL's unique event constraint.
- **Zero-Loss Restart Recovery**: If a server or worker restarts, all pending, waiting, retrying, or uncompleted jobs persist in PostgreSQL and immediately resume when due.

### 4. Worker Concurrency & Graceful Shutdown
- **Worker Pools**: Both `delivery.Worker` and `workflow.Worker` support configurable worker pool concurrency (`WORKER_CONCURRENCY` and `WORKFLOW_WORKER_CONCURRENCY`).
- **Graceful Draining**: On `SIGTERM` or `SIGINT`, workers stop polling for new tasks, cancel the polling context, release clean idle leases, and wait on a `sync.WaitGroup` for active in-flight jobs to complete cleanly before process exit.

### 5. Prometheus Metrics & Structured Logs
Distributed execution operations are tracked in real-time via `/metrics`:
- `kerob_leases_acquired_total`: Number of job or execution leases successfully acquired.
- `kerob_leases_renewed_total`: Number of active lease renewals via heartbeats.
- `kerob_leases_taken_over_total`: Number of expired leases taken over after worker crash.
- `kerob_leases_released_total`: Leases voluntarily released upon completion or shutdown.
- `kerob_lease_losses_total`: Count of lease losses detected during heartbeats.

---

## 9. Multi-Tenancy, Role-Based Access Control (RBAC), and Audit Logging

Kerob provides enterprise multi-tenancy and granular role-based access control (RBAC), isolating tenant data at the database and service layers while supporting scoped machine tokens and an immutable, secret-scrubbed audit trail.

```text
 ┌────────────────────────────────────────────────────────────────────────┐
 │                      Tenant: "acme-corp" (ID: org_1)                   │
 │                                                                        │
 │  ┌────────────────────────────────┐   ┌─────────────────────────────┐  │
 │  │      Users & Memberships       │   │    Scoped API Tokens        │  │
 │  │  • Alice   (Owner)             │   │  • krb_ci_...   [workflows:*│  │
 │  │  • Bob     (Admin)             │   │  • krb_ops_...  [events:rep]│  │
 │  │  • Charlie (Editor)            │   └──────────────┬──────────────┘  │
 │  │  • Dave    (Operator)          │                  │                 │
 │  │  • Eve     (Viewer)            │                  ▼                 │
 │  └────────────────────────────────┘      RBAC Middleware Gate          │
 │                                             (Enforces Role & Scopes)   │
 │                                                      │                 │
 │  ┌───────────────────────────────────────────────────▼──────────────┐  │
 │  │                   Isolated Tenant Resources                      │  │
 │  │  Workflows • Credentials • Webhooks • Events • Executions • DLQ  │  │
 │  └───────────────────────────────────────────────────┬──────────────┘  │
 │                                                      │                 │
 │                                                      ▼                 │
 │  ┌──────────────────────────────────────────────────────────────────┐  │
 │  │              Immutable Audit Trail (Secret Scrubbed)             │  │
 │  │  action, actor_id, resource_id, scrubbed_details, ip_address    │  │
 │  └──────────────────────────────────────────────────────────────────┘  │
 └────────────────────────────────────────────────────────────────────────┘
```

### 1. Tenant Data Isolation Model
- **Tenants & Organizations**: Customers or organizational units are represented by `Tenant` records (`tenants` table).
- **Resource Ownership**: Every resource (`workflows`, `credentials`, `webhooks`, `events`, `executions`, `dead_letter`, `sources`, `destinations`, `routes`, `delivery_jobs`) includes `tenant_id VARCHAR(64)` and optional `project_id VARCHAR(64)` foreign keys with indexed lookups (`idx_<resource>_tenant`).
- **Database & Service Layer Enforcement**: All database queries filter by `tenant_id`. Handlers strictly verify tenant matching. Any attempt by an identity from Tenant A to inspect, alter, execute, or delete a resource owned by Tenant B is immediately rejected with HTTP 403 Forbidden and error code `CROSS_TENANT_DENIED`.

### 2. Role-Based Access Control (RBAC) Hierarchy
Kerob defines five standard hierarchical roles (`owner`, `admin`, `editor`, `operator`, `viewer`):

| Role | Rank | Allowed Operations | Typical Use Case |
| :--- | :--- | :--- | :--- |
| **`owner`** | 5 | All tenant capabilities: manage organization, delete/transfer tenant, invite/remove members, manage API tokens, create/edit workflows, execute tests, retry runs, read secrets, view audit logs. | Company Founder, Platform Lead |
| **`admin`** | 4 | All resource mutations, test executions, retries, and team management: invite members, update roles, create and revoke API tokens. Cannot delete the tenant organization. | DevOps Lead, Team Admin |
| **`editor`** | 3 | Create, edit, patch, and delete workflows, credentials, webhooks, sources, and routes; execute tests and inject manual events. Cannot manage members or service tokens. | Automation Developer, Engineer |
| **`operator`** | 2 | Operational control without configuration mutation: trigger on-demand test runs, retry failed executions, resolve dead-letter records, and replay events. Cannot edit pipeline definitions. | SRE, Support Engineer |
| **`viewer`** | 1 | Read-only visibility: inspect pipelines, monitor live execution timelines, view dead-letter failures and audit logs. Sensitive credentials always masked. | Stakeholder, Security Auditor |

### 3. Service Accounts & Scoped API Tokens
Machine-to-machine integrations (e.g. GitHub Actions, CI/CD pipelines, external schedulers) authenticate using high-entropy API tokens:
- **Token Generation**: Generated with format `krb_<prefix>_<32-byte-hex>` (e.g. `krb_live_4a1f8e...`) using `crypto/rand`.
- **Hashed at Rest**: Plaintext secrets are displayed exactly once at creation time and never stored in the database. Kerob stores only the SHA-256 hash (`token_hash`) and a 12-character prefix (`token_prefix`) for identification.
- **Constant-Time Verification**: Inbound Bearer tokens are SHA-256 hashed and compared in constant time (`crypto/subtle.ConstantTimeCompare`) against active token hashes.
- **Granular Scopes**: Tokens support fine-grained scopes (e.g. `workflows:read`, `workflows:write`, `events:replay`, `dlq:retry`). If explicit scopes are configured, the token is strictly bounded by them regardless of the assigned base role. Wildcards (e.g. `workflows:*`) are supported.

### 4. Security Audit Trail & Automated Secret Scrubbing
All administrative, mutation, retry, replay, and sensitive access events emit structured audit records:
- **Audited Events**:
  - `auth.forbidden`: Authorization failures and permission denial attempts.
  - `workflow.created`, `workflow.updated`, `workflow.deleted`, `workflow.patched`, `workflow.tested`.
  - `credential.created`, `credential.updated`, `credential.deleted`, `credential.read`.
  - `execution.retried`, `dlq.retried`, `dlq.discarded`, `dlq.bulk_retried`.
  - `event.replayed`, `event.bulk_replayed`, `event.ingested`.
  - `member.created`, `member.role_updated`, `member.deleted`.
  - `token.created`, `token.revoked`.
- **Automated Secret Scrubbing**: Every detail map passed to `RecordAuditLog` passes through `ScrubAuditDetails`, which recursively scans maps, slices, and structs, replacing any key matching sensitive patterns (`secret`, `token`, `password`, `key`, `auth`, `credential`, `bearer`, `private`, `cert`) with `"[REDACTED]"`. Plaintext secrets or credentials are never stored in audit tables or application logs.

### 5. Backward Compatibility & Single-Tenant Dev Mode
- **Zero Configuration Breaking**: Single-tenant local development remains the default (`MULTI_TENANCY_ENABLED=false`, `DEFAULT_TENANT_ID="default"`).
- **Default Identity**: When multi-tenancy is disabled or running in local dev without authentication, incoming requests map seamlessly to `DefaultIdentity` (`tenant_id: "default"`, role: `owner`), preserving all previous test behaviors, developer scripts, and CLI workflows.

---

## 8. Real-Time Operations Center & Execution Lifecycle Control

Phase 3 introduces real-time execution observability and granular lifecycle management (pause, resume, cancel) designed to handle distributed workloads without polling storms.

```text
  ┌────────────────────────────────────────────────────────────────────────┐
  │                        Browser / Client Tier                           │
  │  Svelte Dashboard / External Tooling (EventSource or Polling Fallback) │
  └───────────────────────────────────▲────────────────────────────────────┘
                                      │
                   GET /api/v1/operations/stream?token=...
                   (text/event-stream; multi-tenant filtered)
                                      │
  ┌───────────────────────────────────┴────────────────────────────────────┐
  │                         LiveHub Subsystem                              │
  │  • Tenant-isolated event broadcasting (no cross-tenant leakage)        │
  │  • Non-blocking ring-buffered subscriber channels                      │
  │  • Keep-alive ping tickers (15s) with auto-reconnection                │
  └───────────────────────────────────▲────────────────────────────────────┘
                                      │
               ExecutionEventListener & DLQ Broadcast Hooks
                                      │
  ┌───────────────────────────────────┴────────────────────────────────────┐
  │                        Workflow Engine Core                            │
  │  • ActiveExecutionRegistry: Tracks active execution context cancels     │
  │  • Immediate Context Cancellation: Cancels running steps & HTTP calls  │
  │  • Pause / Resume State Transitions: Resumes without repeating stages  │
  │  • PostgreSQL & Memory Store Persistence: Durable leases & status locks │
  └────────────────────────────────────────────────────────────────────────┘
```

### 1. Server-Sent Events (SSE) Streaming Architecture
- **Endpoint**: `GET /api/v1/operations/stream` streams structured operational events (`text/event-stream`).
- **Multi-Tenant Filter**: Clients subscribe with their authenticated tenant identity (via `Authorization: Bearer <token>` or `?token=<token>` query parameter). Events generated by Tenant A are never broadcast or visible to subscribers from Tenant B.
- **Resilient Buffering**: `LiveSubscriber` maintains a non-blocking channel buffer (64 events). If a slow client channel fills up, events are dropped with dropped-counter metrics rather than blocking engine throughput.
- **Heartbeat & Keep-Alive**: A 15-second ticker emits periodic ping comments (`event: ping\ndata: {}\n\n`) to prevent intermediate proxies, firewalls, and cloud load balancers from terminating idle HTTP connections.
- **Frontend Automatic Reconnection & Fallback**: The Svelte client listens to native `EventSource`. On disconnect, it enters `reconnecting` state and re-establishes connectivity automatically. If SSE is blocked by environmental restrictions, it gracefully falls back to periodic polling.

### 2. Supported Operational Event Types
- `execution.started`: Triggered when an execution begins running.
- `execution.step`: Emitted as each stage (`trigger`, `condition`, `transformation`, `action`) begins and completes.
- `execution.completed`: Emitted when an execution succeeds.
- `execution.failed`: Emitted on permanent execution failure.
- `execution.retrying`: Emitted when an execution failure is scheduled for retry.
- `execution.paused`: Emitted when an operator pauses an execution.
- `execution.resumed`: Emitted when a paused execution is resumed.
- `execution.cancelled`: Emitted when an execution is cancelled.
- `dlq.created`, `dlq.retried`, `dlq.discarded`: Emitted when dead-letter records are created, retried, or discarded.

### 3. Execution Cancellation Propagation
- **Active Execution Registry**: `ActiveExecutionRegistry` maintains thread-safe mappings of active execution IDs to their respective `context.CancelFunc` and paused state flags.
- **Fast Interruption**: When an operator invokes `POST /api/v1/executions/{id}/cancel`, the active context is cancelled immediately. In-flight HTTP action requests (via SSRF client), delay timers (`executeDelayStep`), and step iterators terminate promptly with context cancellation errors.
- **Durable Persistence**: Status is atomically updated to `ExecutionCancelled` in the PostgreSQL store (or in-memory store for tests). Subsequent worker claims will refuse to run cancelled executions.
- **Audit Logging**: Cancellation emits structured audit records (`execution.cancelled`) with operator identity and execution ID.

### 4. Safe Pause and Resume Semantics
- **Atomic Pause Transition**: `POST /api/v1/executions/{id}/pause` cancels the running context and marks the execution `ExecutionPaused` in database storage. If worker lease heartbeats were active, the lease is released immediately so other workers do not hold locks.
- **Idempotent Delay & Step Continuation**: When an execution is resumed via `POST /api/v1/executions/{id}/resume`, the engine re-evaluates step history. If a delay step was already waiting, or an action step already completed, the engine does not re-execute or re-delay those steps; it continues directly from the unexecuted stages.
- **Race Safety**: Cloned data structures prevent race conditions between background execution goroutines and JSON response serialization.

### 5. Prometheus Observability Metrics
- `kerob_live_subscribers_active`: Current gauge of connected Server-Sent Events subscribers.
- `kerob_execution_cancellations_total`: Counter tracking total executions cancelled.
- `kerob_execution_pauses_total`: Counter tracking total executions paused.
- `kerob_execution_resumes_total`: Counter tracking total executions resumed.



