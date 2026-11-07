# Changelog

All notable changes to the **Kerob** platform are documented in this file.
The project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v0.3.0] - 2026-11-07

### Milestone Release: Enterprise Event Automation & Controlled Intelligence
Kerob `v0.3.0` is a major milestone release delivering 10 architectural phases that evolve Kerob into a distributed, multi-tenant enterprise event orchestration and automation platform with tamper-evident cryptographic governance and controlled AI assistance.

### Added

#### 1. Controlled AI Assistance Layer (Phase 10)
- **Deterministic & Non-Autonomous AI Architecture**:
  - Model outputs are persisted as immutable `Proposal` entities with `status: pending_approval`. Autonomous production mutations and autonomous workflow triggering are strictly prohibited.
  - Acceptance of any proposal strictly requires explicit human approval backed by `PermWorkflowsWrite` (`workflows:write`) permissions.
- **Five AI Capabilities**:
  - **Workflow Drafting (`POST /api/v1/ai/draft-workflow`)**: Drafts complete declarative workflows from natural language briefs.
  - **Payload Field Mapping (`POST /api/v1/ai/map-fields`)**: Analyzes source schemas and target destinations to propose key-to-key mapping pipelines.
  - **Condition Generation (`POST /api/v1/ai/generate-conditions`)**: Translates natural language criteria into declarative IF/ELSE condition AST trees.
  - **Transformation Suggestions (`POST /api/v1/ai/suggest-transformations`)**: Suggests UNIX-pipe transformations (extract, rename, format, cast).
  - **Failure Diagnosis (`POST /api/v1/ai/explain-failure`)**: Analyzes execution trace errors and dead-letters to provide actionable root-cause explanations and remediations.
- **Strict Threat Model & Safety Guardrails**:
  - **Disabled by Default**: AI subsystem remains inactive until explicitly enabled via `AI_ENABLED=true`.
  - **In-Memory Sanitization**: Redacts API keys, passwords, bearer tokens, AWS keys, and PII before dispatching model requests. Zero direct vault access.
  - **Deterministic Safety Validator**: Verifies candidate ASTs against dangerous execution patterns (`bash`, `sh -c`, `eval`, `rm -rf`, `curl`), SSRF addresses, unapproved connectors, and schema syntax errors.
  - **Provider Abstraction**: Supports OpenAI-compatible endpoints (Ollama, LocalAI, vLLM) and deterministic mock provider.
  - **Quota & Cost Limits**: Enforces monthly token ceilings (`AI_MONTHLY_TOKEN_LIMIT`), per-request token caps, timeouts, and tenant opt-out controls.
  - **Database Migration 000009**: Creates multi-tenant isolated `ai_proposals` table.

#### 2. Enterprise Governance & Cryptographic Policy Engine (Phase 9)
- **Tenant Compliance Policy Engine**:
  - Configurable policies per tenant: `allowed_connectors`, `allowed_outbound_domains`, `max_workflow_retries`, `max_payload_size_bytes`, `require_approval_irreversible`, `mask_pii`, `mask_secrets`, and `blocked_keywords`.
  - Enforcement modes: `enforce` (strictly blocks non-compliant publish attempts) and `audit` (records audit warning).
  - Dry-run policy validation endpoint: `POST /api/v1/governance/policy/validate`.
- **Tamper-Evident Cryptographic Audit Trail**:
  - SHA-256 HMAC hash chaining across all tenant audit events (`prev_hash`, `entry_hash`, `signature`).
  - Cryptographic verification endpoint (`GET /api/v1/governance/audit/verify`) detects sequence breaks, tampering, or missing entries.
- **Data Retention & Compliance Exports**:
  - Automated retention pruning (`POST /api/v1/governance/retention/prune`) based on `data_retention_days`.
  - Compliant audit log exports in JSON and CSV formats (`GET /api/v1/audit-logs/export`).
  - Database Migration 000008: Adds governance policy tables and cryptographic hash chaining columns.

#### 3. Unified Developer CLI, GitOps Automation & SDKs (Phase 8)
- **Kerob Unified CLI (`kerob`)**:
  - GitOps deployment (`kerob workflow deploy <path>`) with automated secret scanning, schema validation, semantic diffing, dry-runs, and rollback on failure.
  - Full resource management: `workflow`, `event`, `execution`, `logs`, `dlq`, `tenant`, `policy`, and `ai`.
  - Interactive login (`kerob login`) with configuration caching and shell auto-completion (`bash`, `zsh`, `fish`).
