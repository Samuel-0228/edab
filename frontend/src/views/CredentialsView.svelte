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

  function isOAuthCred(c: Credential): boolean {
    if (!c.masked_fields) return false;
    return 'access_token' in c.masked_fields || 'refresh_token' in c.masked_fields;
  }

  async function handleRefreshOAuth(c: Credential) {
    try {
      await api.refreshOAuthToken(c.integration, c.id);
      notifySuccess(`OAuth token successfully rotated for "${c.name}"`);
      await loadCredentials();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to refresh OAuth token';
      notifyError(msg);
    }
  }

  function confirmRevokeOAuth(c: Credential) {
    showConfirm({
      title: 'Revoke Upstream OAuth Authorization',
      message: `Are you sure you want to revoke authorization for "${c.name}"? The token will be invalidated at ${c.integration} and purged from Kerob.`,
      confirmLabel: 'Revoke & Purge',
      danger: true,
      onConfirm: async () => {
        try {
          await api.revokeOAuthToken(c.integration, c.id);
          notifySuccess(`OAuth token for "${c.name}" revoked`);
          await loadCredentials();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : 'Failed to revoke token';
          notifyError(msg);
        }
      }
    });
  }

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
              {@const isOAuth = isOAuthCred(c)}
              <tr>
                <td>
                  <div class="cred-name">{c.name}</div>
                  <div class="cred-id mono">ID: <code>{c.id.slice(0, 8)}...</code></div>
                </td>
                <td>
                  <div class="integration-badge-group">
                    <Badge variant="neutral">{c.integration}</Badge>
                    {#if isOAuth}
                      <Badge variant="stage-transform">OAuth 2.0</Badge>
                    {/if}
                  </div>
                </td>
                <td class="mono">{c.key_id}</td>
                <td>
                  <div class="masked-box mono">
                    {JSON.stringify(c.masked_fields)}
                  </div>
                </td>
                <td class="text-muted">{new Date(c.created_at).toLocaleDateString()}</td>
                <td style="text-align: right;">
                  <div class="actions-row">
                    {#if isOAuth}
                      {#if c.masked_fields?.refresh_token}
                        <Button variant="secondary" size="sm" onclick={() => handleRefreshOAuth(c)}>
                          Rotate
                        </Button>
                      {/if}
                      <Button variant="danger" size="sm" onclick={() => confirmRevokeOAuth(c)}>
                        Revoke
                      </Button>
                    {:else}
                      <Button variant="danger" size="sm" onclick={() => confirmDelete(c)}>
                        Delete
                      </Button>
                    {/if}
                  </div>
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
  }

  .table-container {
    overflow-x: auto;
  }

  .tech-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }

  .tech-table th {
    text-align: left;
    padding: 10px 16px;
    background: var(--bg-surface-elevated);
    border-bottom: 1px solid var(--border-default);
    color: var(--text-muted);
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .tech-table td {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border-subtle);
    vertical-align: middle;
  }

  .cred-name {
    font-weight: 500;
    color: var(--text-primary);
  }

  .cred-id {
    font-size: 0.75rem;
    color: var(--text-muted);
    margin-top: 2px;
  }

  .integration-badge-group {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .masked-box {
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 4px 8px;
    font-size: 0.75rem;
    color: var(--text-secondary);
    max-width: 320px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .actions-row {
    display: inline-flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
  }
</style>
