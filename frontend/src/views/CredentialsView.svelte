<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    openAddCredentialModal,
    showConfirm,
    notifySuccess,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { Credential } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let credentials = $state<Credential[]>([]);
  let loading = $state(true);

  async function loadCredentials() {
    loading = true;
    try {
      credentials = await api.listCredentials();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load credentials vault';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    loadCredentials();
  });

  onMount(() => {
    loadCredentials();
  });

  function confirmDelete(c: Credential) {
    showConfirm({
      title: 'Delete Encrypted Credential',
      message: `Are you sure you want to delete credential "${c.name}"? Automations referencing this token will fail.`,
      confirmLabel: 'Delete Credential',
      danger: true,
      onConfirm: async () => {
        try {
          await api.deleteCredential(c.id);
          notifySuccess(`Credential "${c.name}" deleted`);
          await loadCredentials();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : 'Failed to delete credential';
          notifyError(msg);
        }
      }
    });
  }
</script>

<div class="credentials-view">
  <Card
    title={`Credentials Vault (${credentials.length})`}
    subtitle="Tokens and API keys encrypted at rest with AES-256-GCM. Raw secrets are never exposed in logs or normal API payloads."
    padding="none"
  >
    {#snippet headerActions()}
      <Button variant="primary" size="sm" icon="plus" onclick={() => openAddCredentialModal()}>
        Add Credential
      </Button>
    {/snippet}

    {#if loading && credentials.length === 0}
      <div class="table-loading">Loading credentials vault...</div>
    {:else if credentials.length > 0}
      <div class="table-container">
        <table class="tech-table">
          <thead>
            <tr>
              <th>Credential Name</th>
              <th>Integration</th>
              <th>Key Identifier</th>
              <th>Masked Values</th>
              <th>Created</th>
              <th style="text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each credentials as c (c.id)}
              <tr>
                <td>
                  <div class="cred-name">{c.name}</div>
                  <div class="cred-id mono">ID: <code>{c.id.slice(0, 8)}...</code></div>
                </td>
                <td>
                  <Badge variant="neutral">{c.integration}</Badge>
                </td>
                <td class="mono">{c.key_id}</td>
                <td>
                  <div class="masked-box mono">
                    {JSON.stringify(c.masked_fields)}
                  </div>
                </td>
                <td class="text-muted">{new Date(c.created_at).toLocaleDateString()}</td>
                <td style="text-align: right;">
                  <Button variant="danger" size="sm" onclick={() => confirmDelete(c)}>
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
        title="Credentials Vault is Empty"
        description="Add encrypted tokens for Telegram, Discord, GitHub, or HTTP APIs. Encrypted tokens can be safely bound to workflow actions by ID."
        icon="credential"
      >
        {#snippet action()}
          <Button variant="primary" size="sm" onclick={() => openAddCredentialModal()}>
            Add Credential
          </Button>
        {/snippet}
      </EmptyState>
    {/if}
  </Card>
</div>

<style>
  .credentials-view {
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

  .cred-name {
    font-weight: 600;
    color: var(--text-primary);
    font-size: 13px;
  }

  .cred-id {
    font-size: 10.5px;
    color: var(--text-faint);
    margin-top: 3px;
  }

  .masked-box {
    display: inline-block;
    padding: 2px 6px;
    background: var(--bg-deep);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-xs);
    font-size: 11px;
    color: var(--text-muted);
  }

  .text-muted {
    color: var(--text-muted);
  }
</style>