- **Go SDK (`pkg/sdk`) & TypeScript SDK (`@kerob/sdk`)**:
  - Zero-runtime-dependency Go SDK with complete typed API coverage.
  - Production TypeScript SDK with automated secret scanning and diff engines.

#### 4. Distributed Sagas, Compensation & Human Approvals (Phase 7)
- **Automatic Reverse-Order Compensation**:
  - Workflows can define compensating actions for reversible steps. On pipeline failure, the engine automatically triggers compensation steps in reverse order.
  - Manual operator compensation endpoint (`POST /api/v1/executions/{id}/compensate`).
- **Human-in-the-Loop Approval Gates**:
  - Dedicated approval step pausing workflow execution pending human decision.
  - Configurable expiration timeouts and escalation callbacks.
  - Cryptographically signed one-time decision tokens for email/webhook integrations.
  - Database Migration 000007: Adds approvals table with tenant isolation.

#### 5. Partitioned Event Streaming & Schema Registry (Phase 6)
- **Partitioned Event Broker**:
  - High-throughput streaming subsystem with partition ordering keys (`ordering_key`).
  - Pluggable streaming adapters (`durable` disk-backed and memory-backed).
  - Consumer offset management and dead-letter queue topics (`/api/v1/streams/dlq`).
- **JSON Schema Registry**:
  - Versioned schema registry for event payload validation (`POST /api/v1/schemas`).
  - Backward and forward compatibility mode enforcement.
  - Database Migration 000006: Adds schema registry, stream offsets, and streaming DLQ tables.

#### 6. Extensible Connector SDK & OAuth2 Lifecycle (Phase 5)
- **Pluggable Connector Architecture**:
  - Standardized `Connector` interface with manifest, capabilities, and dynamic health probes (`GET /api/v1/connectors/{id}/health`).
  - Built-in GitHub connector with webhook triggers and API actions.
- **Full OAuth2 Lifecycle**:
  - Authorization flow (`/api/v1/oauth/{connector}/authorize`), PKCE verification, automatic token refresh, and revocation.

#### 7. Visual Workflow Builder & Studio (Phase 4)
- **Interactive Visual Canvas**:
  - High-performance node graph canvas built in Svelte 5 with zoom, pan, and mini-map.
  - Visual editing for branch, loop (`for_each`), delay, parallel, and approval nodes.
  - Bidirectional visual-to-JSON/YAML synchronization with immediate schema validation.

#### 8. Real-Time Operations Center (Phase 3)
- **Sub-Second Live Operational Feed**:
  - Server-Sent Events (SSE) stream (`GET /api/v1/operations/stream`) broadcasting execution state changes, step transitions, and throughput metrics.
  - Interactive live dashboard with filterable execution logs and latency gauges.

#### 9. Multi-Tenancy & Role-Based Access Control (Phase 2)
- **Organizational Tenant Isolation**:
  - Row-level tenant partitioning across all database tables and queries.
  - Hierarchical RBAC: `admin`, `editor`, `operator`, and `viewer` roles.
  - Scoped API tokens with expiry and revocation management.
  - Database Migration 000005: Adds multi-tenant tables, memberships, and API tokens.

#### 10. Distributed Execution & Worker Coordination (Phase 1)
- **Resilient Distributed Execution**:
  - Distributed job leasing with PostgreSQL row locks and active worker heartbeats.
  - Automatic stale lease reclamation and worker failover recovery.
  - Multi-worker concurrent processing with priority execution queuing.

---

## [v0.2.0] - 2026-10-30

### Added
- **Multi-Step Execution Pipelines & Data Flow**:
  - Actions execute as an ordered pipeline with inter-step data referencing (`{{steps.<id>.output.<path>}}`, `{{steps.<id>.status}}`, `{{steps.<id>.error}}`).
  - Granular per-step error policies: `fail` (halt & fail), `continue` (record warning & proceed), and `skip_rest` (graceful early completion).
  - Individual step timeouts and retry overrides (`max_retries`, `backoff`).
  - Idempotent resumption at failed steps without re-executing already completed actions.
- **Control Flow Primitives**:
  - Branching step (`branch`): Evaluate condition cases sequentially with fallback default step list.
  - Safe array iteration (`for_each`): Loop over arrays in event payload with configurable safety cap (up to 100 iterations).
  - Asynchronous non-blocking delay (`delay` / `wait`): Schedules resumption via background queue without worker thread blocking.
  - Bounded parallel fan-out (`parallel`): Concurrently executes independent action branches with configurable concurrency limits.
