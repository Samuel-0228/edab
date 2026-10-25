<script lang="ts">
  import { onMount } from 'svelte';
  import { refreshTick, openExecutionDetail, notifySuccess, notifyError } from '../api/stores';
  import { api } from '../api/client';
  import type { Execution } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import Select from '../components/common/Select.svelte';
  import Input from '../components/common/Input.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let executions = $state<Execution[]>([]);
  let loading = $state(true);
  let statusFilter = $state('all');
  let workflowIdFilter = $state('');
  let limit = $state(50);
  let offset = $state(0);

  async function loadExecutions() {
    loading = true;
    try {
      executions = await api.listExecutions({
        status: statusFilter !== 'all' ? statusFilter : undefined,
        workflow_id: workflowIdFilter.trim() || undefined,
        limit,
        offset
      });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load execution runs';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    loadExecutions();
  });

  onMount(() => {
    loadExecutions();
  });

  async function retryRun(id: string) {
    try {
      await api.retryExecution(id);
      notifySuccess('Retry triggered for execution run');
      await loadExecutions();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Retry failed';
      notifyError(msg);
    }
  }
</script>

<div class="executions-view">
  <!-- Filter Toolbar -->
  <Card padding="sm">
    <div class="filter-toolbar">
      <div class="filter-inputs">
        <Select
          bind:value={statusFilter}
          onchange={loadExecutions}
          options={[
            { value: 'all', label: 'All Statuses' },
            { value: 'success', label: 'Success Only' },
            { value: 'failed', label: 'Failed Only' },
            { value: 'running', label: 'Running Only' },
            { value: 'retrying', label: 'Retrying Only' },
            { value: 'pending', label: 'Pending Only' }
          ]}
          class="status-select"
        />

        <Input
          placeholder="Filter by Workflow ID..."
          bind:value={workflowIdFilter}
          onchange={loadExecutions}
          mono
          class="wf-filter"
        />
      </div>

      <div class="toolbar-actions">
        <Button variant="secondary" size="sm" icon="refresh" onclick={loadExecutions}>
          Refresh Log
        </Button>
      </div>
    </div>
  </Card>

  <!-- Executions Table -->
  <Card
    title={`Execution Runs (${executions.length})`}
    subtitle="Audit logs and granular step lifecycle inspection for every pipeline event invocation."
    padding="none"
  >
    {#if loading && executions.length === 0}
      <div class="table-loading">Loading execution history...</div>
    {:else if executions.length > 0}
      <div class="table-container">
        <table class="tech-table">
          <thead>
            <tr>
              <th>Execution ID</th>
              <th>Workflow ID</th>
              <th>Event ID</th>
              <th>Status</th>
              <th>Attempts</th>
              <th>Duration</th>
              <th>Started At</th>
              <th style="text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each executions as exec (exec.id)}
              <tr>
                <td>
                  <code class="mono-id">{exec.id.slice(0, 10)}...</code>
                </td>
                <td>
                  <code class="mono-id">{exec.workflow_id.slice(0, 8)}...</code>
                </td>
                <td>
                  <code class="mono-id">{exec.event_id.slice(0, 8)}...</code>
                </td>
                <td>
                  {#if exec.status === 'success'}
                    <Badge variant="success">SUCCESS</Badge>
                  {:else if exec.status === 'failed'}
                    <Badge variant="failed">FAILED</Badge>
                  {:else if exec.status === 'running'}
                    <Badge variant="running">RUNNING</Badge>
                  {:else}
                    <Badge variant="warning">{exec.status}</Badge>
                  {/if}
                </td>
                <td class="mono">{exec.attempts} / {exec.max_attempts}</td>
                <td class="mono">{exec.duration_ms} ms</td>
                <td class="text-muted">{new Date(exec.started_at).toLocaleString()}</td>
                <td style="text-align: right;">
                  <div class="row-actions">
                    <Button variant="ghost" size="sm" onclick={() => openExecutionDetail(exec.id)}>
                      Timeline
                    </Button>
                    {#if exec.status === 'failed'}
                      <Button variant="primary" size="sm" onclick={() => retryRun(exec.id)}>
                        Retry
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
        title="No execution runs matching criteria"
        description="Executions are automatically generated when matching triggers fire. You can also trigger manual tests from the Automations tab."
        icon="execution"
      />
    {/if}
  </Card>
</div>

<style>
  .executions-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .filter-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .filter-inputs {
    display: flex;
    align-items: center;
    gap: 10px;
    flex: 1;
  }

  :global(.status-select) {
    max-width: 180px;
  }
  :global(.wf-filter) {
    max-width: 260px;
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
    padding: 11px 14px;
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-secondary);
  }

  .tech-table tr:hover td {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
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
