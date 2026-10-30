<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    openCreateWorkflow,
    openEditWorkflow,
    notifySuccess,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { Workflow } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let workflows = $state<Workflow[]>([]);
  let loading = $state(true);

  async function loadSchedules() {
    loading = true;
    try {
      const all = await api.listWorkflows();
      workflows = all.filter((w) => w.trigger?.type === 'schedule');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load scheduled workflows';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    loadSchedules();
  });

  onMount(() => {
    loadSchedules();
  });

  function getCadence(wf: Workflow): string {
    const cfg = wf.trigger?.config;
    if (!cfg) return 'Every 5m (default)';
    if (typeof cfg.cron === 'string' && cfg.cron) return `Cron: ${cfg.cron}`;
    if (typeof cfg.interval === 'string' && cfg.interval) return `Every ${cfg.interval}`;
    return 'Periodic Evaluation';
  }

  async function toggleStatus(wf: Workflow) {
    try {
      await api.patchWorkflow(wf.id, { enabled: !wf.enabled });
      notifySuccess(`Schedule "${wf.name}" is now ${!wf.enabled ? 'active' : 'disabled'}`);
      await loadSchedules();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to update schedule status';
      notifyError(msg);
    }
  }

  async function triggerScheduleNow(wf: Workflow) {
    try {
      await api.testWorkflow(wf.id, {
        event_type: 'schedule.tick',
        source_id: 'scheduler',
        payload: {
          scheduled_at: new Date().toISOString(),
          manual_trigger: true
        }
      });
      notifySuccess(`Scheduled tick fired for "${wf.name}"`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to fire schedule tick';
      notifyError(msg);
    }
  }
</script>

<div class="schedules-view">
  <Card
    title={`Scheduled Automations (${workflows.length})`}
    subtitle="Periodic cron routines and fixed interval triggers evaluated automatically by the Kerob background scheduler."
    padding="none"
  >
    {#snippet headerActions()}
      <Button
        variant="primary"
        size="sm"
        icon="plus"
        onclick={() => openCreateWorkflow('schedule')}
      >
        New Scheduled Automation
      </Button>
    {/snippet}

    {#if loading && workflows.length === 0}
      <div class="table-loading">Loading scheduled pipelines...</div>
    {:else if workflows.length > 0}
      <div class="table-container">
        <table class="tech-table">
          <thead>
            <tr>
              <th>Scheduled Automation</th>
              <th>Cadence / Expression</th>
              <th>Actions Executed</th>
              <th>Status</th>
              <th style="text-align: right;">Scheduler Controls</th>
            </tr>
          </thead>
          <tbody>
            {#each workflows as wf (wf.id)}
              <tr>
                <td>
                  <div class="wf-name">{wf.name}</div>
                  <div class="wf-desc">{wf.description || 'Periodic background automation'}</div>
                  <div class="wf-id mono">ID: <code>{wf.id.slice(0, 8)}...</code></div>
                </td>
                <td>
                  <Badge variant="stage-when">SCHEDULE</Badge>
                  <span class="cadence-text mono">{getCadence(wf)}</span>
                </td>
                <td>
                  {#if wf.actions && wf.actions.length > 0}
                    <div class="actions-list">
                      {#each wf.actions as act, actIdx (actIdx)}
                        <div class="action-entry mono">
                          <Badge variant="stage-then">THEN</Badge>
                          <span>{act.integration}:{act.action_type}</span>
                        </div>
                      {/each}
                    </div>
                  {:else}
                    <span class="text-faint mono">None</span>
                  {/if}
                </td>
                <td>
                  <Badge variant={wf.enabled ? 'success' : 'warning'}>
                    {wf.enabled ? 'ACTIVE TICK' : 'PAUSED'}
                  </Badge>
                </td>
                <td style="text-align: right;">
                  <div class="row-actions">
                    <Button
                      variant="ghost"
                      size="sm"
                      icon="play"
                      onclick={() => triggerScheduleNow(wf)}
                    >
                      Trigger Now
                    </Button>
                    <Button variant="ghost" size="sm" onclick={() => toggleStatus(wf)}>
                      {wf.enabled ? 'Pause' : 'Resume'}
                    </Button>
                    <Button variant="secondary" size="sm" onclick={() => openEditWorkflow(wf)}>
                      Edit
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
        title="No scheduled automations configured"
        description="Configure cron expressions or fixed intervals (e.g. 5m, 1h, 24h) to run periodic heartbeats, health checks, or sync pipelines."
        icon="schedule"
      >
        {#snippet action()}
          <Button variant="primary" size="sm" onclick={() => openCreateWorkflow('schedule')}>
            Create Scheduled Automation
          </Button>
        {/snippet}
      </EmptyState>
    {/if}
  </Card>
</div>

<style>
  .schedules-view {
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
    vertical-align: top;
  }

  .tech-table tr:hover td {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
  }

  .wf-name {
    font-weight: 600;
    color: var(--text-primary);
    font-size: 13px;
  }

  .wf-desc {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  .wf-id {
    font-size: 10px;
    color: var(--text-faint);
    margin-top: 3px;
  }

  .cadence-text {
    font-size: 11.5px;
    color: var(--accent-cyan);
    margin-left: 6px;
  }

  .actions-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .action-entry {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
  }

  .row-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
  }

  .text-faint {
    color: var(--text-faint);
  }
</style>