- **Rich Condition Operators & Expression Evaluation**:
  - Added 18 condition operators: `starts_with`, `ends_with`, `exists`, `not_exists`, `less_than`, `less_or_equal`, `greater_or_equal`, `between`, `regex` (guarded RE2), `in`, `not_in`, `is_empty`, `is_not_empty`, and `time_window` with IANA timezone support.
  - JSONPath bracket indexing (`payload.items[0].id`) and wildcard array quantifiers (`payload.items[*].status`).
- **Deterministic Transformations & Safe Filter Library**:
  - 16 pure UNIX-pipe template filters: `upper`, `lower`, `trim`, `truncate:N`, `replace`, `split`, `join`, `default`, `json_encode`, `url_encode`, `base64`, `date_format`, `number_format`, `regex_extract`, `coalesce`, `length`.
  - Transform modes: `mapping`, `pick`, `omit`, `flatten`, `merge`, and `compute` (pure arithmetic & string concatenation).
  - Secure vault secret resolution (`{{secrets.<name>}}`) injected only into action configurations.
- **Integration SDK & Schema-Driven Studio**:
  - Declarative `Integration` interface + registry with manifest, capabilities, and required credential types.
  - Machine-readable schema endpoint (`GET /api/v1/integrations/{id}/schema`).
  - Dynamic schema-driven configuration form in Svelte 5 dashboard (`SchemaConfigForm.svelte`), enabling zero-frontend code additions for new connectors.
- **New Integrations**:
  - Inbound webhook signature verifiers: Generic HMAC-SHA256, Slack-style (`v0=...`), and Stripe-style (`t=...,v1=...`) with replay window tolerance.
  - HTTP Polling trigger (`http_poll`): Periodic JSON polling with deduplication and ETag / Last-Modified caching headers.
  - Slack action: Post to incoming webhooks or `chat.postMessage` API with block kit.
  - Microsoft Teams action: Deliver Office 365 connector cards and adaptive cards.
  - SMTP Email action: Authenticated transactional email dispatch.
  - Call Workflow action (`call_workflow`): Programmatic sub-workflow chaining with loop and depth guards (max depth 5).
- **Traffic Control & Reliability**:
  - Per-workflow token-bucket rate limiting (max executions per minute).
  - Per-workflow concurrency limits and entity serialization concurrency key (`concurrency_key: "payload.chat_id"`).
  - Automated circuit breakers per integration (Closed/Open/Half-Open states) surfaced in `/health` and Prometheus metrics.
  - Execution priority levels (high-priority jobs claimed before normal runs).
  - Outbound HTTP action request signing (`X-Kerob-Signature-256` and `X-Kerob-Timestamp`).
- **Workflow Operability & Tooling**:
  - Revision history tracking, version snapshots, diffing (`GET /api/v1/workflows/{id}/diff`), and instant rollback (`POST /api/v1/workflows/{id}/rollback/{version}`).
  - Dry-run evaluation endpoint (`POST /api/v1/workflows/{id}/test`): Step-by-step trace of trigger matching, conditions, and transforms with mock action outputs.
  - JSON and YAML workflow import/export (`/api/v1/workflows/export`, `/api/v1/workflows/import`).
  - Built-in template recipe library (`GET /api/v1/templates`).
  - Event replay with explicit idempotency bypass (`POST /api/v1/events/{id}/replay`, `POST /api/v1/events/replay-bulk`).
  - Dead-Letter Queue bulk retry and discard endpoints (`POST /api/v1/dead-letter/retry-bulk`, `POST /api/v1/dead-letter/discard-bulk`).
- **Quality & Code Health**:
  - Strict CI coverage gates: Go statement coverage >= 60.0% (current: 66.5%), Frontend coverage >= 65.0% (current: 78.43%).
  - Zero `as any` casts across Svelte 5 frontend codebase with shared API type models.
  - Strict enforcement of file length: all Go source and test files maintained strictly under 500 lines of code.
  - Global error tracking integration with Sentry DSN fallback to structured log scrubbing.

---

## [v0.1.1] - 2026-10-30

