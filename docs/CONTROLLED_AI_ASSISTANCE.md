# Controlled AI Assistance Layer

Kerob provides an optional, strictly controlled AI assistance layer designed for enterprise automation engineering. It empowers developers and operators with AI-assisted workflow drafting, schema-to-schema field mapping, declarative filter condition generation, transformation suggestions, and execution failure explanations—while maintaining zero autonomous mutation risk.

---

## 1. Security Architecture & Threat Model

### Core Principles
1. **Disabled by Default**: AI features are completely deactivated until explicitly enabled via `AI_ENABLED=true`.
2. **Non-Autonomous Proposals (Human-in-the-Loop)**: AI outputs are always stored as intermediate `Proposal` records with `status: pending_approval`. The model is mathematically and architecturally prohibited from applying changes directly to production configurations or triggering executions autonomously.
3. **Deterministic Safety Validation**: Every model output must pass an independent, deterministic AST/JSON validator before being presented to a human reviewer. Proposals containing prohibited execution primitives, shell syntax, command execution, or unapproved connector types are automatically rejected.
4. **Mandatory Secret & PII Scrubbing**: All user prompts, context objects, and schema payloads are sanitized prior to external model dispatch. API tokens, authorization headers, private keys, passwords, and sensitive identifiers are masked in memory.
5. **No Direct Secret Access**: The AI assistance service has zero access to decrypted secret vaults or credentials stored in Kerob.

### Threat Matrix & Mitigations

| Threat Vector | Potential Impact | Kerob Mitigation |
| :--- | :--- | :--- |
| **Prompt Injection** | Malicious input attempts to instruct model to execute arbitrary bash commands or exfiltrate credentials | Deterministic AST safety validator scans for forbidden keywords (`bash`, `sh -c`, `eval`, `rm -rf`, `curl`, etc.) and enforces schema structure. The execution engine has no shell executor. |
| **SSRF (Server-Side Request Forgery)** | Model generates HTTP step endpoints targeting `169.254.169.254`, `127.0.0.1`, or local subnet | Strict URL parsing and SSRF validator verify schemes (`http`/`https`) and reject loopback, link-local, cloud metadata IP addresses, and private subnets. |
| **Secret Exfiltration via Prompt** | Trace logs or failure payloads sent to external model contain sensitive credentials | In-memory regex and key sanitizer automatically mask API keys, bearer tokens, AWS credentials, and PII before dispatching requests. |
| **Autonomous Model Tampering** | Compromised model provider mutates live workflows or triggers actions | Model outputs are strictly persisted as immutable proposals. Applying a proposal requires explicit human approval via `PermWorkflowsWrite`. |
| **Cost / Resource Denial of Service** | Runaway prompt generation consumes excessive API quota | Configurable monthly token limit (`AI_MONTHLY_TOKEN_LIMIT`), per-request token ceilings, and request timeouts. |
| **Multi-Tenant Cross-Contamination** | Tenant A accesses or accepts proposals drafted for Tenant B | PostgreSQL row-level indexing and strict tenant ID assertions reject cross-tenant proposal retrieval or decision attempts. |

---

## 2. Configuration & Provider Setup

### Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `AI_ENABLED` | bool | `false` | Master toggle for the AI assistance subsystem. |
| `AI_PROVIDER` | string | `mock` | Model provider: `openai`, `ollama`, `localai`, or `mock`. |
| `AI_MODEL` | string | `gpt-4o-mini` | Target model identifier. |
| `AI_ENDPOINT` | string | `""` | Base API URL for OpenAI-compatible providers (e.g. `http://localhost:11434/v1`). |
| `AI_API_KEY` | string | `""` | API key for external model authentication. |
| `AI_MAX_TOKENS` | int | `2048` | Maximum token limit per request. |
| `AI_TIMEOUT` | duration | `30s` | HTTP timeout for model inference calls. |
| `AI_MONTHLY_TOKEN_LIMIT` | int64 | `1000000` | Monthly token usage budget per tenant. |

---

## 3. Supported Assistance Capabilities

### 1. Workflow Drafting (`POST /api/v1/ai/draft-workflow`)
Generates a complete, multi-step declarative workflow definition from natural language instructions.
- Validates name, trigger structure, action array, and connector approvals.
- Verifies that action steps only target connectors specified in `allowed_connectors`.

### 2. Payload Field Mapping (`POST /api/v1/ai/map-fields`)
Analyzes source schema structures, destination schemas, and sample payloads to propose optimal field-level mappings.
- Validates destination key identifiers and ensures non-empty mappings.

### 3. Condition Generation (`POST /api/v1/ai/generate-conditions`)
Translates business rules (e.g., "Only allow priority high or critical orders with amount greater than 100") into declarative IF/ELSE condition trees.
- Validates field presence, logical operators (`and`, `or`, `not`), and valid comparison operators (`eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `contains`, `regex`).

### 4. Transformation Suggestions (`POST /api/v1/ai/suggest-transformations`)
Suggests declarative transformations (extract, rename, format timestamp, map values) to convert incoming payloads to downstream schemas.

### 5. Failure Diagnosis (`POST /api/v1/ai/explain-failure`)
Analyzes execution errors, dead letters, and trace history to identify the root cause and propose actionable remediation steps.

---

## 4. Human-in-the-Loop Proposal Lifecycle

```
User Prompt / Context
        │
        ▼
[ Secret & PII Sanitizer ]  ──> Redacts credentials, tokens, sensitive fields
        │
        ▼
[ AI Provider Engine ]      ──> Generates draft payload & explanation
        │
        ▼
[ Deterministic Validator ] ──> Verifies AST, AST schemas, SSRF, keywords
        │
        ▼
[ Proposal Stored (pending_approval) ]
        │
 ┌──────┴──────────────────────────┐
 │                                 │
 ▼                                 ▼
[ Explicit User Rejection ]       [ Explicit User Approval ]
 (status: rejected)                (Requires PermWorkflowsWrite)
                                           │
                                           ▼
                                 [ Workflow Instantiated / Updated ]
```

---

## 5. API Reference

### Check Status & Quota
```bash
curl -X GET http://localhost:8080/api/v1/ai/status \
  -H "Authorization: Bearer $KEROB_TOKEN"
```

### Draft Workflow
```bash
curl -X POST http://localhost:8080/api/v1/ai/draft-workflow \
  -H "Authorization: Bearer $KEROB_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "When a GitHub release is created, post an announcement to Telegram",
    "allowed_connectors": ["http", "telegram", "webhook"]
  }'
```

### Accept Proposal (Creates Workflow)
```bash
curl -X POST http://localhost:8080/api/v1/ai/proposals/{proposal_id}/accept?create_workflow=true \
  -H "Authorization: Bearer $KEROB_TOKEN"
```

### Reject Proposal
```bash
curl -X POST http://localhost:8080/api/v1/ai/proposals/{proposal_id}/reject \
  -H "Authorization: Bearer $KEROB_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason": "Does not match current channel requirements"}'
```

---

## 6. CLI Usage

The Kerob CLI provides first-class support for AI operations:

```bash
# Check AI subsystem availability and quota
kerob ai status

# Draft a workflow proposal
kerob ai draft "Send Telegram notification on webhook events" --connectors http,telegram

# Diagnose an execution failure
kerob ai explain exec_01HQXYZ123456

# List recent proposals
kerob ai proposals

# Accept and instantiate a proposed workflow
kerob ai accept prop_01HQXYZ789 --create-workflow

# Reject a proposal with reason
kerob ai reject prop_01HQXYZ789 --reason "Redundant rule"
```
