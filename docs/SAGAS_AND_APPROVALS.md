# Sagas, Compensation, and Human Approvals in Kerob

This document outlines the architecture, execution model, lifecycle, security guarantees, and operational procedures for **Saga Compensations** and **Human Approval Steps** within Kerob.

---

## 1. Architectural Overview

Kerob implements the **Saga Pattern (Backward Recovery)** and **Durable Asynchronous Human Approvals** directly inside its distributed workflow engine.

```
       [Workflow Execution Initiated]
                     │
                     ▼
             [Action Step A]  ──────────► Succeeded (recorded in step audit)
                     │
                     ▼
          [Human Approval Step B]
                     │
        ┌────────────┴────────────┐
        ▼                         ▼
   [Approved]             [Rejected / Expired]
        │                         │
        ▼                         ▼
 [Action Step C]         [Trigger Saga Compensation]
        │                         │
        ▼ (if Step C fails)       ▼
[Trigger Saga Rollback]  [Compensate A in Reverse Order (LIFO)]
```

### Key Guarantees
- **Durable Non-Blocking Waiting**: When a workflow reaches a human approval step, the engine transitions the execution to `waiting_approval` status, persists the state to PostgreSQL (or in-memory store in dev/test), and **immediately releases the worker goroutine and database connection**. No threads, goroutines, or database locks are held while awaiting human input.
- **Reverse Order (LIFO) Rollback**: When a failure occurs (with retries exhausted), cancellation is invoked, or an operator triggers compensation, Kerob iterates backwards through all completed steps and executes their declared compensation actions in strict reverse order (`LIFO`).
- **Cryptographic Token Security**: Approval tokens are generated using 32 cryptographically secure random bytes (64 hex characters) and stored exclusively as SHA-256 hashes (`token_hash`) in the database.
- **Tenant Isolation and RBAC**: Approval endpoints, listings, and operator rollback endpoints are strictly protected by tenant isolation and scoped permissions (`approvals:read`, `approvals:write`, `executions:compensate`).

---

## 2. Human Approvals

### 2.1 Workflow Definition
Human approval steps can be configured either as dedicated steps with `type: "approval"` or attached to action steps with an `approval` configuration block:

```json
{
  "id": "step_mgr_approval",
  "name": "Manager Spend Approval",
  "type": "approval",
  "approval": {
    "title": "Approve High Value Transaction",
    "description": "Payment amount exceeds $5,000 threshold. Verify invoice authenticity.",
    "approver_roles": ["admin", "owner"],
    "approver_users": ["finance-director@example.com"],
    "timeout_seconds": 86400,
    "escalation_role": "owner"
  }
}
```

### 2.2 Lifecycle & State Machine
1. **Creation**:
   - Step is executed; engine creates a durable record in `workflow_approvals`.
   - Secure cryptographic token generated; SHA-256 hash stored.
   - Execution status transitions to `waiting_approval`.
   - Real-time event `execution.waiting_approval` broadcast over SSE/WebSockets.
2. **Approval**:
   - Authorized user calls `POST /api/v1/approvals/{id}/approve` or provides the secure token via `POST /api/v1/approval-tokens/{token}`.
   - Approval status is updated atomically from `pending` to `approved`.
   - Execution status transitions to `running` and processing resumes immediately on the next step.
3. **Rejection**:
   - Approver or token holder calls reject endpoint with an optional reason note.
   - Approval status updates to `rejected`.
   - Engine records the step as failed/rejected and **immediately initiates saga compensation** for all preceding completed steps.
4. **Timeout & Escalation**:
   - The background worker periodically executes `CheckExpiredApprovals(ctx)`.
   - If `timeout_seconds` has elapsed and `escalation_role` is set: status transitions to `escalated`, extending authorization to the escalation role.
   - If no escalation role is configured or escalation period expires: status transitions to `expired` and saga compensation is triggered automatically.

---

## 3. Sagas & Compensation

### 3.1 Defining Compensation Actions
Every action step in a workflow may optionally declare a `compensation` step:

```json
{
  "id": "step_charge_card",
  "name": "Charge Customer Credit Card",
  "type": "action",
  "integration": "stripe",
  "action_type": "charge",
  "config": {
    "amount": 5000,
    "currency": "USD"
  },
  "compensation": {
    "id": "step_refund_card",
    "name": "Refund Customer Charge",
    "type": "action",
    "integration": "stripe",
    "action_type": "refund",
    "config": {
      "charge_id": "{{ steps.step_charge_card.output.charge_id }}",
      "reason": "workflow_rollback"
    }
  }
}
```

