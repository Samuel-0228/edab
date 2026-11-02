<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    openCreateWorkflow,
    openAddCredentialModal,
    navigate,
    notifySuccess,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { IntegrationMetadata, ConnectorManifest, ConnectorHealthStatus } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';

  let catalog = $state<IntegrationMetadata[]>([]);
  let connectors = $state<ConnectorManifest[]>([]);
  let healthMap = $state<Record<string, { status: string; latency?: number; message?: string; checking?: boolean }>>({});

  async function loadData() {
    try {
      const [catList, connList] = await Promise.all([
        api.listIntegrations(),
        api.listConnectors().catch(() => [] as ConnectorManifest[])
      ]);
      catalog = catList;
      connectors = connList;
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load integrations catalog';
      notifyError(msg);
    }
  }

  $effect(() => {
    void $refreshTick;
    loadData();
  });

  onMount(() => {
    loadData();
  });

  function getConnector(id: string): ConnectorManifest | undefined {
    return connectors.find((c) => c.id === id);
  }

  async function handleOAuthConnect(connectorId: string) {
    try {
      const redirectUri = window.location.origin + '/ui/credentials';
      const resp = await api.authorizeOAuth(connectorId, redirectUri);
      if (resp.authorize_url) {
        window.location.href = resp.authorize_url;
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to start OAuth authorization';
      notifyError(msg);
    }
  }

  async function handleHealthCheck(connectorId: string) {
    healthMap[connectorId] = { status: 'checking', checking: true };
    try {
      const res: ConnectorHealthStatus = await api.checkConnectorHealth(connectorId);
      healthMap[connectorId] = {
        status: res.status,
        latency: res.latency_ms,
        message: res.message,
        checking: false
      };
      if (res.status === 'healthy') {
        notifySuccess(`${connectorId} is healthy (${res.latency_ms}ms)`);
      } else {
        notifyError(`${connectorId} health check: ${res.status} (${res.message || 'unknown error'})`);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Health check failed';
      healthMap[connectorId] = {
        status: 'unhealthy',
        message: msg,
        checking: false
      };
      notifyError(msg);
    }
  }
</script>

<div class="integrations-view">
  <!-- Top Intro Card -->
  <Card
    title="Supported Integration Adapters & Connectors"
    subtitle="Uniform SDK adapter contracts isolating external protocols from the Kerob core routing engine with OAuth2 and health observability."
  >
    {#snippet headerActions()}
      <Button variant="secondary" size="sm" onclick={() => navigate('credentials')}>
        Credentials Vault
      </Button>
    {/snippet}

    <div class="catalog-grid">
      {#each catalog as item (item.id)}
        {@const conn = getConnector(item.id)}
        {@const isOAuth = conn?.auth_type === 'oauth2' || item.capabilities?.includes('oauth2')}
        {@const supportsHealth = conn?.capabilities?.includes('health_check')}
        {@const health = healthMap[item.id]}

        <div class="catalog-card">
          <div class="card-top">
            <div class="card-title-group">
              <span class="adapter-title">{item.name}</span>
              <div class="badges-row">
                {#if isOAuth}
                  <Badge variant="stage-transform">OAuth 2.0</Badge>
                {/if}
                <Badge variant="success">PRODUCTION READY</Badge>
              </div>
            </div>
            <p class="adapter-desc">{item.description}</p>
          </div>

          <!-- Dynamic Health Indicator -->
          {#if health}
            <div class="health-status-box">
              <span class="health-label">LIVE STATUS:</span>
              {#if health.checking}
                <span class="health-checking">Probing endpoint...</span>
              {:else if health.status === 'healthy'}
                <Badge variant="success">Healthy ({health.latency}ms)</Badge>
              {:else if health.status === 'degraded'}
                <Badge variant="warning">Degraded ({health.latency}ms)</Badge>
              {:else}
                <Badge variant="danger">Unhealthy</Badge>
              {/if}
              {#if health.message}
                <span class="health-msg text-muted mono">{health.message}</span>
              {/if}
            </div>
          {/if}

          <div class="capabilities-box">
            <!-- Triggers -->
            <div class="capability-group">
              <span class="capability-label">SUPPORTED TRIGGERS</span>
              {#if item.triggers && item.triggers.length > 0}
                <div class="caps-list">
                  {#each item.triggers as t (t.type)}
                    <div class="cap-entry">
                      <code class="cap-type">{t.type}</code>
                      <span class="cap-desc">{t.name}</span>
                    </div>
                  {/each}
                </div>
              {:else}
                <span class="cap-none">None (Action destination only)</span>
              {/if}
            </div>

            <!-- Actions -->
            <div class="capability-group">
              <span class="capability-label">SUPPORTED ACTIONS</span>
              {#if item.actions && item.actions.length > 0}
                <div class="caps-list">
                  {#each item.actions as a (a.type)}
                    <div class="cap-entry">
                      <code class="cap-type">{a.type}</code>
                      <span class="cap-desc">{a.name}</span>
                    </div>
                  {/each}
                </div>
              {:else}
                <span class="cap-none">None (Source trigger only)</span>
              {/if}
            </div>
          </div>

          <div class="card-footer-actions">
            <div class="left-actions">
              <Button variant="secondary" size="sm" onclick={() => openCreateWorkflow(item.id)}>
                + Use in Automation
              </Button>
              {#if supportsHealth || conn}
                <Button variant="ghost" size="sm" onclick={() => handleHealthCheck(item.id)}>
                  Check Health
                </Button>
              {/if}
            </div>

            <div class="right-actions">
              {#if isOAuth}
                <Button variant="primary" size="sm" onclick={() => handleOAuthConnect(item.id)}>
                  Connect via OAuth
                </Button>
              {:else if item.credentials && item.credentials.length > 0}
                <Button variant="ghost" size="sm" onclick={() => openAddCredentialModal(item.id)}>
                  Configure Token
                </Button>
              {/if}
            </div>
          </div>
        </div>
      {/each}
    </div>
  </Card>
</div>

<style>
  .integrations-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .catalog-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
  }

  @media (max-width: 900px) {
    .catalog-grid {
      grid-template-columns: 1fr;
    }
  }

  .catalog-card {
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 16px;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    gap: 16px;
  }

  .card-top {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .card-title-group {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .badges-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .adapter-title {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .adapter-desc {
    font-size: 0.85rem;
    color: var(--text-muted);
    line-height: 1.4;
    margin: 0;
  }

  .health-status-box {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    font-size: 0.8rem;
  }

  .health-label {
    font-size: 0.7rem;
    font-weight: 700;
    color: var(--text-muted);
    letter-spacing: 0.05em;
  }

  .health-checking {
    font-size: 0.8rem;
    color: var(--text-secondary);
  }

  .health-msg {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .capabilities-box {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
  }

  .capability-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .capability-label {
    font-size: 0.7rem;
    font-weight: 700;
    color: var(--text-muted);
    letter-spacing: 0.05em;
  }

  .caps-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .cap-entry {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.8rem;
  }

  .cap-type {
    background: var(--bg-code);
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 0.75rem;
    color: var(--accent-primary);
  }

  .cap-desc {
    color: var(--text-secondary);
  }

  .cap-none {
    font-size: 0.8rem;
    color: var(--text-muted);
    font-style: italic;
  }

  .card-footer-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-top: 12px;
    border-top: 1px solid var(--border-subtle);
    gap: 8px;
  }

  .left-actions,
  .right-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
</style>