### Added
- **Release Engineering & Standalone Distribution**:
  - Automated multi-platform release builder (`scripts/build-release.sh`) compiling self-contained release distributions for `linux/amd64`, `linux/arm64`, `darwin/amd64` (Intel macOS), `darwin/arm64` (Apple Silicon), and `windows/amd64`.
  - Embedded single-file Svelte 5 operations dashboard directly compiled into backend binaries—no Node.js runtime or separate asset serving required in production.
  - Built-in `--version`, `-v`, and `version` CLI flags and startup logs displaying semantic version, git commit SHA, and build timestamp.
  - Automated GitHub Actions release pipeline (`.github/workflows/release.yml`) running the full verification pipeline (deps, linters, tests, coverage, frontend, docker) before cross-compiling and publishing release assets.
  - Cryptographic SHA-256 checksums (`checksums.txt`) generated and attached to all downloadable release bundles.
  - Reproducible `make build`, `make release-artifacts`, and `make release` targets.
- **Schema-Based Boundary Request Validation Layer**:
  - Pre-domain schema and parameter validation across all mutation endpoints (`/api/v1/workflows`, `/api/v1/credentials`, `/api/v1/webhooks`, `/api/v1/sources`, `/api/v1/destinations`, `/api/v1/routes`, `/api/v1/events`).
  - Standardized error schema (`APIError` and `FieldError`) with machine-readable error codes and request tracing IDs.
  - Table-driven boundary test suite covering edge cases: invalid UUID formats, missing required fields, payload limits, negative rates, and malformed JSON.
- **In-Memory Storage & Offline Test Architecture**:
  - Thread-safe, deep-copy in-memory store implementations for workflows, execution tracking, dead-letter records, credentials, routes, and events.
  - Complete dependency injection architecture allowing all unit, integration, and e2e tests to execute offline in milliseconds without Docker, PostgreSQL, or external network connectivity.
  - Categorized test tiers: `make test-unit`, `make test-integration`, `make test-e2e`, `make test-docker`, `make test-frontend`, `make test-coverage`, and `make test-all`.
- **Execution Observability & Error Tracking Hardening**:
  - Modular `ErrorTracker` provider abstraction (`noop`, `memory`, `log`, `sentry`) for recording unhandled exceptions and security events without coupling core logic.
  - Contextual error capturing with `request_id`, `workflow_id`, and `execution_id`.
  - Automatic credential and secret scrubbing in logs and error captures.
- **Strict Coverage Enforcement Gates**:
  - Enforced Go statement coverage threshold (≥60.0%) blocking CI on regressions (current statement coverage: 65.4%).
  - Enforced frontend coverage threshold (≥65.0%) across Svelte components, API client, and modal workflows.
- **Enforced Dependency Health & Freshness System**:
  - Dedicated `dependency-health` job in GitHub Actions CI running on all PRs and pushes.
  - Blocking `govulncheck ./...` vulnerability scanning in CI with zero vulnerabilities.
  - Blocking `npm audit --audit-level=high` frontend vulnerability auditing.
  - Enforced Go and frontend dependency freshness policies (`scripts/check-go-deps.sh`, `scripts/check-frontend-deps.sh`, `scripts/check-dependencies.sh`).
  - Active policy rules defined in `.dependency-policy.json` preventing lockfile drift and unreviewed upstream bumps.
  - Synchronized and verified frontend dependencies (`package.json` and `package-lock.json`).
- **Security & Governance**:
  - Comprehensive vulnerability disclosure and security policy documented in `SECURITY.md`.
  - AES-256-GCM encryption for credentials at rest with unique random nonces.
  - Proactive outbound SSRF protection blocking loopback, link-local, private networks (RFC1918), cloud metadata IPs, and defeating DNS rebinding.

### Changed
- Refactored monolithic test files into domain-focused test suites (`internal/api`, `internal/core/workflow`, `internal/adapters`).
- Updated frontend version alignment to match platform version `v0.1.1`.
- Cleaned and reinforced Docker Compose production configurations with mandatory token validation.

### Known Limitations
- In-memory storage is optimized for local testing and ephemeral workflows; persistent production deployments require PostgreSQL 15+.
- SSRF protection strictly prevents outbound HTTP requests to intranet and loopback addresses; external API endpoints must be accessible via public DNS.

---

## [v0.1.0] - 2026-10-22

### Added
- **Core Workflow & Scheduling Engine**:
  - Declarative `WHEN [trigger] IF [conditions] TRANSFORM THEN [actions]` execution architecture.
  - Granular lifecycle tracking (`pending`, `running`, `success`, `failed`, `retrying`, `cancelled`).
  - Idempotency protection on `(workflow_id, event_id)` preventing duplicate processing across restarts.
  - Exponential backoff retry engine with configurable jitter and limits.
  - Dead-letter failure queue for unrecoverable executions with manual retry and discard capabilities.
  - Workflow Scheduling Engine supporting standard 5-part cron syntax (`min hour dom month dow`) and duration intervals (`5m`, `1h`, `24h`), emitting normalized `schedule.tick` events.
  - Safe, deterministic condition AST evaluation with boolean combinators (`and`, `or`, `not`) and operators (`equals`, `contains`, `matches`, `greater_than`, `in`, etc.).
  - Template transformation engine with JSON path navigation (`{{event.payload.title}}`) and fallback syntax (`{{payload.title | default: "Untitled"}}`).