### 3.2 Dynamic Template Interpolation
Compensation steps have full access to:
- The execution's original trigger event payload (`{{ event.payload.* }}`)
- The original step's output (`{{ steps.<step_id>.output.* }}`)
- Upstream steps' outputs

### 3.3 Triggering Compensation
Saga compensation is triggered under three conditions:
1. **Automatic on Step Failure**: When a step fails and all configured retry attempts have been exhausted.
2. **Automatic on Approval Rejection / Expiration**: When an approval step is rejected or times out.
3. **Manual Operator Rollback**: An authorized operator calls `POST /api/v1/executions/{id}/compensate` with an optional reason.

---

## 4. Irreversible Actions & Non-Compensatable Operations

> [!CAUTION]
> In distributed systems, certain side-effects are physically or contractually irreversible. The workflow author is responsible for designing workflows with these limitations in mind.

### External Actions That Cannot Be Safely Compensated
1. **External Communication**:
   - **Emails (SMTP, SendGrid, Postmark)**: Once delivered to recipient mailboxes, an email cannot be un-sent or revoked. Compensation can only send an apology or corrective follow-up email.
   - **SMS & Mobile Push (Twilio, Firebase Cloud Messaging)**: Cannot be recalled once transmitted across cellular networks.
   - **Chat Notifications (Slack, Discord, Microsoft Teams)**: While message deletion APIs exist, chat messages are often read or forwarded immediately upon delivery.
2. **External Financial Mutations**:
   - **Non-Refundable Bank Wires (SWIFT, Fedwire, ACH Credits)**: Completed wire transfers cannot be programmatically pulled back without inter-bank manual dispute procedures.
   - **Cryptocurrency / Blockchain Transfers**: Block transactions are immutable and cannot be undone without counter-transactions signed by the counterparty.
3. **Third-Party External Webhooks & API Sinks**:
   - External webhooks sent to vendor systems that do not expose idempotent undo/cancellation endpoints.
4. **Destructive Physical Operations**:
   - Hardware triggering, factory line robotics, automated provisioning with non-refundable billing triggers.

### Recommended Architectural Best Practices
- **Reserve First, Commit Later**: Use two-phase reserves (e.g., Stripe PaymentIntent `capture_method: manual` or inventory reservations) before performing irreversible side-effects.
- **Place Approvals Before Irreversible Steps**: Position human approval steps *prior* to executing non-compensatable external actions.
- **Idempotent Compensations**: Always write compensation steps to be idempotent (e.g. check if a refund was already processed before issuing a new one).

---

## 5. REST API Reference

### 5.1 Approvals

| Method | Endpoint | Description | Scopes Required |
|---|---|---|---|
| `GET` | `/api/v1/approvals` | List approvals with filter by status, workflow, execution | `approvals:read` |
| `GET` | `/api/v1/approvals/{id}` | Get approval details | `approvals:read` |
| `POST` | `/api/v1/approvals/{id}/approve` | Approve an approval step | `approvals:write` |
| `POST` | `/api/v1/approvals/{id}/reject` | Reject an approval step | `approvals:write` |
| `GET` | `/api/v1/approval-tokens/{token}` | Public token preview (safe fields only) | None (Token auth) |
| `POST` | `/api/v1/approval-tokens/{token}` | Decide approval via one-click token | None (Token auth) |

### 5.2 Operator Saga Rollback

| Method | Endpoint | Description | Scopes Required |
|---|---|---|---|
| `POST` | `/api/v1/executions/{id}/compensate` | Trigger manual saga compensation | `executions:compensate` |

---

## 6. Database Migrations

The database schema is upgraded via migration `000007_sagas_compensation_and_approvals.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS workflow_approvals (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL DEFAULT '',
    execution_id VARCHAR(64) NOT NULL,
    step_id VARCHAR(64) NOT NULL,
    workflow_id VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    approver_roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    approver_users JSONB NOT NULL DEFAULT '[]'::jsonb,
    timeout_seconds INT NOT NULL DEFAULT 86400,
    escalation_role VARCHAR(64) NOT NULL DEFAULT '',
    token_hash VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    decided_by VARCHAR(128) NOT NULL DEFAULT '',
    decision_note TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_approvals_tenant_status ON workflow_approvals (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_workflow_approvals_token_hash ON workflow_approvals (token_hash);
CREATE INDEX IF NOT EXISTS idx_workflow_approvals_exec_step ON workflow_approvals (execution_id, step_id);
```
