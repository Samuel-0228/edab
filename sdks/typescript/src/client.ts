import type {
  Workflow,
  WorkflowCreateInput,
  WorkflowVersion,
  WorkflowDiff,
  Execution,
  ExecutionStep,
  DeadLetter,
  EventRecord,
  Approval,
  Tenant,
  Member,
  APIToken,
  Identity,
  GovernancePolicy,
  PolicyValidationResult,
  AuditVerificationResult,
  RetentionPruneResult,
  AIStatusResponse,
  AIProposal,
  AIDraftWorkflowRequest,
  AIMapFieldsRequest,
  AIGenerateConditionsRequest,
  AISuggestTransformationRequest,
  AIExplainFailureRequest,
  AIAcceptProposalResult
} from './types.js';

export class KerobApiError extends Error {
  constructor(
    public readonly statusCode: number,
    message: string,
    public readonly code?: string,
    public readonly details?: unknown
  ) {
    super(message);
    this.name = 'KerobApiError';
  }
}

export interface ClientOptions {
  baseUrl?: string;
  token?: string;
  tenantId?: string;
  fetch?: typeof fetch;
  headers?: Record<string, string>;
}

export class KerobClient {
  private baseUrl: string;
  private token: string;
  private tenantId: string;
  private fetchFn: typeof fetch;
  private extraHeaders: Record<string, string>;

  constructor(options: ClientOptions = {}) {
    this.baseUrl = (options.baseUrl || 'http://localhost:8080').replace(/\/+$/, '');
    this.token = options.token || '';
    this.tenantId = options.tenantId || '';
    this.fetchFn = options.fetch || (typeof fetch !== 'undefined' ? fetch.bind(globalThis) : (fetch as typeof globalThis.fetch));
    this.extraHeaders = options.headers || {};
  }

  public setToken(token: string): void {
    this.token = token;
  }

  public setTenantId(tenantId: string): void {
    this.tenantId = tenantId;
  }

  private async request<T>(
    method: string,
    path: string,
    options: { query?: Record<string, string | number | boolean | undefined>; body?: unknown } = {}
  ): Promise<T> {
    const url = new URL(this.baseUrl + path);
    if (options.query) {
      for (const [k, v] of Object.entries(options.query)) {
        if (v !== undefined) {
          url.searchParams.set(k, String(v));
        }
      }
    }

    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...this.extraHeaders
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }
    if (this.tenantId) {
      headers['X-Tenant-ID'] = this.tenantId;
    }

