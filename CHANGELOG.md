# Changelog

All notable changes to the **Kerob** platform are documented in this file.
The project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v0.1.0] - 2026-10-20

### Added
- **Core Workflow Engine**:
  - Declarative `WHEN [trigger] IF [conditions] THEN [actions]` execution architecture.
  - Granular lifecycle tracking (`pending`, `running`, `success`, `failed`, `retrying`, `cancelled`).
  - Structured step audit history recording execution stages: trigger ingestion, condition evaluation, payload transformation, and action execution.
  - Strict idempotency protection on `(workflow_id, event_id)` preventing duplicate processing across restarts or network retries.
  - Exponential backoff retry engine with configurable jitter and limits.
  - Dead-letter failure queue for unrecoverable executions with manual retry and discard capabilities.
- **Condition Evaluator**:
  - Safe, deterministic AST evaluation without arbitrary code execution.
  - Supported operators: `equals`, `not_equals`, `contains`, `not_contains`, `starts_with`, `ends_with`, `matches` (bounded regex), `exists`, `not_exists`, `greater_than`, `less_than`, `in`, `not_in`.
  - Boolean combinators: `and`, `or`, `not` with arbitrary nesting.
- **Transformation Engine**:
  - String template rendering with event data path navigation: `{{event.payload.title}}`, `{{event.id}}`, `{{metadata.author}}`.
  - Default value fallback syntax: `{{payload.title | default: "Untitled"}}`.
  - Field mapping, key renaming, and JSON config template interpolation.
- **Security & Credentials**:
  - Encrypted storage at rest for integration tokens and keys using AES-256-GCM.
  - Masked credentials rendering in API responses and logs (`••••••••1234`).
  - Strict Proactive SSRF Protection on all outbound HTTP actions and webhooks:
    - Blocks loopback (`127.0.0.0/8`, `::1`), private networks (RFC1918), link-local (`169.254.0.0/16`), multicast, and cloud metadata IP (`169.254.169.254`).
    - Defeats DNS rebinding by validating resolved IP addresses at connection dial time.
    - Prevents dangerous HTTP redirects to internal addresses.
  - Constant-time token verification for API authentication.
- **Integrations & Adapters**:
  - **Telegram**: Channel/group monitoring, Album/media-group aggregation window, public channel web scraper, rate-limited bot client, formatted text/photo/document/album delivery.
  - **Discord**: Webhook and bot delivery supporting markdown messages and rich embeds (title, description, color, fields, footer, timestamp).
  - **RSS / Atom**: Periodic feed polling and XML parsing, item deduplication by GUID/link, normalized event emission.
  - **GitHub**: Inbound webhook ingestion with HMAC-SHA256 signature verification (`X-Hub-Signature-256`), issue creation action.
  - **Generic Inbound Webhooks**: Secure endpoints at `/webhooks/{id}` with secret token validation and 1MB payload limits.
  - **Generic HTTP Action**: Calling external REST APIs (GET, POST, PUT, PATCH, DELETE) with body templating and SSRF defense.
- **REST API & Developer Dashboard**:
  - Modular REST API partitioned by domain (`/api/v1/overview`, `/api/v1/workflows`, `/api/v1/integrations`, `/api/v1/credentials`, `/api/v1/executions`, `/api/v1/failures`, `/api/v1/webhooks`, `/api/v1/events`).
  - Developer Dashboard UI with high-density technical dark aesthetic (black, dark gray, emerald accents).
  - Full operational metrics: active automations, executions 24h, success rate, dead-letter count.
  - Execution timeline modal inspecting step status, inputs, outputs, errors, and millisecond durations.
  - Liveness (`/health`), Readiness (`/ready`), and Prometheus metrics (`/metrics`).
- **Production Infrastructure**:
  - Multi-stage Dockerfile with non-root user and healthchecks.
  - GitHub Actions CI workflow (formatting, vetting, race detection tests, binary compilation, docker build).
  - Automated dependency updates via Dependabot.
