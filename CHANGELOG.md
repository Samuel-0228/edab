# Changelog

All notable changes to the **Kerob** platform are documented in this file.
The project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
