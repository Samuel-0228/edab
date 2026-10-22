<script lang="ts">
  import { apiConfig, triggerRefresh, notifySuccess, notifyError } from '../api/stores';
  import { api } from '../api/client';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Input from '../components/common/Input.svelte';
  import Badge from '../components/common/Badge.svelte';

  let token = $state($apiConfig.token);
  let baseUrl = $state($apiConfig.baseUrl);
  let testingConnection = $state(false);
  let testResult = $state<{ success: boolean; message: string } | null>(null);

  function handleSave() {
    api.setToken(token);
    api.setBaseUrl(baseUrl);
    apiConfig.set({
      token: api.getToken(),
      baseUrl: api.getBaseUrl()
    });
    notifySuccess('API connection settings updated');
    triggerRefresh();
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
</script>

<div class="settings-view">
  <!-- Authentication & Base URL Settings -->
  <Card
    title="API Authentication & Connectivity"
    subtitle="Configure the Bearer API Token and target host for all HTTP requests issued by this control dashboard."
  >
    <div class="settings-grid">
      <Input
        type="password"
        label="Bearer API Token (API_TOKEN)"
        placeholder="Enter API_TOKEN configured on Kerob engine"
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
        <Button variant="secondary" size="sm" loading={testingConnection} onclick={testConnection}>
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
</div>

<style>
  .settings-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
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
