<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    summaryCounts,
    engineConnection,
    openCreateWorkflow,
    openDispatchModal,
    openExecutionDetail,
    openTourModal,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { OverviewResponse } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import StatCard from '../components/common/StatCard.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let data = $state<OverviewResponse | null>(null);
  let loading = $state(true);

  async function loadData() {
    loading = true;
    try {
      const res = await api.getOverview();
      data = res;
      summaryCounts.set({
        activeWorkflows: res.active_workflows,
        totalWorkflows: res.total_workflows,
        deadLetters: res.dead_letter_count,
        executionsTotal: res.executions_total,
        uptimeSeconds: res.uptime_seconds
      });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load overview data';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    // Re-fetch on refreshTick
    void $refreshTick;
    loadData();
  });

  onMount(() => {
    loadData();
  });
</script>

<div class="overview-view">
  <!-- Operational State Banner -->
  <div class="system-status-banner {$engineConnection.online === false ? 'offline' : ''}">
    <div class="status-left">
      <span class="status-indicator {$engineConnection.online === false ? 'offline' : ''}"></span>
      <div>
        <div class="status-heading">
          {$engineConnection.online === false
            ? 'KEROB ENGINE OFFLINE / UNREACHABLE'
            : 'KEROB ENGINE OPERATIONAL'}
        </div>
        <div class="status-desc">
          {$engineConnection.online === false
            ? 'Connection to backend failed. Check that the Kerob engine process or Docker container is started on port 8080.'
            : 'Core runtime active with PostgreSQL persistent storage. Real-time deterministic event routing.'}
        </div>
      </div>
    </div>

    <div class="status-right">
      {#if $engineConnection.online === false}
        <Button variant="primary" size="sm" icon="refresh" onclick={loadData}>
          Retry Connection
        </Button>
      {:else}
        <Button variant="secondary" size="sm" onclick={openTourModal}>
          Architectural Walkthrough
        </Button>
      {/if}
    </div>
  </div>

  <!-- Key Operational Metric Panels -->
  <div class="grid-4 metrics-row">
    <StatCard
      label="Active Automations"
      value={data?.active_workflows ?? '-'}
      subtext={`${data?.total_workflows ?? 0} total configured`}
      indicator={data && data.active_workflows > 0 ? 'positive' : 'neutral'}
      icon="workflow"
    />

    <StatCard
      label="Execution Runs"
      value={data?.executions_total ?? '-'}
      subtext={`${data?.executions_success ?? 0} successful`}
      indicator="neutral"
      icon="execution"
    />

    <StatCard
      label="Delivery Success Rate"
      value={data ? `${data.success_rate.toFixed(1)}%` : '-'}
      subtext={data && data.executions_failed > 0
        ? `${data.executions_failed} failed runs`
        : 'Zero unrecovered failures'}
      indicator={data && data.success_rate > 95 ? 'positive' : 'warning'}
      icon="check"
    />

    <StatCard
      label="Dead-Letter Depth"
      value={data?.dead_letter_count ?? '-'}
      subtext={data?.dead_letter_count === 0
        ? 'All retries cleared'
        : 'Requires operator inspection'}
      indicator={data && data.dead_letter_count > 0 ? 'negative' : 'positive'}
      icon="failure"
    />
  </div>

  <!-- Middle Row: Success Rate Bar & Platform Diagnostics -->
  <div class="grid-2 middle-row">
    <Card title="Throughput & Success Rate" subtitle="Continuous execution reliability ratio">
      <div class="success-rate-container">
        <div class="rate-header">
          <span class="rate-label">Success vs Failure</span>
          <span class="rate-value mono">{data ? `${data.success_rate.toFixed(1)}%` : '100%'}</span>
        </div>
        <div class="rate-track">
          <div
            class="rate-fill"
            style="width: {data ? Math.min(100, Math.max(0, data.success_rate)) : 100}%"
          ></div>
        </div>
        <div class="rate-meta">
          <span class="mono">Success: {data?.executions_success ?? 0}</span>
          <span class="mono">Failed: {data?.executions_failed ?? 0}</span>
          <span class="mono">Total: {data?.executions_total ?? 0}</span>
        </div>
      </div>
    </Card>

    <Card title="Runtime Environment" subtitle="Engine connectivity & security stance">
      <div class="env-specs">
        <div class="env-row">
          <span class="env-key">Engine Architecture</span>
          <Badge variant="neutral">Go + PostgreSQL Standard</Badge>
        </div>
        <div class="env-row">
          <span class="env-key">Active Adapters</span>
          <span class="env-val mono">{data?.registered_adapters ?? 6} registered</span>
        </div>
        <div class="env-row">
          <span class="env-key">Encrypted Credentials Vault</span>
          <span class="env-val mono">{data?.connected_credentials ?? 0} tokens stored</span>
        </div>
        <div class="env-row">
          <span class="env-key">Engine Uptime</span>
          <span class="env-val mono"
            >{data ? `${Math.floor(data.uptime_seconds / 60)} minutes` : '-'}</span
          >
        </div>
      </div>
    </Card>
  </div>

  <!-- Recent Executions Table -->
  <Card
    title="Recent Execution Runs"
    subtitle="Latest pipeline invocations and execution statuses"
    padding="none"
  >
    {#snippet headerActions()}
      <Button variant="ghost" size="sm" onclick={() => openDispatchModal()}>
        Dispatch Test Event
      </Button>
      <Button variant="primary" size="sm" onclick={() => openCreateWorkflow()}>
        New Automation
      </Button>
    {/snippet}

    {#if loading && !data}
      <div class="table-loading">Loading recent execution runs...</div>
    {:else if data?.recent_executions && data.recent_executions.length > 0}
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
            {#each data.recent_executions as exec}
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
                <td class="text-muted">{new Date(exec.started_at).toLocaleTimeString()}</td>
                <td style="text-align: right;">
                  <Button variant="ghost" size="sm" onclick={() => openExecutionDetail(exec.id)}>
                    Timeline
                  </Button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <EmptyState
        title="No execution runs recorded yet"
        description="Ingest an event or trigger a workflow test to see real execution runs recorded here."
        icon="execution"
      >
        {#snippet action()}
          <Button variant="primary" size="sm" onclick={() => openDispatchModal()}>
            Dispatch Test Event
          </Button>
        {/snippet}
      </EmptyState>
    {/if}
  </Card>
</div>

<style>
  .overview-view {
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .system-status-banner {
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 14px 18px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    position: relative;
    overflow: hidden;
  }

  .system-status-banner::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    background: var(--accent-emerald);
    transition: background 0.2s ease;
  }

  .system-status-banner.offline::before {
    background: var(--accent-crimson);
  }

  .status-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .status-indicator {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--accent-emerald);
    box-shadow: 0 0 10px rgba(0, 229, 153, 0.7);
    transition: all 0.2s ease;
  }

  .status-indicator.offline {
    background: var(--accent-crimson);
    box-shadow: 0 0 10px rgba(255, 51, 85, 0.7);
  }

  .status-heading {
    font-size: 13px;
    font-weight: 700;
    letter-spacing: 0.04em;
    color: var(--text-primary);
  }

  .status-desc {
    font-size: 11.5px;
    color: var(--text-muted);
    margin-top: 1px;
  }

  .metrics-row {
    margin-bottom: 2px;
  }

  .middle-row {
    margin-bottom: 2px;
  }

  .success-rate-container {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .rate-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .rate-label {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .rate-value {
    font-size: 18px;
    font-weight: 700;
    color: var(--accent-emerald);
  }

  .rate-track {
    width: 100%;
    height: 6px;
    background: var(--bg-surface-elevated);
    border-radius: 3px;
    overflow: hidden;
    position: relative;
  }

  .rate-fill {
    height: 100%;
    background: var(--accent-emerald);
    transition: width 0.3s ease-out;
  }

  .rate-meta {
    display: flex;
    justify-content: space-between;
    font-size: 11px;
    color: var(--text-muted);
  }

  .env-specs {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .env-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 8px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .env-row:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }

  .env-key {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .env-val {
    font-size: 11.5px;
    color: var(--text-primary);
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
    padding: 10px 14px;
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-secondary);
  }

  .tech-table tr:hover td {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
  }

  .mono-id {
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .text-muted {
    color: var(--text-muted);
  }
</style>