    let reqBody: string | undefined;
    if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      reqBody = typeof options.body === 'string' ? options.body : JSON.stringify(options.body);
    }

    const res = await this.fetchFn(url.toString(), {
      method,
      headers,
      body: reqBody
    });

    if (!res.ok) {
      const errText = await res.text();
      let msg = errText;
      let code: string | undefined;
      let details: unknown;
      try {
        const parsed = JSON.parse(errText);
        msg = parsed.error || parsed.message || errText;
        code = parsed.code;
        details = parsed.details;
      } catch {
        // use raw text
      }
      throw new KerobApiError(res.status, msg, code, details);
    }

    if (res.status === 204) {
      return undefined as unknown as T;
    }

    const contentType = res.headers.get('content-type') || '';
    if (contentType.includes('application/json')) {
      return (await res.json()) as T;
    }
    return (await res.text()) as unknown as T;
  }

  // --- Auth & Tenant ---
  public async getMe(): Promise<Identity> {
    return this.request<Identity>('GET', '/api/v1/auth/me');
  }

  public async listTenants(): Promise<Tenant[]> {
    return this.request<Tenant[]>('GET', '/api/v1/tenants');
  }

  public async getTenant(id: string): Promise<Tenant> {
    return this.request<Tenant>('GET', `/api/v1/tenants/${encodeURIComponent(id)}`);
  }

  public async createTenant(name: string, slug: string): Promise<Tenant> {
    return this.request<Tenant>('POST', '/api/v1/tenants', { body: { name, slug } });
  }

  public async listMembers(): Promise<Member[]> {
    return this.request<Member[]>('GET', '/api/v1/members');
  }

  public async listAPITokens(): Promise<APIToken[]> {
    return this.request<APIToken[]>('GET', '/api/v1/tokens');
  }

  public async createAPIToken(name: string, scopes: string[]): Promise<APIToken> {
    return this.request<APIToken>('POST', '/api/v1/tokens', { body: { name, scopes } });
  }

  public async revokeAPIToken(id: string): Promise<void> {
    return this.request<void>('DELETE', `/api/v1/tokens/${encodeURIComponent(id)}`);
  }

  // --- Workflows ---
  public async listWorkflows(filter?: { enabled?: boolean; tag?: string; limit?: number; offset?: number }): Promise<Workflow[]> {
    return this.request<Workflow[]>('GET', '/api/v1/workflows', { query: filter });
  }

  public async getWorkflow(id: string): Promise<Workflow> {
    return this.request<Workflow>('GET', `/api/v1/workflows/${encodeURIComponent(id)}`);
  }

  public async createWorkflow(input: WorkflowCreateInput): Promise<Workflow> {
    return this.request<Workflow>('POST', '/api/v1/workflows', { body: input });
  }

  public async updateWorkflow(id: string, input: WorkflowCreateInput): Promise<Workflow> {
    return this.request<Workflow>('PUT', `/api/v1/workflows/${encodeURIComponent(id)}`, { body: input });
  }

  public async patchWorkflow(id: string, patch: Record<string, unknown>): Promise<Workflow> {
    return this.request<Workflow>('PATCH', `/api/v1/workflows/${encodeURIComponent(id)}`, { body: patch });
  }

  public async deleteWorkflow(id: string): Promise<void> {
    return this.request<void>('DELETE', `/api/v1/workflows/${encodeURIComponent(id)}`);
  }

  public async testWorkflow(id: string, payload: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/workflows/${encodeURIComponent(id)}/test`, { body: payload });
  }

  public async listWorkflowVersions(id: string): Promise<WorkflowVersion[]> {
    return this.request<WorkflowVersion[]>('GET', `/api/v1/workflows/${encodeURIComponent(id)}/versions`);
  }

  public async getWorkflowVersion(id: string, version: number): Promise<WorkflowVersion> {
    return this.request<WorkflowVersion>('GET', `/api/v1/workflows/${encodeURIComponent(id)}/versions/${version}`);
  }

  public async rollbackWorkflow(id: string, version: number): Promise<Workflow> {
    const res = await this.request<{ workflow: Workflow }>('POST', `/api/v1/workflows/${encodeURIComponent(id)}/rollback/${version}`);
    return res.workflow;
  }

  public async diffWorkflowVersions(id: string, from: number, to: number): Promise<WorkflowDiff> {
    return this.request<WorkflowDiff>('GET', `/api/v1/workflows/${encodeURIComponent(id)}/diff`, { query: { from, to } });
  }

  public async exportWorkflow(id: string, format: 'json' | 'yaml' = 'json'): Promise<string> {
    return this.request<string>('GET', `/api/v1/workflows/${encodeURIComponent(id)}/export`, { query: { format } });
  }

  public async importWorkflow(definition: WorkflowCreateInput | string): Promise<Workflow> {
    return this.request<Workflow>('POST', '/api/v1/workflows/import', { body: definition });
  }

  // --- Executions ---
  public async listExecutions(filter?: { workflow_id?: string; status?: string; limit?: number; offset?: number }): Promise<Execution[]> {
    return this.request<Execution[]>('GET', '/api/v1/executions', { query: filter });
  }

  public async getExecution(id: string): Promise<Execution> {
    return this.request<Execution>('GET', `/api/v1/executions/${encodeURIComponent(id)}`);
  }

  public async retryExecution(id: string): Promise<Execution> {
    return this.request<Execution>('POST', `/api/v1/executions/${encodeURIComponent(id)}/retry`);
  }

  public async pauseExecution(id: string): Promise<Execution> {
    return this.request<Execution>('POST', `/api/v1/executions/${encodeURIComponent(id)}/pause`);
  }

  public async resumeExecution(id: string): Promise<Execution> {
    return this.request<Execution>('POST', `/api/v1/executions/${encodeURIComponent(id)}/resume`);
  }

  public async cancelExecution(id: string): Promise<Execution> {
    return this.request<Execution>('POST', `/api/v1/executions/${encodeURIComponent(id)}/cancel`);
  }

  public async compensateExecution(id: string, reason?: string): Promise<Execution> {
    return this.request<Execution>('POST', `/api/v1/executions/${encodeURIComponent(id)}/compensate`, { body: { reason } });
  }

  public async getExecutionLogs(id: string): Promise<ExecutionStep[]> {
    const exec = await this.getExecution(id);
    return exec.steps || [];
  }

  // --- Approvals ---
  public async listApprovals(filter?: { workflow_id?: string; execution_id?: string; status?: string; limit?: number; offset?: number }): Promise<Approval[]> {
    return this.request<Approval[]>('GET', '/api/v1/approvals', { query: filter });
  }

  public async getApproval(id: string): Promise<Approval> {
    return this.request<Approval>('GET', `/api/v1/approvals/${encodeURIComponent(id)}`);
  }

  public async approve(id: string, note?: string): Promise<Approval> {
    return this.request<Approval>('POST', `/api/v1/approvals/${encodeURIComponent(id)}/approve`, { body: { note } });
  }

  public async reject(id: string, note?: string): Promise<Approval> {
    return this.request<Approval>('POST', `/api/v1/approvals/${encodeURIComponent(id)}/reject`, { body: { note } });
  }

  public async getApprovalByToken(token: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/approval-tokens/${encodeURIComponent(token)}`);
  }

  public async decideApprovalByToken(token: string, decision: 'approve' | 'reject', note?: string, approver?: string): Promise<Approval> {
    return this.request<Approval>('POST', `/api/v1/approval-tokens/${encodeURIComponent(token)}`, { body: { decision, note, approver } });
  }

  // --- Events ---
  public async listEvents(limit?: number, offset?: number): Promise<EventRecord[]> {
    return this.request<EventRecord[]>('GET', '/api/v1/events', { query: { limit, offset } });
  }

  public async getEvent(id: string): Promise<EventRecord> {
    return this.request<EventRecord>('GET', `/api/v1/events/${encodeURIComponent(id)}`);
  }

  public async ingestEvent(sourceId: string, eventType: string, payload: unknown): Promise<EventRecord> {
    return this.request<EventRecord>('POST', '/api/v1/events', { body: { source_id: sourceId, event_type: eventType, payload } });
  }

  public async replayEvent(id: string, bypassIdempotency = false): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/events/${encodeURIComponent(id)}/replay`, { body: { bypass_idempotency: bypassIdempotency } });
  }

  // --- Failures (DLQ) ---
  public async listFailures(includeDiscarded = false): Promise<DeadLetter[]> {
    return this.request<DeadLetter[]>('GET', '/api/v1/failures', { query: { include_discarded: includeDiscarded } });
  }

  public async getFailure(id: string): Promise<DeadLetter> {
    return this.request<DeadLetter>('GET', `/api/v1/failures/${encodeURIComponent(id)}`);
  }

  public async retryFailure(id: string): Promise<Execution> {
    return this.request<Execution>('POST', `/api/v1/failures/${encodeURIComponent(id)}/retry`);
  }

  public async discardFailure(id: string): Promise<void> {
    return this.request<void>('POST', `/api/v1/failures/${encodeURIComponent(id)}/discard`);
  }

  public async bulkRetryFailures(ids: string[]): Promise<number> {
    const res = await this.request<{ retried_count: number }>('POST', '/api/v1/failures/retry-bulk', { body: { ids } });
    return res.retried_count;
  }

  public async bulkDiscardFailures(ids: string[]): Promise<number> {
    const res = await this.request<{ discarded_count: number }>('POST', '/api/v1/failures/discard-bulk', { body: { ids } });
    return res.discarded_count;
  }

  // --- Governance & Policy Engine ---
  public async getGovernancePolicy(): Promise<GovernancePolicy> {
    return this.request<GovernancePolicy>('GET', '/api/v1/governance/policy');
  }

  public async updateGovernancePolicy(policy: Partial<GovernancePolicy>): Promise<GovernancePolicy> {
    return this.request<GovernancePolicy>('PUT', '/api/v1/governance/policy', { body: policy });
  }

  public async validateWorkflowPolicy(workflow: unknown): Promise<PolicyValidationResult> {
    return this.request<PolicyValidationResult>('POST', '/api/v1/governance/policy/validate', { body: workflow });
  }

  public async verifyAuditChain(): Promise<AuditVerificationResult> {
    return this.request<AuditVerificationResult>('GET', '/api/v1/governance/audit/verify');
  }

  public async exportAuditLogs(format: 'json' | 'csv' = 'json', filter?: Record<string, string>): Promise<string> {
    return this.request<string>('GET', '/api/v1/audit-logs/export', { query: { format, ...filter } });
  }

  public async pruneRetention(): Promise<RetentionPruneResult> {
    return this.request<RetentionPruneResult>('POST', '/api/v1/governance/retention/prune');
  }

  // --- Controlled AI Assistance ---
  public async getAIStatus(): Promise<AIStatusResponse> {
    return this.request<AIStatusResponse>('GET', '/api/v1/ai/status');
  }

  public async draftWorkflowAI(req: AIDraftWorkflowRequest): Promise<AIProposal> {
    return this.request<AIProposal>('POST', '/api/v1/ai/draft-workflow', { body: req });
  }

  public async mapFieldsAI(req: AIMapFieldsRequest): Promise<AIProposal> {
    return this.request<AIProposal>('POST', '/api/v1/ai/map-fields', { body: req });
  }

  public async generateConditionsAI(req: AIGenerateConditionsRequest): Promise<AIProposal> {
    return this.request<AIProposal>('POST', '/api/v1/ai/generate-conditions', { body: req });
  }

  public async suggestTransformationsAI(req: AISuggestTransformationRequest): Promise<AIProposal> {
    return this.request<AIProposal>('POST', '/api/v1/ai/suggest-transformations', { body: req });
  }

  public async explainFailureAI(req: AIExplainFailureRequest): Promise<AIProposal> {
    return this.request<AIProposal>('POST', '/api/v1/ai/explain-failure', { body: req });
  }

  public async listAIProposals(params?: { task?: string; status?: string; limit?: number; offset?: number }): Promise<AIProposal[]> {
    return this.request<AIProposal[]>('GET', '/api/v1/ai/proposals', {
      query: {
        task_type: params?.task,
        status: params?.status,
        limit: params?.limit ? String(params.limit) : undefined,
        offset: params?.offset ? String(params.offset) : undefined
      }
    });
  }

  public async getAIProposal(id: string): Promise<AIProposal> {
    return this.request<AIProposal>('GET', `/api/v1/ai/proposals/${encodeURIComponent(id)}`);
  }

  public async acceptAIProposal(id: string, createWorkflow: boolean = false): Promise<AIAcceptProposalResult> {
    return this.request<AIAcceptProposalResult>('POST', `/api/v1/ai/proposals/${encodeURIComponent(id)}/accept`, {
      query: createWorkflow ? { create_workflow: 'true' } : undefined,
      body: { create_workflow: createWorkflow }
    });
  }

  public async rejectAIProposal(id: string, reason?: string): Promise<AIProposal> {
    return this.request<AIProposal>('POST', `/api/v1/ai/proposals/${encodeURIComponent(id)}/reject`, {
      body: { reason: reason || '' }
    });
  }
}
