<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    openExecutionDetail,
    notifySuccess,
    notifyError,
    currentIdentity,
    canOperateActions
  } from '../api/stores';
  import { api } from '../api/client';
  import type { Execution, Approval } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import Select from '../components/common/Select.svelte';
  import Input from '../components/common/Input.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let executions = $state<Execution[]>([]);
  let pendingApprovals = $state<Approval[]>([]);
  let loading = $state(true);
  let statusFilter = $state('all');
  let workflowIdFilter = $state('');
  let limit = $state(50);
  let offset = $state(0);

  async function loadExecutions() {
    loading = true;
    try {
      const [execData, appData] = await Promise.all([
        api.listExecutions({
          status: statusFilter !== 'all' ? statusFilter : undefined,
          workflow_id: workflowIdFilter.trim() || undefined,
          limit,
          offset
        }),
        api.listApprovals({ status: 'pending', limit: 20 }).catch(() => [])
      ]);
      executions = execData;
      pendingApprovals = appData;
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

  async function handleQuickApprove(appId: string) {
    try {
      await api.approveApproval(appId, 'Approved from Operations dashboard');
      notifySuccess('Approval granted — resuming workflow execution');
      await loadExecutions();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Approval failed';
      notifyError(msg);
    }
  }

  async function handleQuickReject(appId: string) {
    try {
      await api.rejectApproval(appId, 'Rejected from Operations dashboard');
      notifySuccess('Approval rejected — initiated saga compensation');
      await loadExecutions();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Rejection failed';
      notifyError(msg);
    }
  }

  async function handleCompensate(id: string) {
    if (
      !confirm(
        'Initiate saga rollback compensation for this execution? Completed steps with defined compensation actions will be undone.'
      )
    ) {
      return;
    }
    try {
      await api.compensateExecution(id, 'Operator-initiated rollback');
      notifySuccess('Saga compensation completed');
      await loadExecutions();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Compensation failed';
      notifyError(msg);
    }
  }

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

  async function handlePause(id: string) {
    try {
      await api.pauseExecution(id);
      notifySuccess('Execution paused');
      await loadExecutions();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Pause failed';
      notifyError(msg);
    }
  }

  async function handleResume(id: string) {
    try {
      await api.resumeExecution(id);
      notifySuccess('Execution resumed');
      await loadExecutions();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Resume failed';
      notifyError(msg);
    }
  }

  async function handleCancel(id: string) {
    try {
      await api.cancelExecution(id);
      notifySuccess('Execution cancelled');
      await loadExecutions();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Cancel failed';
      notifyError(msg);
    }
  }
</script>

<div class="executions-view">
  <!-- Pending Approvals Banner if any -->
  {#if pendingApprovals.length > 0}
    <Card
      title={`Pending Human Approvals (${pendingApprovals.length})`}
      subtitle="Execution steps halted in durable non-blocking waiting state awaiting explicit human decision."
      padding="sm"
    >
      <div class="approvals-grid">
        {#each pendingApprovals as app (app.id)}
          <div class="approval-card">
            <div class="approval-info">
              <div class="approval-title-row">
                <Badge variant="warning">AWAITING APPROVAL</Badge>
                <span class="approval-title">{app.title || 'Step Approval Required'}</span>
              </div>
              {#if app.description}
                <div class="approval-desc">{app.description}</div>
              {/if}
              <div class="approval-meta">
                <span
                  >Execution: <code class="mono-id">{app.execution_id.slice(0, 10)}...</code></span
                >
                <span>Workflow: <code class="mono-id">{app.workflow_id.slice(0, 8)}...</code></span>
                {#if app.approver_roles?.length}
                  <span>Roles: {app.approver_roles.join(', ')}</span>
                {/if}
                <span>Expires: {new Date(app.expires_at).toLocaleTimeString()}</span>
              </div>
            </div>
            <div class="approval-actions">
              <Button
                variant="ghost"
                size="sm"
                onclick={() => openExecutionDetail(app.execution_id)}
              >
                View Run
              </Button>
              {#if canOperateActions($currentIdentity?.role)}
                <Button variant="primary" size="sm" onclick={() => handleQuickApprove(app.id)}>
                  Approve
                </Button>
                <Button variant="danger" size="sm" onclick={() => handleQuickReject(app.id)}>
                  Reject
                </Button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    </Card>
  {/if}

  <!-- Filter Toolbar -->
  <Card padding="sm">
    <div class="filter-toolbar">
      <div class="filter-inputs">
        <Select
          bind:value={statusFilter}
          onchange={loadExecutions}
          options={[
            { value: 'all', label: 'All Statuses' },
            { value: 'waiting_approval', label: 'Waiting Approval' },
            { value: 'running', label: 'Running Only' },
            { value: 'retrying', label: 'Retrying Only' },
            { value: 'paused', label: 'Paused Only' },
            { value: 'success', label: 'Success Only' },
            { value: 'failed', label: 'Failed Only' },
            { value: 'compensating', label: 'Compensating' },
            { value: 'compensated', label: 'Compensated' },
            { value: 'compensation_failed', label: 'Comp Failed' },
            { value: 'cancelled', label: 'Cancelled Only' },
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
                  {:else if exec.status === 'waiting_approval'}
                    <Badge variant="warning">WAITING APPROVAL</Badge>
                  {:else if exec.status === 'compensating'}
                    <Badge variant="running">COMPENSATING</Badge>
                  {:else if exec.status === 'compensated'}
                    <Badge variant="neutral">COMPENSATED</Badge>
                  {:else if exec.status === 'compensation_failed'}
                    <Badge variant="failed">COMP FAILED</Badge>
                  {:else if exec.status === 'paused'}
                    <Badge variant="warning">PAUSED</Badge>
                  {:else if exec.status === 'cancelled'}
                    <Badge variant="neutral">CANCELLED</Badge>
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
                    {#if canOperateActions($currentIdentity?.role)}
                      {#if exec.status === 'failed'}
                        <Button variant="primary" size="sm" onclick={() => retryRun(exec.id)}>
                          Retry
                        </Button>
                        <Button
                          variant="danger"
                          size="sm"
                          onclick={() => handleCompensate(exec.id)}
                        >
                          Rollback
                        </Button>
                      {:else if exec.status === 'waiting_approval'}
                        <Button
                          variant="danger"
                          size="sm"
                          onclick={() => handleCompensate(exec.id)}
                        >
                          Rollback
                        </Button>
                      {:else if exec.status === 'running' || exec.status === 'pending' || exec.status === 'retrying'}
                        <Button variant="secondary" size="sm" onclick={() => handlePause(exec.id)}>
                          Pause
                        </Button>
                        <Button variant="ghost" size="sm" onclick={() => handleCancel(exec.id)}>
                          Cancel
                        </Button>
                      {:else if exec.status === 'paused'}
                        <Button variant="primary" size="sm" onclick={() => handleResume(exec.id)}>
                          Resume
                        </Button>
                        <Button variant="ghost" size="sm" onclick={() => handleCancel(exec.id)}>
                          Cancel
                        </Button>
                      {/if}
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

  .approvals-grid {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .approval-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 14px;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-subtle);
    border-left: 3px solid #eab308;
    border-radius: var(--radius-sm);
    gap: 16px;
  }

  .approval-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex: 1;
  }

  .approval-title-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .approval-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .approval-desc {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .approval-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  .approval-actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }
</style>
