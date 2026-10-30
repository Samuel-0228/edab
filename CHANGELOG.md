# Changelog

All notable changes to the **Kerob** platform are documented in this file.
The project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
