import { describe, it, expect, beforeEach, vi } from 'vitest';
import { KerobApiClient } from './client';

describe('KerobApiClient', () => {
  let client: KerobApiClient;
  const storageMap = new Map<string, string>();

  beforeEach(() => {
    storageMap.clear();
    Object.defineProperty(window, 'localStorage', {
      value: {
        getItem: (key: string) => storageMap.get(key) ?? null,
        setItem: (key: string, val: string) => storageMap.set(key, val),
        removeItem: (key: string) => storageMap.delete(key),
        clear: () => storageMap.clear()
      },
      writable: true,
      configurable: true
    });
    client = new KerobApiClient();
  });

  it('manages auth tokens in memory and storage', () => {
    expect(client.getToken()).toBe('');
    client.setToken('kerob-secret-token-123');
    expect(client.getToken()).toBe('kerob-secret-token-123');
    expect(window.localStorage.getItem('kerob_api_token')).toBe('kerob-secret-token-123');

    client.setToken('');
    expect(client.getToken()).toBe('');
    expect(window.localStorage.getItem('kerob_api_token')).toBeNull();
  });

  it('formats base URLs cleanly without trailing slashes', () => {
    client.setBaseUrl('http://localhost:8080///');
    expect(client.getBaseUrl()).toBe('http://localhost:8080');

    client.setBaseUrl('');
    expect(client.getBaseUrl()).toBe('');
  });

  it('attaches Authorization header when token is configured', async () => {
    client.setToken('my-token');
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ status: 'operational', active_workflows: 3 })
    });
    vi.stubGlobal('fetch', mockFetch);

    const overview = await client.getOverview();
    expect(overview.status).toBe('operational');
    expect(mockFetch).toHaveBeenCalledTimes(1);

    const callArgs = mockFetch.mock.calls[0];
    const headers = callArgs[1].headers as Headers;
    expect(headers.get('Authorization')).toBe('Bearer my-token');
  });

  it('throws ApiError with server message when HTTP response is not ok', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ error: 'workflow id wf_999 not found' })
    });
    vi.stubGlobal('fetch', mockFetch);

    await expect(client.getWorkflow('wf_999')).rejects.toThrow('workflow id wf_999 not found');
  });

  it('handles 204 No Content as undefined for void return types', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
      headers: new Headers(),
      json: async () => null
    });
    vi.stubGlobal('fetch', mockFetch);

    const res = await client.deleteWorkflow('wf_123');
    expect(res).toBeUndefined();
  });

  it('formats query parameters correctly for listExecutions', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => []
    });
    vi.stubGlobal('fetch', mockFetch);

    await client.listExecutions({
      workflow_id: 'wf_test',
      status: 'failed',
      limit: 25,
      offset: 10
    });

    const callUrl = mockFetch.mock.calls[0][0];
    expect(callUrl).toContain('workflow_id=wf_test');
    expect(callUrl).toContain('status=failed');
    expect(callUrl).toContain('limit=25');
    expect(callUrl).toContain('offset=10');
  });

  it('handles network failure and notifies status listeners', async () => {
    let onlineStatus: boolean | null = null;
    let statusError: string | undefined = undefined;

    client.onStatusChange((online, err) => {
      onlineStatus = online;
      statusError = err;
    });

    const mockFetch = vi.fn().mockRejectedValue(new Error('Failed to fetch'));
    vi.stubGlobal('fetch', mockFetch);

    await expect(client.getOverview()).rejects.toThrow('Failed to fetch');
    expect(onlineStatus).toBe(false);
    expect(statusError).toContain('Failed to fetch');
  });

  it('handles 500 error responses with plain text or json fallback', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      headers: new Headers({ 'content-type': 'text/plain' }),
      text: async () => 'Database connection timeout',
      json: async () => {
        throw new Error('not json');
      }
    });
    vi.stubGlobal('fetch', mockFetch);

    await expect(client.listWorkflows()).rejects.toThrow();
  });
});
