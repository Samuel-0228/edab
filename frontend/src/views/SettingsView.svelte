<script lang="ts">
  import {
    apiConfig,
    currentIdentity,
    fetchCurrentIdentity,
    canManageMembers,
    triggerRefresh,
    notifySuccess,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { Membership, APIToken, AuditLog, Role } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Input from '../components/common/Input.svelte';
  import Badge from '../components/common/Badge.svelte';
  import { onMount } from 'svelte';

  let activeTab = $state<'connectivity' | 'access' | 'audit' | 'security'>('connectivity');

  // Connectivity
  let token = $state($apiConfig.token);
  let baseUrl = $state($apiConfig.baseUrl);
  let testingConnection = $state(false);
  let testResult = $state<{ success: boolean; message: string } | null>(null);

  // Access & RBAC
  let members = $state<Membership[]>([]);
  let tokens = $state<APIToken[]>([]);
  let loadingAccess = $state(false);

  // Member invite form
  let newUserId = $state('');
  let newMemberRole = $state<Role>('editor');
  let invitingMember = $state(false);

  // Token create form
  let newTokenName = $state('');
  let newTokenScopes = $state('workflows:read,workflows:write,executions:read');
  let creatingToken = $state(false);
  let newlyCreatedToken = $state<string | null>(null);

  // Audit Logs
  let auditLogs = $state<AuditLog[]>([]);
  let loadingAudit = $state(false);

  onMount(() => {
    loadAccessData();
  });

  async function loadAccessData() {
    loadingAccess = true;
    try {
      await fetchCurrentIdentity();
      const [mList, tList] = await Promise.all([
        api.listMembers().catch(() => []),
        api.listAPITokens().catch(() => [])
      ]);
      members = mList;
      tokens = tList;
    } catch (err: unknown) {
      console.warn('Could not load access details:', err);
    } finally {
      loadingAccess = false;
    }
  }

  async function loadAuditLogs() {
    loadingAudit = true;
    try {
      const logs = await api.listAuditLogs(50, 0);
      auditLogs = logs;
    } catch (err: unknown) {
      notifyError(
        'Failed to load audit logs: ' + (err instanceof Error ? err.message : String(err))
      );
    } finally {
      loadingAudit = false;
    }
  }

  function handleSave() {
    api.setToken(token);
    api.setBaseUrl(baseUrl);
    apiConfig.set({
      token: api.getToken(),
      baseUrl: api.getBaseUrl()
    });
    notifySuccess('API connection settings updated');
    triggerRefresh();
    loadAccessData();
  }

  async function testConnection() {
    testingConnection = true;
    testResult = null;
    try {
      api.setToken(token);
      api.setBaseUrl(baseUrl);
      const overview = await api.getOverview();
      testResult = {
        success: true,
        message: `Connected successfully! Engine is ${overview.status} with ${overview.active_workflows} active automations.`
      };
      notifySuccess('Connection verified');
      loadAccessData();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Connection test failed';
      testResult = {
        success: false,
        message: `Connection failed: ${msg}`
      };
      notifyError(msg);
    } finally {
      testingConnection = false;
    }
  }

  function handleClear() {
    token = '';
    baseUrl = '';
    handleSave();
    testResult = null;
  }

  async function handleAddMember() {
    if (!newUserId.trim()) {
      notifyError('User ID is required');
      return;
    }
    invitingMember = true;
    try {
      await api.createMember(newUserId.trim(), newMemberRole);
      notifySuccess(`Member added with role ${newMemberRole}`);
      newUserId = '';
      loadAccessData();
    } catch (err: unknown) {
      notifyError(err instanceof Error ? err.message : 'Failed to add member');
    } finally {
      invitingMember = false;
    }
  }

  async function handleUpdateRole(userId: string, role: Role) {
    try {
      await api.updateMemberRole(userId, role);
      notifySuccess('Member role updated');
      loadAccessData();
    } catch (err: unknown) {
      notifyError(err instanceof Error ? err.message : 'Failed to update role');
    }
  }

  async function handleDeleteMember(userId: string) {
    if (!confirm(`Are you sure you want to remove user ${userId} from this tenant?`)) return;
    try {
      await api.deleteMember(userId);
      notifySuccess('Member removed');
      loadAccessData();
    } catch (err: unknown) {
      notifyError(err instanceof Error ? err.message : 'Failed to remove member');
    }
  }

  async function handleCreateToken() {
    if (!newTokenName.trim()) {
      notifyError('Token name is required');
      return;
    }
    creatingToken = true;
    newlyCreatedToken = null;
    try {
      const scopes = newTokenScopes
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);
      const res = await api.createAPIToken(newTokenName.trim(), scopes);
      newlyCreatedToken = res.token || null;
      notifySuccess('API Token created');
      newTokenName = '';
      loadAccessData();
    } catch (err: unknown) {
      notifyError(err instanceof Error ? err.message : 'Failed to create token');
    } finally {
      creatingToken = false;
    }
  }

  async function handleRevokeToken(id: string) {
    if (
      !confirm(
        'Are you sure you want to revoke this API token? Any client using it will lose access.'
      )
    )
      return;
    try {
      await api.revokeAPIToken(id);
      notifySuccess('API Token revoked');
      loadAccessData();
    } catch (err: unknown) {
      notifyError(err instanceof Error ? err.message : 'Failed to revoke token');
    }
  }
