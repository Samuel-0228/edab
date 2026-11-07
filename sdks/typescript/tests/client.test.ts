import { describe, it, expect, vi, beforeEach } from 'vitest';
import { KerobClient } from '../src/client.js';
import { validateWorkflowSecrets, diffWorkflow } from '../src/gitops.js';
import type { Workflow } from '../src/types.js';

describe('Kerob TypeScript SDK', () => {
  let mockFetch: ReturnType<typeof vi.fn>;
  let client: KerobClient;

  beforeEach(() => {
    mockFetch = vi.fn();
    client = new KerobClient({
      baseUrl: 'http://localhost:8080',
      token: 'test-token',
      tenantId: 'tenant-123',
      fetch: mockFetch as unknown as typeof fetch
    });
  });

  it('sends correct Authorization and X-Tenant-ID headers', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ user_id: 'u1', email: 'test@example.com' })
    });

    const me = await client.getMe();
    expect(me.user_id).toBe('u1');

    const [url, init] = mockFetch.mock.calls[0];
    expect(url).toBe('http://localhost:8080/api/v1/auth/me');
    expect(init.headers['Authorization']).toBe('Bearer test-token');
    expect(init.headers['X-Tenant-ID']).toBe('tenant-123');
  });

  it('fetches workflows with query parameters', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => [{ id: 'wf-1', name: 'Order Processing', enabled: true }]
    });

    const wfs = await client.listWorkflows({ enabled: true, limit: 10 });
    expect(wfs).toHaveLength(1);
    expect(wfs[0].id).toBe('wf-1');

    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain('enabled=true');
    expect(url).toContain('limit=10');
  });

  it('handles API errors correctly', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      headers: new Headers({ 'content-type': 'application/json' }),
      text: async () => JSON.stringify({ code: 'NOT_FOUND', error: 'Workflow not found' })
    });

    await expect(client.getWorkflow('wf-missing')).rejects.toThrow('Workflow not found');
  });

  it('scans and detects hardcoded secrets in workflows', () => {
    const safeWf: Workflow = {
      id: 'wf-safe',
      name: 'Safe Workflow',
      enabled: true,
      trigger: { type: 'webhook' },
      actions: [
        {
          id: 'step1',
          name: 'Charge Card',
          integration: 'stripe',
          credential_id: 'cred_stripe_live',
          config: {
            amount: 1000,
            webhook_secret: '{{ secret.STRIPE_WEBHOOK_SECRET }}'
          }
        }
      ]
    };

    const violationsSafe = validateWorkflowSecrets(safeWf);
    expect(violationsSafe).toHaveLength(0);

    const unsafeWf: Workflow = {
      id: 'wf-unsafe',
      name: 'Unsafe Workflow',
      enabled: true,
      trigger: { type: 'webhook' },
      actions: [
        {
          id: 'step1',
          name: 'AWS Upload',
          integration: 's3',
          config: {
            api_key: 'AKIAIOSFODNN7EXAMPLE'
          }
        }
      ]
    };

    const violationsUnsafe = validateWorkflowSecrets(unsafeWf);
    expect(violationsUnsafe.length).toBeGreaterThan(0);
    expect(violationsUnsafe[0].reason).toContain('AWS Access Key');
  });

  it('calculates diffs between local and remote workflows', () => {
    const local = {
      id: 'wf-1',
      name: 'Order Workflow v2',
      trigger: { type: 'webhook' },
      actions: [{ id: 'step1', name: 'Step 1' }, { id: 'step2', name: 'Step 2' }]
    };

    const remote: Workflow = {
      id: 'wf-1',
      name: 'Order Workflow v1',
      enabled: true,
      trigger: { type: 'webhook' },
      actions: [{ id: 'step1', name: 'Step 1' }]
    };

    const diff = diffWorkflow(local, remote);
    expect(diff.has_changes).toBe(true);
    expect(diff.changes).toEqual(
      expect.arrayContaining([
        expect.stringContaining('Name changed'),
        expect.stringContaining('Action count changed')
      ])
    );
  });

  it('fetches governance policy and verifies audit chain', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        tenant_id: 'tenant-123',
        enforcement_mode: 'enforce',
        allowed_connectors: ['http', 'telegram']
      })
    });

    const pol = await client.getGovernancePolicy();
    expect(pol.tenant_id).toBe('tenant-123');
    expect(pol.allowed_connectors).toContain('http');

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        valid: true,
        tenant_id: 'tenant-123',
        total_records: 12
      })
    });

    const chainRes = await client.verifyAuditChain();
    expect(chainRes.valid).toBe(true);
    expect(chainRes.total_records).toBe(12);
  });

  it('interacts with controlled AI assistance layer', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        enabled: true,
        provider: 'mock',
        model: 'gpt-4o',
        tenant_id: 'tenant-123',
        opted_out: false,
        max_tokens: 2000,
        timeout_seconds: 30,
        available_tasks: ['draft_workflow', 'map_fields', 'generate_conditions']
      })
    });

    const status = await client.getAIStatus();
    expect(status.enabled).toBe(true);
    expect(status.available_tasks).toContain('draft_workflow');

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 201,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        id: 'prop-101',
        tenant_id: 'tenant-123',
        task: 'draft_workflow',
        status: 'pending_approval',
        prompt: 'Sync Github issues',
        explanation: 'Proposed workflow drafted',
        validation: { valid: true },
        tokens_used: 150,
        created_at: new Date().toISOString()
      })
    });

    const proposal = await client.draftWorkflowAI({ prompt: 'Sync Github issues' });
    expect(proposal.id).toBe('prop-101');
    expect(proposal.status).toBe('pending_approval');
    expect(proposal.validation.valid).toBe(true);
  });
});
