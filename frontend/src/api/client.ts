import type {
  Credential,
  CredentialCreateInput,
  DeadLetterRecord,
  EventIngestInput,
  EventRecord,
  Execution,
  HealthResponse,
  IntegrationMetadata,
  IntegrationSchemaResponse,
  OverviewResponse,
  ReadyResponse,
  Webhook,
  WebhookCreateInput,
  Workflow,
  WorkflowCreateInput,
  WorkflowTestInput,
  WorkflowTestResponse,
  Role,
  Tenant,
  Membership,
  APIToken,
  AuditLog,
  Identity,
  ConnectorManifest,
  ConnectorHealthStatus,
  OAuthAuthorizeResponse,
  EventSchema,
  StreamPublishInput,
  StreamPublishResult,
  StreamConsumerOffset,
  StreamDeadLetterEvent,
  StreamReplayResult,
  Approval,
  AIProposal,
  AIStatusResponse,
  AIAcceptProposalResult
} from './types';

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly endpoint: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

const STORAGE_KEY_TOKEN = 'kerob_api_token';
const STORAGE_KEY_BASE_URL = 'kerob_api_base_url';

export type EngineStatusListener = (online: boolean, error?: string) => void;

export class KerobApiClient {
  private token: string = '';
  private baseUrl: string = '';
  private statusListeners: EngineStatusListener[] = [];