</script>

<div class="settings-view">
  <!-- Tabs Navigation -->
  <div class="tabs-nav">
    <button
      class="tab-btn {activeTab === 'connectivity' ? 'active' : ''}"
      onclick={() => (activeTab = 'connectivity')}
    >
      Connectivity & API
    </button>
    <button
      class="tab-btn {activeTab === 'access' ? 'active' : ''}"
      onclick={() => {
        activeTab = 'access';
        loadAccessData();
      }}
    >
      Team & Access (RBAC)
    </button>
    <button
      class="tab-btn {activeTab === 'audit' ? 'active' : ''}"
      onclick={() => {
        activeTab = 'audit';
        loadAuditLogs();
      }}
    >
      Audit Logs
    </button>
    <button
      class="tab-btn {activeTab === 'security' ? 'active' : ''}"
      onclick={() => (activeTab = 'security')}
    >
      Security & Worker
    </button>
  </div>

  {#if activeTab === 'connectivity'}
    <!-- Authentication & Base URL Settings -->
    <Card
      title="API Authentication & Connectivity"
      subtitle="Configure the Bearer API Token and target host for all HTTP requests issued by this control dashboard."
    >
      <div class="settings-grid">
        <Input
          type="password"
          label="Bearer API Token (API_TOKEN or krb_...)"
          placeholder="Enter API_TOKEN or scoped krb_ token"
          bind:value={token}
          mono
          hint="When Kerob is deployed in production mode, all REST endpoints require this token."
        />

        <Input
          label="Engine Base URL (Optional)"
          placeholder="e.g. http://localhost:8080 or https://kerob.internal"
          bind:value={baseUrl}
          mono
          hint="Leave empty to use same-origin relative URLs (/api/v1/...)."
        />

        <div class="actions-row">
          <Button variant="primary" size="sm" onclick={handleSave}>Save Settings</Button>
          <Button
            variant="secondary"
            size="sm"
            loading={testingConnection}
            onclick={testConnection}
          >
            Test Connection
          </Button>
          <Button variant="ghost" size="sm" onclick={handleClear}>Reset Defaults</Button>
        </div>

        {#if testResult}
          <div class="test-result-box {testResult.success ? 'success' : 'error'}">
            <Badge variant={testResult.success ? 'success' : 'failed'}>
              {testResult.success ? 'CONNECTION VERIFIED' : 'AUTH FAILED'}
            </Badge>
            <span class="test-result-msg mono">{testResult.message}</span>
          </div>
        {/if}
      </div>
    </Card>
  {/if}

  {#if activeTab === 'access'}
    <!-- Multi-tenancy & Identity Info -->
    <Card
      title="Active Tenant & User Identity"
      subtitle="Tenant context and role permissions controlling resource authorization."
    >
      {#if $currentIdentity}
        <div class="identity-banner mono">
          <div class="banner-item">
            <span class="banner-label">Organization:</span>
            <span class="banner-val"
              >{$currentIdentity.tenant_name} ({$currentIdentity.tenant_id})</span
            >
          </div>
          <div class="banner-item">
            <span class="banner-label">User:</span>
            <span class="banner-val">{$currentIdentity.name} &lt;{$currentIdentity.email}&gt;</span>
          </div>
          <div class="banner-item">
            <span class="banner-label">Role:</span>
            <span class="role-tag {$currentIdentity.role}"
              >{$currentIdentity.role.toUpperCase()}</span
            >
          </div>
        </div>
      {:else}
        <p class="text-muted text-sm">Identity not loaded. Ensure engine is reachable.</p>
      {/if}
    </Card>

    <!-- Team Members Table -->
    <Card
      title="Tenant Members & Roles"
      subtitle="Role-Based Access Control governs permissions across automations, integrations, and audit records."
    >
      {#if canManageMembers($currentIdentity?.role)}
        <div class="invite-form">
          <Input placeholder="User ID (e.g. usr_123 or user@domain)" bind:value={newUserId} mono />
          <select class="role-select" bind:value={newMemberRole}>
            <option value="viewer">Viewer</option>
            <option value="editor">Editor</option>
            <option value="operator">Operator</option>
            <option value="admin">Admin</option>
            <option value="owner">Owner</option>
          </select>
          <Button variant="primary" size="sm" loading={invitingMember} onclick={handleAddMember}>
            Add Member
          </Button>
        </div>
      {/if}

      <div class="table-container">
        <table class="access-table">
          <thead>
            <tr>
              <th>User ID</th>
              <th>Role</th>
              <th>Joined At</th>
              {#if canManageMembers($currentIdentity?.role)}
                <th>Actions</th>
              {/if}
            </tr>
          </thead>
          <tbody>
            {#if loadingAccess}
              <tr>
                <td
                  colspan={canManageMembers($currentIdentity?.role) ? 4 : 3}
                  class="text-center text-muted"
                >
                  Loading team access...
                </td>
              </tr>
            {:else}
              {#each members as m (m.id || m.user_id)}
                <tr>
                  <td class="mono font-semibold">{m.user_id}</td>
                  <td>
                    {#if canManageMembers($currentIdentity?.role) && m.user_id !== $currentIdentity?.user_id}
                      <select
                        class="role-select-inline"
                        value={m.role}
                        onchange={(e) =>
                          handleUpdateRole(
                            m.user_id,
                            (e.currentTarget as HTMLSelectElement).value as Role
                          )}
                      >
                        <option value="viewer">Viewer</option>
                        <option value="editor">Editor</option>
                        <option value="operator">Operator</option>
                        <option value="admin">Admin</option>
                        <option value="owner">Owner</option>
                      </select>
                    {:else}
                      <span class="role-tag {m.role}">{m.role.toUpperCase()}</span>
                    {/if}
                  </td>
                  <td class="mono text-muted">{new Date(m.created_at).toLocaleDateString()}</td>
                  {#if canManageMembers($currentIdentity?.role)}
                    <td>
                      {#if m.user_id !== $currentIdentity?.user_id}
                        <Button
                          variant="ghost"
                          size="sm"
                          onclick={() => handleDeleteMember(m.user_id)}
                        >
                          Remove
                        </Button>
                      {:else}
                        <span class="text-muted text-xs mono">(You)</span>
                      {/if}
                    </td>
                  {/if}
                </tr>
              {:else}
                <tr>
                  <td
                    colspan={canManageMembers($currentIdentity?.role) ? 4 : 3}
                    class="text-center text-muted"
                  >
                    No members loaded.
                  </td>
                </tr>
              {/each}
            {/if}
          </tbody>
        </table>
      </div>
    </Card>

    <!-- Scoped API Tokens -->
    <Card
      title="Service Accounts & Scoped API Tokens"
      subtitle="Tokens generated with prefix krb_ and SHA-256 hashed at rest. Scopes restrict automation permissions."
    >
      {#if newlyCreatedToken}
        <div class="secret-box">
          <Badge variant="warning">NEW SECRET GENERATED</Badge>
          <p class="secret-instruction text-sm">
            Make sure to copy your API Token now. You will not be able to see it again!
          </p>
          <pre class="secret-val mono">{newlyCreatedToken}</pre>
        </div>
      {/if}

      {#if canManageMembers($currentIdentity?.role)}
        <div class="token-form">
          <Input placeholder="Token Name (e.g. CI/CD Deployer)" bind:value={newTokenName} />
          <Input
            placeholder="Scopes (comma-separated, e.g. workflows:read,events:replay)"
            bind:value={newTokenScopes}
            mono
          />
          <Button variant="secondary" size="sm" loading={creatingToken} onclick={handleCreateToken}>
            Create API Token
          </Button>
        </div>
      {/if}

      <div class="table-container">
        <table class="access-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Prefix</th>
              <th>Scopes</th>
              <th>Created</th>
              <th>Last Used</th>
              {#if canManageMembers($currentIdentity?.role)}
                <th>Revoke</th>
              {/if}
            </tr>
          </thead>
          <tbody>
            {#each tokens as t (t.id)}
              <tr>
                <td class="font-semibold">{t.name}</td>
                <td class="mono">{t.token_prefix}...</td>
                <td>
                  <div class="scopes-wrap">
                    {#each t.scopes as sc (sc)}
                      <span class="scope-pill mono">{sc}</span>
                    {/each}
                  </div>
                </td>
                <td class="mono text-muted">{new Date(t.created_at).toLocaleDateString()}</td>
                <td class="mono text-muted">
                  {t.last_used_at ? new Date(t.last_used_at).toLocaleDateString() : 'Never'}
                </td>
                {#if canManageMembers($currentIdentity?.role)}
                  <td>
                    <Button variant="ghost" size="sm" onclick={() => handleRevokeToken(t.id)}>
                      Revoke
                    </Button>
                  </td>
                {/if}
              </tr>
            {:else}
              <tr>
                <td
                  colspan={canManageMembers($currentIdentity?.role) ? 6 : 5}
                  class="text-center text-muted"
                >
                  No API tokens configured.
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </Card>
  {/if}

  {#if activeTab === 'audit'}
    <!-- Audit Trail -->
    <Card
      title="Security Audit Log"
      subtitle="Immutable trail of mutations, credential access, auth attempts, and retry/replay operations with secrets scrubbed."
    >
      <div class="table-container">
        <table class="access-table">
          <thead>
            <tr>
              <th>Timestamp</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Resource</th>
              <th>IP Address</th>
            </tr>
          </thead>
          <tbody>
            {#each auditLogs as log (log.id)}
              <tr>
                <td class="mono text-muted text-xs">{new Date(log.created_at).toLocaleString()}</td>
                <td class="mono text-sm">{log.actor_email || log.user_id || 'system'}</td>
                <td>
                  <span class="action-tag mono">{log.action}</span>
                </td>
                <td class="mono text-xs">{log.resource_type}:{log.resource_id || '-'}</td>
                <td class="mono text-muted text-xs">{log.ip_address || '-'}</td>
              </tr>
            {:else}
              <tr>
                <td colspan="5" class="text-center text-muted">
                  {#if loadingAudit}
                    Loading audit trail...
                  {:else}
                    No audit records found.
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </Card>
  {/if}

  {#if activeTab === 'security'}
    <!-- Security & Architecture Information -->
    <div class="grid-2">
      <Card title="Security & Hardening Posture" subtitle="Active platform protections">
        <div class="info-list">
          <div class="info-item">
            <span class="info-label">Encryption Algorithm</span>
            <Badge variant="neutral">AES-256-GCM (256-bit Key)</Badge>
          </div>
          <div class="info-item">
            <span class="info-label">SSRF Defenses</span>
            <Badge variant="success">RFC 1918 & Cloud Metadata Blocked</Badge>
          </div>
          <div class="info-item">
            <span class="info-label">Multi-Tenancy Isolation</span>
            <Badge variant="success">Database Row & Tenant ID Layer</Badge>
          </div>
          <div class="info-item">
            <span class="info-label">Secret Scrubbing</span>
            <Badge variant="neutral">Automated Redaction on Audit Trail</Badge>
          </div>
          <div class="info-item">
            <span class="info-label">Inbound Request Cap</span>
            <span class="info-val mono">1 MB Max Payload</span>
          </div>
          <div class="info-item">
            <span class="info-label">Idempotency Protection</span>
            <span class="info-val mono">External Event ID Deduplication</span>
          </div>
        </div>
      </Card>

      <Card title="Worker & Concurrency Parameters" subtitle="Background job worker config">
        <div class="info-list">
          <div class="info-item">
            <span class="info-label">Worker Poll Interval</span>
            <span class="info-val mono">1s Polling Loop</span>
          </div>
          <div class="info-item">
            <span class="info-label">Worker Concurrency</span>
            <span class="info-val mono">4 Concurrent Goroutines</span>
          </div>
          <div class="info-item">
            <span class="info-label">Durable Job Leases</span>
            <span class="info-val mono">Heartbeat Renewals & Safe Takeover</span>
          </div>
          <div class="info-item">
            <span class="info-label">Retry Strategy</span>
            <span class="info-val mono">Exponential Backoff (Initial: 2s, Max: 1m)</span>
          </div>
          <div class="info-item">
            <span class="info-label">Scheduler Interval</span>
            <span class="info-val mono">10s Background Tick</span>
          </div>
        </div>
      </Card>
    </div>
  {/if}
</div>

<style>
  .settings-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .tabs-nav {
    display: flex;
    gap: 8px;
    border-bottom: 1px solid var(--border-default, #272a30);
    padding-bottom: 8px;
  }

  .tab-btn {
    background: none;
    border: none;
    color: var(--text-muted, #71767b);
    font-size: 13px;
    font-weight: 500;
    padding: 6px 14px;
    cursor: pointer;
    border-radius: var(--radius-sm, 4px);
    transition: all 0.15s ease;
  }
  .tab-btn:hover {
    color: var(--text-primary, #e6edf3);
    background: var(--bg-surface-elevated, #1c2128);
  }
  .tab-btn.active {
    color: var(--text-primary, #ffffff);
    background: var(--bg-surface-elevated, #21262d);
    font-weight: 600;
  }

  .settings-grid {
    display: flex;
    flex-direction: column;
    gap: 16px;
    max-width: 600px;
  }

  .actions-row {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 4px;
  }

  .test-result-box {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    border-radius: var(--radius-sm);
    border: 1px solid transparent;
  }
  .test-result-box.success {
    background: var(--accent-emerald-dim);
    border-color: var(--accent-emerald-border);
  }
  .test-result-box.error {
    background: var(--accent-crimson-dim);
    border-color: var(--accent-crimson-border);
  }

  .test-result-msg {
    font-size: 11.5px;
    color: var(--text-primary);
  }

  .identity-banner {
    display: flex;
    flex-wrap: wrap;
    gap: 24px;
    padding: 12px 16px;
    background: var(--bg-surface-elevated, #161b22);
    border: 1px solid var(--border-default, #30363d);
    border-radius: var(--radius-sm, 4px);
  }

  .banner-item {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
  }

  .banner-label {
    color: var(--text-secondary, #8b949e);
  }

  .banner-val {
    color: var(--text-primary, #f0f6fc);
  }

  .invite-form,
  .token-form {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-bottom: 16px;
  }

  .role-select {
    height: 32px;
    padding: 0 10px;
    background: var(--bg-surface-elevated, #21262d);
    border: 1px solid var(--border-default, #30363d);
    color: var(--text-primary, #f0f6fc);
    border-radius: var(--radius-sm, 4px);
    font-size: 12px;
  }

  .role-select-inline {
    height: 24px;
    padding: 0 6px;
    background: var(--bg-surface-elevated, #21262d);
    border: 1px solid var(--border-default, #30363d);
    color: var(--text-primary, #f0f6fc);
    border-radius: var(--radius-sm, 4px);
    font-size: 11px;
  }

  .table-container {
    overflow-x: auto;
  }

  .access-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 12px;
  }

  .access-table th {
    text-align: left;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border-default, #30363d);
    color: var(--text-muted, #8b949e);
    font-weight: 500;
  }

  .access-table td {
    padding: 10px 12px;
    border-bottom: 1px solid var(--border-subtle, #21262d);
    color: var(--text-primary, #c9d1d9);
  }

  .role-tag {
    font-size: 10px;
    font-weight: 700;
    padding: 2px 6px;
    border-radius: 3px;
    letter-spacing: 0.05em;
  }
  .role-tag.owner {
    background: rgba(239, 68, 68, 0.2);
    color: #f87171;
  }
  .role-tag.admin {
    background: rgba(245, 158, 11, 0.2);
    color: #fbbf24;
  }
  .role-tag.operator {
    background: rgba(59, 130, 246, 0.2);
    color: #60a5fa;
  }
  .role-tag.editor {
    background: rgba(16, 185, 129, 0.2);
    color: #34d399;
  }
  .role-tag.viewer {
    background: rgba(156, 163, 175, 0.2);
    color: #9ca3af;
  }

  .scopes-wrap {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .scope-pill {
    font-size: 10px;
    padding: 1px 5px;
    background: var(--bg-surface-elevated, #21262d);
    border: 1px solid var(--border-default, #30363d);
    border-radius: 3px;
    color: var(--text-secondary, #8b949e);
  }

  .action-tag {
    font-size: 11px;
    color: #58a6ff;
  }

  .secret-box {
    padding: 12px 16px;
    background: rgba(245, 158, 11, 0.08);
    border: 1px solid rgba(245, 158, 11, 0.3);
    border-radius: var(--radius-sm, 4px);
    margin-bottom: 16px;
  }

  .secret-instruction {
    margin: 8px 0;
    color: #fbbf24;
  }

  .secret-val {
    padding: 8px 12px;
    background: #0d1117;
    border: 1px solid #30363d;
    border-radius: 4px;
    color: #58a6ff;
    font-size: 13px;
    user-select: all;
  }

  .grid-2 {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: 16px;
  }

  .info-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .info-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 8px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .info-item:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }

  .info-label {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .info-val {
    font-size: 11.5px;
    color: var(--text-primary);
  }
</style>
