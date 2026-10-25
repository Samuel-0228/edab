<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    openCreateWorkflow,
    openAddCredentialModal,
    navigate,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { IntegrationMetadata } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';

  let catalog = $state<IntegrationMetadata[]>([]);

  async function loadCatalog() {
    try {
      catalog = await api.listIntegrations();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load integrations catalog';
      notifyError(msg);
    }
  }

  $effect(() => {
    void $refreshTick;
    loadCatalog();
  });

  onMount(() => {
    loadCatalog();
  });
</script>

<div class="integrations-view">
  <!-- Top Intro Card -->
  <Card
    title="Supported Integration Adapters"
    subtitle="Uniform adapter contracts isolating external protocols from the Kerob core routing engine."
  >
    {#snippet headerActions()}
      <Button variant="secondary" size="sm" onclick={() => navigate('credentials')}>
        Credentials Vault
      </Button>
    {/snippet}

    <div class="catalog-grid">
      {#each catalog as item (item.id)}
        <div class="catalog-card">
          <div class="card-top">
            <div class="card-title-group">
              <span class="adapter-title">{item.name}</span>
              <Badge variant="success">PRODUCTION READY</Badge>
            </div>
            <p class="adapter-desc">{item.description}</p>
          </div>

          <div class="capabilities-box">
            <!-- Triggers -->
            <div class="capability-group">
              <span class="capability-label">SUPPORTED TRIGGERS</span>
              {#if item.triggers.length > 0}
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
              {#if item.actions.length > 0}
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
            <Button variant="secondary" size="sm" onclick={() => openCreateWorkflow(item.id)}>
              + Use in Automation
            </Button>
            {#if item.credentials.length > 0}
              <Button variant="ghost" size="sm" onclick={() => openAddCredentialModal(item.id)}>
                Configure Token
              </Button>
            {/if}
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

  .adapter-title {
    font-size: 14px;
    font-weight: 700;
    color: var(--text-primary);
    letter-spacing: -0.01em;
  }

  .adapter-desc {
    font-size: 11.5px;
    color: var(--text-muted);
    line-height: 1.45;
  }

  .capabilities-box {
    display: flex;
    flex-direction: column;
    gap: 12px;
    background: var(--bg-canvas);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 12px;
  }

  .capability-group {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .capability-label {
    font-family: var(--font-mono);
    font-size: 9.5px;
    color: var(--text-muted);
    letter-spacing: 0.05em;
    font-weight: 600;
  }

  .caps-list {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .cap-entry {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 11px;
  }

  .cap-type {
    color: var(--accent-cyan);
    font-size: 10.5px;
  }

  .cap-desc {
    color: var(--text-secondary);
  }

  .cap-none {
    font-size: 11px;
    color: var(--text-faint);
  }

  .card-footer-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
</style>
