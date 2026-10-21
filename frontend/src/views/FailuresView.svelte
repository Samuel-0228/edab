<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    showConfirm,
    summaryCounts,
    notifySuccess,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { DeadLetterRecord } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let failures = $state<DeadLetterRecord[]>([]);
  let loading = $state(true);

  async function loadFailures() {
    loading = true;
    try {
      failures = await api.listFailures();
      summaryCounts.update((s) => ({
        ...s,
        deadLetters: failures.length
      }));
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load dead-letter queue';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    loadFailures();
  });

  onMount(() => {
    loadFailures();
  });

  async function retryFailure(id: string) {
    try {
      await api.retryFailure(id);
      notifySuccess('Dead-letter item reset and queued for retry');
      await loadFailures();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to retry failure';
      notifyError(msg);
    }
  }

  function confirmDiscard(f: DeadLetterRecord) {
    showConfirm({
      title: 'Discard Dead-Letter Item',
      message: `Permanently discard failure record for Execution ${f.execution_id.slice(0, 8)}...? This event will no longer be retried.`,
      confirmLabel: 'Discard Item',
      danger: true,
      onConfirm: async () => {
        try {
          await api.discardFailure(f.id);
          notifySuccess('Dead-letter item discarded');
          await loadFailures();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : 'Failed to discard item';
          notifyError(msg);
        }
      }
    });
  }
</script>

<div class="failures-view">
  <Card
    title={`Dead-Letter Queue (${failures.length})`}
    subtitle="Failed executions with exhausted retries requiring operator inspection, diagnosis, retry, or discard."
    padding="none"
  >
    {#snippet headerActions()}
      <Button variant="secondary" size="sm" icon="refresh" onclick={loadFailures}>
        Refresh Queue
      </Button>
    {/snippet}

    {#if loading && failures.length === 0}
      <div class="table-loading">Checking dead-letter queue...</div>
    {:else if failures.length > 0}
      <div class="table-container">
        <table class="tech-table">
          <thead>
            <tr>
              <th>Timestamp</th>
              <th>Workflow ID</th>
              <th>Execution ID</th>
              <th>Failure Reason</th>
              <th>Attempts</th>
              <th style="text-align: right;">Recovery Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each failures as f}
              <tr>
                <td class="text-muted">{new Date(f.failed_at).toLocaleTimeString()}</td>
                <td>
                  <code class="mono-id">{f.workflow_id.slice(0, 8)}...</code>
                </td>
                <td>
                  <code class="mono-id">{f.execution_id.slice(0, 8)}...</code>
                </td>
                <td>
                  <div class="failure-reason mono" title={f.error}>
                    {f.error}
                  </div>
                </td>
                <td class="mono">{f.attempts}x</td>
                <td style="text-align: right;">
                  <div class="row-actions">
                    <Button
                      variant="primary"
                      size="sm"
                      onclick={() => retryFailure(f.id)}
                    >
                      Retry Delivery
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      onclick={() => confirmDiscard(f)}
                    >
                      Discard
                    </Button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <EmptyState
        title="Dead-Letter Queue is Clean"
        description="All execution pipelines have processed cleanly or are actively succeeding within retry backoff limits. Zero stalled failures."
        icon="check"
      />
    {/if}
  </Card>
</div>

<style>
  .failures-view {
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

  .failure-reason {
    color: var(--accent-crimson);
    font-size: 11.5px;
    max-width: 340px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
  }

  .mono-id {
    font-size: 11px;
  }

  .text-muted {
    color: var(--text-muted);
  }
</style>