- **Offline Testability & In-Memory Storage**:
  - Complete dependency injection architecture for persistence layer.
  - Thread-safe in-memory stores for Sources, Destinations, Routes, Events, and Deliveries with mutex synchronization.
  - Error wrapping with standard domain errors (`ErrSourceNotFound`, `ErrDestinationNotFound`, `ErrRouteNotFound`) preserving `errors.Is` semantics.
  - 100% offline test execution: full Go test suite runs cleanly with `go test ./...` without requiring PostgreSQL, Docker, Telegram, Discord, or external services.
- **Strict CI Test Coverage Enforcement**:
  - Enforced Go statement coverage threshold (≥60.0%) in CI pipeline via `go test -coverprofile=coverage.out ./...` and `go tool cover`.
  - Automated CI failure trigger if test coverage drops below 60.0% (current coverage: 61.4%).
  - Added unit and edge-case tests across workflow execution, condition evaluation, payload transformation, Telegram poller, actions, SSRF validation, and failure paths.
- **Frontend Quality Assurance & Developer Control Plane**:
  - Svelte 5 and TypeScript interactive operations studio embedded into the Go server binary.
  - Automated quality tooling: ESLint with `@typescript-eslint` and `eslint-plugin-svelte`, Prettier code formatting, `svelte-check` type checking, and Vitest component testing.
  - Frontend test suite covering critical UI components (`StatCard`, `Badge`, `Button`, `EmptyState`, `ConfirmModal`, `APIClient`).
  - Strict CI pipeline checks for frontend: `npm run check`, `npm run lint`, `npm run format:check`, and `npm test` enforced on push and PR.
- **API Boundary Validation Layer**:
  - Pre-domain schema and parameter validation for all POST, PUT, PATCH, and webhook ingestion endpoints.
  - Consistent structured 400 Bad Request error responses with correlation `request_id`, descriptive `message`, and field-level validation errors.
  - Table-driven boundary validation tests for malformed JSON, empty fields, invalid URLs, and unknown enums.
  - OpenAPI 3.0 schema definitions updated with `APIError` and `FieldError`.
- **Architecture & Production Error Tracking**:
  - Modular `ErrorTracker` provider abstraction (`noop`, `memory`, `log`, `sentry`) for recording unhandled exceptions and security events without coupling core logic.
  - Contextual error capturing with `request_id`, `workflow_id`, and `execution_id`.
  - Automatic credential and secret scrubbing in logs and error captures.
- **Security & Proactive SSRF Protection**:
  - Encrypted storage at rest for integration credentials using AES-256-GCM.
  - Masked credential representation in API responses (`••••••••1234`).
  - Proactive SSRF defense on all outbound HTTP actions and webhooks: blocks loopback, private networks (RFC1918), link-local, cloud metadata IPs, and defeats DNS rebinding via custom socket dialers.
- **Integrations & Adapters**:
  - **Telegram**: Channel/group monitoring, media-group aggregation window, public channel web scraper, rate-limited bot client, formatted text/photo/document/album delivery.
  - **Discord**: Webhook and bot delivery supporting markdown messages and rich embeds.
  - **RSS / Atom**: Periodic feed polling and XML parsing with GUID/link deduplication.
  - **GitHub**: Inbound webhook ingestion with HMAC-SHA256 signature verification (`X-Hub-Signature-256`) and issue creation action.
  - **Generic Inbound Webhooks**: Secure endpoints at `/webhooks/{id}` with secret token validation and 1MB payload limits.
  - **Generic HTTP Action**: Calling external REST APIs (GET, POST, PUT, PATCH, DELETE) with body templating and SSRF defense.
- **DevOps & Repository Hardening**:
  - Multi-stage Dockerfile with non-root security context, automated frontend build, and health checks.
  - GitHub Actions CI workflow running Go dependency freshness, `gofmt`, `golangci-lint`, `govulncheck`, frontend quality checks, race detector tests, coverage gate, and multi-stage Docker build.
  - Automated dependency updates via Dependabot for both Go modules and npm packages.
  - Updated environment configuration templates (`.env.example` with `VITE_BACKEND_URL`).
