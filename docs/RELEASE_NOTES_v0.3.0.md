# Kerob v0.3.0 Release Notes

**Release Date:** November 7, 2026  
**Tag:** `v0.3.0`  
**License:** MIT  

---

## Executive Summary

Kerob `v0.3.0` represents a milestone evolution from a single-node workflow engine into a fully distributed, multi-tenant enterprise event orchestration and automation platform. Over the course of 10 structured architectural phases, Kerob has been augmented with:

1. **Distributed Execution & Fair Worker Coordination**: Row-locked lease distribution, active heartbeat monitors, and automatic stale lease recovery.
2. **Multi-Tenancy & Hierarchical RBAC**: Strict tenant isolation at the database layer, four built-in role tiers (`admin`, `editor`, `operator`, `viewer`), and scoped API tokens.
3. **Real-Time Operations Center**: Low-latency Server-Sent Events (SSE) telemetry hub with live metric visualizations.
4. **Visual Workflow Studio**: Node-graph workflow canvas built in Svelte 5 with bidirectional YAML/JSON synchronization.
5. **Extensible Connector SDK**: Pluggable connector manifest framework with dynamic health checking and complete OAuth2 token lifecycle management.
6. **Partitioned Event Streaming & Schema Registry**: High-throughput partitioned event broker with partition keys, consumer offset tracking, and versioned JSON Schema enforcement.
7. **Distributed Sagas & Human Approvals**: Reverse-order automatic compensation on workflow failure, human-in-the-loop approval gates, and cryptographically signed decision tokens.
8. **Unified CLI & GitOps Automation**: Production CLI (`kerob`) with semantic diffing, dry-runs, automated secret scanning, and companion Go and TypeScript SDKs.
9. **Enterprise Governance & Cryptographic Policy Engine**: Tenant-level policy enforcement, tamper-evident SHA-256 HMAC chained audit logs, and automated GDPR/SOC2 retention pruning.
10. **Controlled AI Assistance Layer**: Secure AI workflow drafting, field mapping, condition generation, and failure diagnosis with deterministic AST validation, secret masking, and strict human review requirements.

All new capabilities adhere to Kerob's strict architectural tenets:
- **Zero external runtime Go dependencies**: The entire Go engine, CLI, and SDK rely solely on the Go standard library.
- **Strict backward compatibility**: Workflows, configurations, and APIs from `v0.1.x` and `v0.2.x` continue to function without modification.
- **100% file length compliance**: Every Go source and test file remains strictly under 500 lines of code.

---

## Key Capabilities & Highlights

### 1. Controlled AI Assistance (Phase 10)
- **Threat Model**: AI features are disabled by default (`AI_ENABLED=false`). When enabled, AI outputs are strictly intermediate proposals (`status: pending_approval`) requiring explicit human authorization (`PermWorkflowsWrite`) before any workflow can be created or updated.
- **Data Privacy**: Automatic in-memory sanitization masks credentials, tokens, passwords, and PII prior to model inference.
- **Deterministic AST Validator**: Automatically intercepts and rejects dangerous execution patterns (`bash`, `eval`, arbitrary scripts) and SSRF targets.
- **Supported Capabilities**: Workflow Drafting, Schema Field Mapping, Condition Generation, Transformation Suggestions, and Execution Failure Explanations.

### 2. Enterprise Governance & Cryptographic Auditing (Phase 9)
- **Policy Enforcement**: Enforce organizational constraints on allowed connectors, outbound domains, retries, and payload sizes with `enforce` or `audit` modes.
- **Tamper-Evident Hash Chain**: Every tenant audit event is cryptographically hashed with SHA-256 HMAC and chained to its predecessor. Any modification or deletion breaks the chain and is detected via `GET /api/v1/governance/audit/verify`.
- **Automated Data Retention**: Automatic background pruning removes audit logs older than the tenant's configured retention window.

### 3. Unified Developer CLI & SDKs (Phase 8)
- **`kerob` CLI**:
  - `kerob workflow deploy <path> --dry-run`: GitOps deployment with automated AST secret scanning and semantic diffing.
  - Resource commands: `workflow`, `event`, `execution`, `dlq`, `logs`, `tenant`, `policy`, `ai`, and `completion`.
- **Typed SDKs**:
  - Pure Go SDK (`pkg/sdk`) for building automation microservices and integrations.
  - Typed TypeScript SDK (`@kerob/sdk`) for Node.js, Bun, Deno, and web browsers.

### 4. Distributed Sagas & Human Approvals (Phase 7)
- **Compensating Actions**: Define compensating steps that automatically execute in reverse order if a workflow fails downstream.
- **Approval Steps**: Pause execution until an operator reviews the request via the dashboard, API, or one-time signed decision token.

### 5. Partitioned Event Streaming & Schema Registry (Phase 6)
- **Streaming Engine**: High-throughput partitioned topics with strict ordering keys (`ordering_key`).
- **Schema Registry**: Enforce JSON Schemas with backward/forward compatibility checks.

---

## Database Migrations

Kerob `v0.3.0` includes 4 new database migrations (000006 through 000009):
- `000006_streaming_and_schemas`: Tables for event streaming, schema registry, and streaming DLQ.
- `000007_sagas_and_approvals`: Tables for human approvals and saga execution state.
- `000008_governance_and_policy`: Tables for tenant governance policies and cryptographic audit log chaining.
- `000009_controlled_ai_assistance`: Tables for AI proposals with multi-tenant isolation.

To apply migrations manually:
```bash
./bin/kerob migrate up
```
Or when running the engine, migrations apply automatically on startup.

---

## Upgrading from v0.2.0

Kerob `v0.3.0` is a drop-in replacement for `v0.2.0`. No breaking changes to existing workflow schemas or API endpoints have been introduced.

1. **Pull the latest Docker image or binary:**
   ```bash
   docker pull ghcr.io/samuel-0228/kerob:v0.3.0
   ```
2. **Apply migrations:** Migrations run automatically when starting the binary or container.
3. **Enable optional new features:**
   - AI Assistance: Set `AI_ENABLED=true` and configure `AI_PROVIDER` (e.g. `openai`, `ollama`).
   - Multi-Tenancy: Set `MULTI_TENANCY_ENABLED=true` to enable tenant isolation.
   - Partitioned Streaming: Set `STREAMING_ENABLED=true`.

---

## Verification & Quality Metrics

- **Go Unit & Race Tests**: 100% PASS with `-race` across all 25 Go packages.
- **Go SDK Tests**: 100% PASS.
- **TypeScript SDK Tests**: 100% PASS (7/7 suites).
- **Frontend Unit Tests**: 100% PASS (17 test files, 44 tests in Vitest).
- **Formatting & Vet**: `gofmt -l .` and `go vet ./...` clean with zero warnings.
- **Binary Compilation**: Clean standalone builds for server (`kerob`) and CLI (`kerob-cli`).
