import type {
  Credential,
  CredentialCreateInput,
  DeadLetterRecord,
  EventIngestInput,
  EventRecord,
  Execution,
  HealthResponse,
  IntegrationMetadata,
  OverviewResponse,
  ReadyResponse,
  Webhook,
  WebhookCreateInput,
  Workflow,
  WorkflowCreateInput,
  WorkflowTestInput,
  WorkflowTestResponse
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

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
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
      if (response.status === 503 || errorMsg.includes('ECONNREFUSED') || errorMsg.includes('unreachable')) {
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

  public async updateWorkflow(
    id: string,
    input: WorkflowCreateInput
  ): Promise<Workflow> {
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

  public async testWorkflow(
    id: string,
    input: WorkflowTestInput
  ): Promise<WorkflowTestResponse> {
    return this.request<WorkflowTestResponse>(
      `/api/v1/workflows/${encodeURIComponent(id)}/test`,
      {
        method: 'POST',
        body: JSON.stringify(input)
      }
    );
  }

  // --- Integrations Catalog ---
  public async listIntegrations(): Promise<IntegrationMetadata[]> {
    const data = await this.request<IntegrationMetadata[]>('/api/v1/integrations');
    return Array.isArray(data) ? data : [];
  }

  public async getIntegration(id: string): Promise<IntegrationMetadata> {
    return this.request<IntegrationMetadata>(
      `/api/v1/integrations/${encodeURIComponent(id)}`
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

  public async retryExecution(id: string): Promise<void> {
    await this.request<void>(`/api/v1/executions/${encodeURIComponent(id)}/retry`, {
      method: 'POST'
    });
  }

  // --- Dead-Letter Failures ---
  public async listFailures(includeDiscarded = false): Promise<DeadLetterRecord[]> {
    const qs = includeDiscarded ? '?include_discarded=true' : '';
    const data = await this.request<DeadLetterRecord[]>(`/api/v1/failures${qs}`);
    return Array.isArray(data) ? data : [];
  }

  public async getFailure(id: string): Promise<DeadLetterRecord> {
    return this.request<DeadLetterRecord>(
      `/api/v1/failures/${encodeURIComponent(id)}`
    );
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
}

export const api = new KerobApiClient();