  constructor() {
    if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
      try {
        this.token = window.localStorage.getItem(STORAGE_KEY_TOKEN) || '';
        this.baseUrl = window.localStorage.getItem(STORAGE_KEY_BASE_URL) || '';
      } catch {
        // Storage access restricted or unavailable
      }
    }
  }

  public onStatusChange(listener: EngineStatusListener): () => void {
    this.statusListeners.push(listener);
    return () => {
      this.statusListeners = this.statusListeners.filter((l) => l !== listener);
    };
  }

  private notifyStatus(online: boolean, error?: string): void {
    for (const listener of this.statusListeners) {
      try {
        listener(online, error);
      } catch {
        // ignore listener errors
      }
    }
  }

  public getToken(): string {
    return this.token;
  }

  public setToken(token: string): void {
    this.token = token.trim();
    if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
      try {
        if (this.token) {
          window.localStorage.setItem(STORAGE_KEY_TOKEN, this.token);
        } else {
          window.localStorage.removeItem(STORAGE_KEY_TOKEN);
        }
      } catch {
        // Storage access restricted
      }
    }
  }

  public getBaseUrl(): string {
    return this.baseUrl;
  }

  public setBaseUrl(url: string): void {
    this.baseUrl = url.trim().replace(/\/+$/, '');
    if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
      try {
        if (this.baseUrl) {
          window.localStorage.setItem(STORAGE_KEY_BASE_URL, this.baseUrl);
        } else {
          window.localStorage.removeItem(STORAGE_KEY_BASE_URL);
        }
      } catch {
        // Storage access restricted
      }
    }
  }

  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const headers = new Headers(options.headers || {});

    if (this.token && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${this.token}`);
    }

    if (!headers.has('Content-Type') && options.body && typeof options.body === 'string') {
      headers.set('Content-Type', 'application/json');
    }

    let response: Response;
    try {
      response = await fetch(url, {
        ...options,
        headers
      });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Network request failed';
      this.notifyStatus(false, msg);
      throw new ApiError(0, `Network error on ${endpoint}: ${msg}`, endpoint);
    }

    if (!response.ok) {
      let errorMsg = response.statusText;
      try {
        const errorJson = await response.json();
        if (errorJson && typeof errorJson.error === 'string') {
          errorMsg = errorJson.error;
        }
      } catch {
        // use default statusText
      }
      if (
        response.status === 503 ||
        errorMsg.includes('ECONNREFUSED') ||
        errorMsg.includes('unreachable')
      ) {
        this.notifyStatus(false, errorMsg);
      }
      throw new ApiError(response.status, errorMsg, endpoint);
    }

    // Success response indicates engine is active
    this.notifyStatus(true);

    if (response.status === 204) {
      return null as T;
    }

    const contentType = response.headers.get('content-type') || '';
    if (contentType.includes('text/plain')) {
      return (await response.text()) as unknown as T;
    }

    return (await response.json()) as T;
  }

  // --- Overview ---
  public async getOverview(): Promise<OverviewResponse> {
    return this.request<OverviewResponse>('/api/v1/overview');
  }

  // --- Workflows ---
  public async listWorkflows(): Promise<Workflow[]> {
    const data = await this.request<Workflow[]>('/api/v1/workflows');
    return Array.isArray(data) ? data : [];
  }

  public async getWorkflow(id: string): Promise<Workflow> {
    return this.request<Workflow>(`/api/v1/workflows/${encodeURIComponent(id)}`);
  }

  public async createWorkflow(input: WorkflowCreateInput): Promise<Workflow> {
    return this.request<Workflow>('/api/v1/workflows', {
      method: 'POST',
      body: JSON.stringify(input)
    });
  }

  public async updateWorkflow(id: string, input: WorkflowCreateInput): Promise<Workflow> {
    return this.request<Workflow>(`/api/v1/workflows/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(input)
    });
  }

  public async patchWorkflow(
    id: string,
    patch: { name?: string; description?: string; enabled?: boolean }
  ): Promise<Workflow> {
    return this.request<Workflow>(`/api/v1/workflows/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(patch)
    });
  }

  public async deleteWorkflow(id: string): Promise<void> {
    await this.request<void>(`/api/v1/workflows/${encodeURIComponent(id)}`, {
      method: 'DELETE'
    });
  }

  public async testWorkflow(id: string, input: WorkflowTestInput): Promise<WorkflowTestResponse> {
    return this.request<WorkflowTestResponse>(`/api/v1/workflows/${encodeURIComponent(id)}/test`, {
      method: 'POST',
      body: JSON.stringify(input)
    });
  }

  // --- Integrations Catalog ---
  public async listIntegrations(): Promise<IntegrationMetadata[]> {
    const data = await this.request<IntegrationMetadata[]>('/api/v1/integrations');
    return Array.isArray(data) ? data : [];
  }

  public async getIntegration(id: string): Promise<IntegrationMetadata> {
    return this.request<IntegrationMetadata>(`/api/v1/integrations/${encodeURIComponent(id)}`);
  }

  public async getIntegrationSchema(
    id: string,
    actionType?: string
  ): Promise<IntegrationSchemaResponse> {
    const query = actionType ? `?action_type=${encodeURIComponent(actionType)}` : '';
    return this.request<IntegrationSchemaResponse>(
      `/api/v1/integrations/${encodeURIComponent(id)}/schema${query}`
    );
  }

  // --- Credentials ---
  public async listCredentials(): Promise<Credential[]> {
    const data = await this.request<Credential[]>('/api/v1/credentials');
    return Array.isArray(data) ? data : [];
  }

  public async createCredential(input: CredentialCreateInput): Promise<Credential> {
    return this.request<Credential>('/api/v1/credentials', {
      method: 'POST',
      body: JSON.stringify(input)
    });
  }

  public async deleteCredential(id: string): Promise<void> {
    await this.request<void>(`/api/v1/credentials/${encodeURIComponent(id)}`, {
      method: 'DELETE'
    });
  }

  // --- Connector SDK & OAuth ---
  public async listConnectors(): Promise<ConnectorManifest[]> {
    const data = await this.request<ConnectorManifest[]>('/api/v1/connectors');
    return Array.isArray(data) ? data : [];
  }

  public async getConnector(id: string): Promise<ConnectorManifest> {
    return this.request<ConnectorManifest>(`/api/v1/connectors/${encodeURIComponent(id)}`);
  }

  public async checkConnectorHealth(id: string, credentialId?: string): Promise<ConnectorHealthStatus> {
    const query = credentialId ? `?credential_id=${encodeURIComponent(credentialId)}` : '';
    return this.request<ConnectorHealthStatus>(`/api/v1/connectors/${encodeURIComponent(id)}/health${query}`);
  }

  public async authorizeOAuth(connectorId: string, redirectUri?: string): Promise<OAuthAuthorizeResponse> {
    const query = redirectUri ? `?redirect_uri=${encodeURIComponent(redirectUri)}` : '';
    return this.request<OAuthAuthorizeResponse>(`/api/v1/oauth/${encodeURIComponent(connectorId)}/authorize${query}`);
  }

  public async callbackOAuth(connectorId: string, state: string, code: string, redirectUri?: string): Promise<Credential> {
    return this.request<Credential>(`/api/v1/oauth/${encodeURIComponent(connectorId)}/callback`, {
      method: 'POST',
      body: JSON.stringify({ state, code, redirect_uri: redirectUri })
    });
  }

  public async refreshOAuthToken(connectorId: string, credentialId: string): Promise<Credential> {
    return this.request<Credential>(`/api/v1/oauth/${encodeURIComponent(connectorId)}/refresh`, {
      method: 'POST',
      body: JSON.stringify({ credential_id: credentialId })
    });
  }

  public async revokeOAuthToken(connectorId: string, credentialId: string): Promise<{ status: string; message: string }> {
    return this.request<{ status: string; message: string }>(`/api/v1/oauth/${encodeURIComponent(connectorId)}/revoke`, {
      method: 'POST',
      body: JSON.stringify({ credential_id: credentialId })
    });
  }

  // --- Schema Registry & Event Streaming ---
  public async registerSchema(input: {
    name: string;
    schema_type?: 'json_schema' | 'protobuf';
    definition: string;
    compatibility?: 'BACKWARD' | 'FORWARD' | 'FULL' | 'NONE';
    description?: string;
  }): Promise<EventSchema> {
    return this.request<EventSchema>('/api/v1/schemas', {
      method: 'POST',
      body: JSON.stringify(input)
    });
  }

  public async listSchemas(): Promise<EventSchema[]> {
    const data = await this.request<EventSchema[]>('/api/v1/schemas');
    return Array.isArray(data) ? data : [];
  }

  public async getSchema(name: string): Promise<EventSchema> {
    return this.request<EventSchema>(`/api/v1/schemas/${encodeURIComponent(name)}`);
  }

  public async listSchemaVersions(name: string): Promise<EventSchema[]> {
    const data = await this.request<EventSchema[]>(`/api/v1/schemas/${encodeURIComponent(name)}/versions`);
    return Array.isArray(data) ? data : [];
  }

  public async validatePayloadAgainstSchema(name: string, payload: unknown): Promise<{ valid: boolean; errors: unknown[] }> {
    return this.request<{ valid: boolean; errors: unknown[] }>(`/api/v1/schemas/${encodeURIComponent(name)}/validate`, {
      method: 'POST',
      body: JSON.stringify({ payload })
    });
  }

  public async publishToStream(input: StreamPublishInput): Promise<StreamPublishResult> {
    return this.request<StreamPublishResult>('/api/v1/streams/publish', {
      method: 'POST',
      body: JSON.stringify(input)
    });
  }

  public async getStreamOffsets(groupId?: string): Promise<StreamConsumerOffset[]> {
    const query = groupId ? `?group_id=${encodeURIComponent(groupId)}` : '';
    const data = await this.request<StreamConsumerOffset[]>(`/api/v1/streams/offsets${query}`);
    return Array.isArray(data) ? data : [];
  }

  public async replayStream(filter: Record<string, unknown>): Promise<StreamReplayResult> {
    return this.request<StreamReplayResult>('/api/v1/streams/replay', {
      method: 'POST',
      body: JSON.stringify(filter)
    });
  }

  public async listStreamDLQ(filter?: { topic?: string; resolved?: boolean; limit?: number; offset?: number }): Promise<StreamDeadLetterEvent[]> {
    const params = new URLSearchParams();
    if (filter?.topic) params.set('topic', filter.topic);
    if (filter?.resolved !== undefined) params.set('resolved', String(filter.resolved));
    if (filter?.limit) params.set('limit', String(filter.limit));
    if (filter?.offset) params.set('offset', String(filter.offset));
    const query = params.toString() ? `?${params.toString()}` : '';
    const data = await this.request<StreamDeadLetterEvent[]>(`/api/v1/streams/dlq${query}`);
    return Array.isArray(data) ? data : [];
  }

  public async retryStreamDLQ(id: string): Promise<{ event: unknown; executions: unknown[]; status: string }> {
    return this.request<{ event: unknown; executions: unknown[]; status: string }>(`/api/v1/streams/dlq/${encodeURIComponent(id)}/retry`, {
      method: 'POST'
    });
  }

  // --- Executions ---
  public async listExecutions(filter?: {
    workflow_id?: string;
    status?: string;
    limit?: number;
    offset?: number;
  }): Promise<Execution[]> {
    const params = new URLSearchParams();
    if (filter?.workflow_id) params.set('workflow_id', filter.workflow_id);
    if (filter?.status) params.set('status', filter.status);
    if (filter?.limit) params.set('limit', String(filter.limit));
    if (filter?.offset) params.set('offset', String(filter.offset));
    const qs = params.toString();
    const endpoint = `/api/v1/executions${qs ? `?${qs}` : ''}`;
    const data = await this.request<Execution[]>(endpoint);
    return Array.isArray(data) ? data : [];
  }

  public async getExecution(id: string): Promise<Execution> {
    return this.request<Execution>(`/api/v1/executions/${encodeURIComponent(id)}`);
  }

  public async retryExecution(id: string): Promise<Execution> {
    return this.request<Execution>(`/api/v1/executions/${encodeURIComponent(id)}/retry`, {
      method: 'POST'
    });
  }

  public async cancelExecution(id: string): Promise<Execution> {
    return this.request<Execution>(`/api/v1/executions/${encodeURIComponent(id)}/cancel`, {
      method: 'POST'
    });
  }

  public async pauseExecution(id: string): Promise<Execution> {
    return this.request<Execution>(`/api/v1/executions/${encodeURIComponent(id)}/pause`, {
      method: 'POST'
    });
  }

  public async resumeExecution(id: string): Promise<Execution> {
    return this.request<Execution>(`/api/v1/executions/${encodeURIComponent(id)}/resume`, {
      method: 'POST'
    });
  }

  public async compensateExecution(id: string, reason?: string): Promise<Execution> {
    return this.request<Execution>(`/api/v1/executions/${encodeURIComponent(id)}/compensate`, {
      method: 'POST',
      body: JSON.stringify({ reason })
    });
  }

  // --- Sagas & Human Approvals ---
  public async listApprovals(filter?: {
    workflow_id?: string;
    execution_id?: string;
    status?: string;
    limit?: number;
    offset?: number;
  }): Promise<Approval[]> {
    const params = new URLSearchParams();
    if (filter?.workflow_id) params.set('workflow_id', filter.workflow_id);
    if (filter?.execution_id) params.set('execution_id', filter.execution_id);
    if (filter?.status) params.set('status', filter.status);
    if (filter?.limit) params.set('limit', String(filter.limit));
    if (filter?.offset) params.set('offset', String(filter.offset));
    const qs = params.toString();
    const data = await this.request<Approval[]>(`/api/v1/approvals${qs ? `?${qs}` : ''}`);
    return Array.isArray(data) ? data : [];
  }

  public async getApproval(id: string): Promise<Approval> {
    return this.request<Approval>(`/api/v1/approvals/${encodeURIComponent(id)}`);
  }

  public async approveApproval(id: string, note?: string): Promise<Approval> {
    return this.request<Approval>(`/api/v1/approvals/${encodeURIComponent(id)}/approve`, {
      method: 'POST',
      body: JSON.stringify({ note })
    });
  }

  public async rejectApproval(id: string, note?: string): Promise<Approval> {
    return this.request<Approval>(`/api/v1/approvals/${encodeURIComponent(id)}/reject`, {
      method: 'POST',
      body: JSON.stringify({ note })
    });
  }

  public async getApprovalByToken(token: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>(`/api/v1/approval-tokens/${encodeURIComponent(token)}`);
  }

  public async decideApprovalByToken(
    token: string,
    decision: 'approve' | 'reject',
    note?: string,
    approver?: string
  ): Promise<Approval> {
    return this.request<Approval>(`/api/v1/approval-tokens/${encodeURIComponent(token)}`, {
      method: 'POST',
      body: JSON.stringify({ decision, note, approver })
    });
  }

  public getOperationsStreamUrl(): string {
    const base = this.baseUrl || (typeof window !== 'undefined' ? window.location.origin : '');
    const url = new URL('/api/v1/operations/stream', base || 'http://localhost:8080');
    if (this.token) {
      url.searchParams.set('token', this.token);
    }
    return url.toString();
  }

  // --- Dead-Letter Failures ---
  public async listFailures(includeDiscarded = false): Promise<DeadLetterRecord[]> {
    const qs = includeDiscarded ? '?include_discarded=true' : '';
    const data = await this.request<DeadLetterRecord[]>(`/api/v1/failures${qs}`);
    return Array.isArray(data) ? data : [];
  }

  public async getFailure(id: string): Promise<DeadLetterRecord> {
    return this.request<DeadLetterRecord>(`/api/v1/failures/${encodeURIComponent(id)}`);
  }

  public async retryFailure(id: string): Promise<void> {
    await this.request<void>(`/api/v1/failures/${encodeURIComponent(id)}/retry`, {
      method: 'POST'
    });
  }

  public async discardFailure(id: string): Promise<void> {
    await this.request<void>(`/api/v1/failures/${encodeURIComponent(id)}/discard`, {
      method: 'POST'
    });
  }

  // --- Events Stream ---
  public async listEvents(filter?: {
    source_id?: string;
    event_type?: string;
    limit?: number;
    offset?: number;
  }): Promise<EventRecord[]> {
    const params = new URLSearchParams();
    if (filter?.source_id) params.set('source_id', filter.source_id);
    if (filter?.event_type) params.set('event_type', filter.event_type);
    if (filter?.limit) params.set('limit', String(filter.limit));
    if (filter?.offset) params.set('offset', String(filter.offset));
    const qs = params.toString();
    const endpoint = `/api/v1/events${qs ? `?${qs}` : ''}`;
    const data = await this.request<EventRecord[]>(endpoint);
    return Array.isArray(data) ? data : [];
  }

  public async getEvent(id: string): Promise<EventRecord> {
    return this.request<EventRecord>(`/api/v1/events/${encodeURIComponent(id)}`);
  }

  public async ingestEvent(input: EventIngestInput): Promise<EventRecord> {
    return this.request<EventRecord>('/api/v1/events', {
      method: 'POST',
      body: JSON.stringify(input)
    });
  }

  // --- Inbound Webhooks ---
  public async listWebhooks(): Promise<Webhook[]> {
    const data = await this.request<Webhook[]>('/api/v1/webhooks');
    return Array.isArray(data) ? data : [];
  }

  public async getWebhook(id: string): Promise<Webhook> {
    return this.request<Webhook>(`/api/v1/webhooks/${encodeURIComponent(id)}`);
  }

  public async createWebhook(input: WebhookCreateInput): Promise<Webhook> {
    return this.request<Webhook>('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify(input)
    });
  }

  public async deleteWebhook(id: string): Promise<void> {
    await this.request<void>(`/api/v1/webhooks/${encodeURIComponent(id)}`, {
      method: 'DELETE'
    });
  }

  // --- Health, Readiness & Metrics ---
  public async getHealth(): Promise<HealthResponse> {
    return this.request<HealthResponse>('/health');
  }

  public async getReady(): Promise<ReadyResponse> {
    return this.request<ReadyResponse>('/ready');
  }

  public async getMetricsText(): Promise<string> {
    return this.request<string>('/metrics');
  }

  // --- Multi-tenancy & RBAC ---
  public async getMe(): Promise<Identity> {
    return this.request<Identity>('/api/v1/auth/me');
  }

  public async listTenants(): Promise<Tenant[]> {
    const data = await this.request<Tenant[]>('/api/v1/tenants');
    return Array.isArray(data) ? data : [];
  }

  public async createTenant(name: string, slug?: string): Promise<Tenant> {
    return this.request<Tenant>('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({ name, slug })
    });
  }

  public async listMembers(): Promise<Membership[]> {
    const data = await this.request<Membership[]>('/api/v1/tenants/current/members');
    return Array.isArray(data) ? data : [];
  }

  public async createMember(userId: string, role: Role): Promise<Membership> {
    return this.request<Membership>('/api/v1/tenants/current/members', {
      method: 'POST',
      body: JSON.stringify({ user_id: userId, role })
    });
  }

  public async updateMemberRole(userId: string, role: Role): Promise<Membership> {
    return this.request<Membership>(`/api/v1/tenants/current/members/${encodeURIComponent(userId)}`, {
      method: 'PUT',
      body: JSON.stringify({ role })
    });
  }

  public async deleteMember(userId: string): Promise<void> {
    await this.request<void>(`/api/v1/tenants/current/members/${encodeURIComponent(userId)}`, {
      method: 'DELETE'
    });
  }

  public async listAPITokens(): Promise<APIToken[]> {
    const data = await this.request<APIToken[]>('/api/v1/tokens');
    return Array.isArray(data) ? data : [];
  }

  public async createAPIToken(name: string, scopes: string[], expiresAt?: string): Promise<APIToken> {
    return this.request<APIToken>('/api/v1/tokens', {
      method: 'POST',
      body: JSON.stringify({ name, scopes, expires_at: expiresAt })
    });
  }

  public async revokeAPIToken(id: string): Promise<void> {
    await this.request<void>(`/api/v1/tokens/${encodeURIComponent(id)}`, {
      method: 'DELETE'
    });
  }

  public async listAuditLogs(limit?: number, offset?: number): Promise<AuditLog[]> {
    const params = new URLSearchParams();
    if (limit) params.set('limit', String(limit));
    if (offset) params.set('offset', String(offset));
    const qs = params.toString();
    const data = await this.request<AuditLog[]>(`/api/v1/audit-logs${qs ? `?${qs}` : ''}`);
    return Array.isArray(data) ? data : [];
  }

  // --- Controlled AI Assistance ---
  public async getAIStatus(): Promise<AIStatusResponse> {
    return this.request<AIStatusResponse>('/api/v1/ai/status');
  }

  public async draftWorkflowAI(prompt: string, allowedConnectors?: string[]): Promise<AIProposal> {
    return this.request<AIProposal>('/api/v1/ai/draft-workflow', {
      method: 'POST',
      body: JSON.stringify({ prompt, allowed_connectors: allowedConnectors })
    });
  }

  public async mapFieldsAI(req: { source_schema?: Record<string, unknown>; target_schema?: Record<string, unknown>; sample_payload?: Record<string, unknown>; description?: string }): Promise<AIProposal> {
    return this.request<AIProposal>('/api/v1/ai/map-fields', {
      method: 'POST',
      body: JSON.stringify(req)
    });
  }

  public async generateConditionsAI(criteria: string, samplePayload?: Record<string, unknown>): Promise<AIProposal> {
    return this.request<AIProposal>('/api/v1/ai/generate-conditions', {
      method: 'POST',
      body: JSON.stringify({ criteria, sample_payload: samplePayload })
    });
  }

  public async suggestTransformationsAI(goal: string, inputPayload: Record<string, unknown>, desiredOutput?: Record<string, unknown>): Promise<AIProposal> {
    return this.request<AIProposal>('/api/v1/ai/suggest-transformations', {
      method: 'POST',
      body: JSON.stringify({ goal, input_payload: inputPayload, desired_output: desiredOutput })
    });
  }

  public async explainFailureAI(req: { workflow_id: string; execution_id: string; step_id?: string; step_type?: string; error: string; step_input?: unknown; step_output?: unknown }): Promise<AIProposal> {
    return this.request<AIProposal>('/api/v1/ai/explain-failure', {
      method: 'POST',
      body: JSON.stringify(req)
    });
  }

  public async listAIProposals(filter?: { task_type?: string; status?: string; limit?: number; offset?: number }): Promise<AIProposal[]> {
    const params = new URLSearchParams();
    if (filter?.task_type) params.set('task_type', filter.task_type);
    if (filter?.status) params.set('status', filter.status);
    if (filter?.limit) params.set('limit', String(filter.limit));
    if (filter?.offset) params.set('offset', String(filter.offset));
    const qs = params.toString();
    const data = await this.request<AIProposal[]>(`/api/v1/ai/proposals${qs ? `?${qs}` : ''}`);
    return Array.isArray(data) ? data : [];
  }

  public async getAIProposal(id: string): Promise<AIProposal> {
    return this.request<AIProposal>(`/api/v1/ai/proposals/${encodeURIComponent(id)}`);
  }

  public async acceptAIProposal(id: string, createWorkflow: boolean = false): Promise<AIAcceptProposalResult> {
    return this.request<AIAcceptProposalResult>(`/api/v1/ai/proposals/${encodeURIComponent(id)}/accept${createWorkflow ? '?create_workflow=true' : ''}`, {
      method: 'POST',
      body: JSON.stringify({ create_workflow: createWorkflow })
    });
  }

  public async rejectAIProposal(id: string, reason?: string): Promise<AIProposal> {
    return this.request<AIProposal>(`/api/v1/ai/proposals/${encodeURIComponent(id)}/reject`, {
      method: 'POST',
      body: JSON.stringify({ reason: reason || '' })
    });
  }
}

export const api = new KerobApiClient();
