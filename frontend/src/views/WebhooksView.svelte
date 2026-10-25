<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    openCreateWebhookModal,
    showConfirm,
    notifySuccess,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { Webhook } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let webhooks = $state<Webhook[]>([]);
  let loading = $state(true);

  async function loadWebhooks() {
    loading = true;
    try {
      webhooks = await api.listWebhooks();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load webhooks';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    loadWebhooks();
  });

  onMount(() => {
    loadWebhooks();
  });

  function getWebhookUrl(id: string): string {
    const base = typeof window !== 'undefined' ? window.location.origin : '';
    return `${base}/api/v1/webhooks/${id}/ingest`;
  }

  async function copyUrl(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      notifySuccess('Webhook URL copied to clipboard');
    } catch {
      notifyError('Failed to copy to clipboard');
    }
  }

  function confirmDelete(wh: Webhook) {
    showConfirm({
      title: 'Delete Inbound Webhook Endpoint',
      message: `Permanently delete endpoint "${wh.name}"? External services calling this webhook URL will receive 404 Not Found.`,
      confirmLabel: 'Delete Endpoint',
      danger: true,
      onConfirm: async () => {
        try {
          await api.deleteWebhook(wh.id);
          notifySuccess(`Webhook "${wh.name}" deleted`);
          await loadWebhooks();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : 'Failed to delete webhook';
          notifyError(msg);
        }
      }
    });
  }
</script>

<div class="webhooks-view">
  <Card
    title={`Inbound Webhook Endpoints (${webhooks.length})`}
    subtitle="HTTP POST endpoints receiving payloads from Stripe, GitHub, or external services and converting them into normalized events."
    padding="none"
  >
    {#snippet headerActions()}
      <Button variant="primary" size="sm" icon="plus" onclick={() => openCreateWebhookModal()}>
        Create Webhook
      </Button>
    {/snippet}

    {#if loading && webhooks.length === 0}
      <div class="table-loading">Loading inbound endpoints...</div>
    {:else if webhooks.length > 0}
      <div class="table-container">
        <table class="tech-table">
          <thead>
            <tr>
              <th>Endpoint Name</th>
              <th>Ingestion URL (HTTP POST)</th>
              <th>Event Type Emitted</th>
              <th>Status</th>
              <th style="text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each webhooks as wh (wh.id)}
              <tr>
                <td>
                  <div class="wh-name">{wh.name}</div>
                  <div class="wh-id mono">ID: <code>{wh.id}</code></div>
                </td>
                <td>
                  <div class="url-row">
                    <code class="url-code">POST /api/v1/webhooks/{wh.id}/ingest</code>
                    <Button
                      variant="ghost"
                      size="sm"
                      icon="copy"
                      onclick={() => copyUrl(getWebhookUrl(wh.id))}
                    >
                      Copy
                    </Button>
                  </div>
                </td>
                <td>
                  <Badge variant="neutral">{wh.event_type}</Badge>
                </td>
                <td>
                  <Badge variant={wh.enabled ? 'success' : 'warning'}>
                    {wh.enabled ? 'ACTIVE' : 'DISABLED'}
                  </Badge>
                </td>
                <td style="text-align: right;">
                  <Button variant="danger" size="sm" onclick={() => confirmDelete(wh)}>
                    Delete
                  </Button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <EmptyState
        title="No inbound webhooks configured"
        description="Create a dedicated webhook endpoint to ingest real-time HTTP events from Stripe, Shopify, GitHub, or custom services."
        icon="webhook"
      >
        {#snippet action()}
          <Button variant="primary" size="sm" onclick={() => openCreateWebhookModal()}>
            Create Inbound Webhook
          </Button>
        {/snippet}
      </EmptyState>
    {/if}
  </Card>
</div>

<style>
  .webhooks-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .table-loading {
    padding: 30px;
    text-align: center;
    color: var(--text-muted);
    font-size: 12px;
  }

  .table-container {
    overflow-x: auto;
  }

  .tech-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 12px;
  }

  .tech-table th {
    text-align: left;
    padding: 10px 14px;
    font-size: 10.5px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    background: var(--bg-surface-elevated);
    border-bottom: 1px solid var(--border-default);
  }

  .tech-table td {
    padding: 12px 14px;
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-secondary);
  }

  .tech-table tr:hover td {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
  }

  .wh-name {
    font-weight: 600;
    color: var(--text-primary);
    font-size: 13px;
  }

  .wh-id {
    font-size: 10.5px;
    color: var(--text-faint);
    margin-top: 3px;
  }

  .url-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .url-code {
    font-size: 11px;
    color: var(--accent-emerald);
  }
</style>
