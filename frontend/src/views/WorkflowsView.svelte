<script lang="ts">
  import { onMount } from 'svelte';
  import {
    refreshTick,
    openCreateWorkflow,
    openEditWorkflow,
    openDispatchModal,
    showConfirm,
    summaryCounts,
    notifySuccess,
    notifyError
  } from '../api/stores';
  import { api } from '../api/client';
  import type { Workflow } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import Input from '../components/common/Input.svelte';
  import Select from '../components/common/Select.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';

  let workflows = $state<Workflow[]>([]);
  let loading = $state(true);
  let searchQuery = $state('');
  let statusFilter = $state('all');
  let triggerFilter = $state('all');

  async function loadWorkflows() {
    loading = true;
    try {
      workflows = await api.listWorkflows();
      summaryCounts.update((s) => ({
        ...s,
        activeWorkflows: workflows.filter((w) => w.enabled).length,
        totalWorkflows: workflows.length
      }));
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load automations';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    loadWorkflows();
  });

  onMount(() => {
    loadWorkflows();
  });

  let filteredWorkflows = $derived.by(() => {
    return workflows.filter((wf) => {
      if (statusFilter === 'active' && !wf.enabled) return false;
      if (statusFilter === 'disabled' && wf.enabled) return false;
      if (triggerFilter !== 'all' && wf.trigger?.type !== triggerFilter) return false;
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const matchesName = wf.name.toLowerCase().includes(q);
        const matchesDesc = (wf.description || '').toLowerCase().includes(q);
        const matchesTrigger = (wf.trigger?.type || '').toLowerCase().includes(q);
        const matchesId = wf.id.toLowerCase().includes(q);
        if (!matchesName && !matchesDesc && !matchesTrigger && !matchesId) return false;
      }
      return true;
    });
  });

  async function toggleWorkflow(wf: Workflow) {
    try {
      const updated = await api.patchWorkflow(wf.id, { enabled: !wf.enabled });
      notifySuccess(`Automation "${wf.name}" is now ${updated.enabled ? 'active' : 'disabled'}`);
      await loadWorkflows();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to toggle status';
      notifyError(msg);
    }
  }

  function confirmDelete(wf: Workflow) {
    showConfirm({
      title: 'Delete Automation Pipeline',
      message: `Are you sure you want to delete "${wf.name}"? Ingested events will no longer trigger this rule.`,
      confirmLabel: 'Delete Automation',
      danger: true,
      onConfirm: async () => {
        try {
          await api.deleteWorkflow(wf.id);
          notifySuccess(`Automation "${wf.name}" deleted`);
          await loadWorkflows();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : 'Failed to delete automation';
          notifyError(msg);
        }
      }
    });
  }
</script>

<div class="workflows-view">
  <!-- Top Filter Toolbar -->
  <Card padding="sm" class="filter-card">
    <div class="filter-toolbar">
      <div class="filter-inputs">
        <Input
          placeholder="Search automations by name, trigger, or ID..."
          bind:value={searchQuery}
          class="search-field"
        />

        <Select
          bind:value={statusFilter}
          options={[
            { value: 'all', label: 'All Statuses' },
            { value: 'active', label: 'Active Only' },
            { value: 'disabled', label: 'Disabled Only' }
          ]}
          class="status-select"
        />

        <Select
          bind:value={triggerFilter}
          options={[
            { value: 'all', label: 'All Triggers' },
            { value: 'telegram', label: 'Telegram' },
            { value: 'github', label: 'GitHub' },
            { value: 'webhook', label: 'Webhook' },
            { value: 'schedule', label: 'Schedule' },
            { value: 'rss', label: 'RSS Feed' },
            { value: 'discord', label: 'Discord' }
          ]}
          class="trigger-select"
        />
      </div>

      <div class="toolbar-actions">
        <Button variant="secondary" size="sm" onclick={() => openDispatchModal()}>
          Test Trigger
        </Button>
        <Button variant="primary" size="sm" icon="plus" onclick={() => openCreateWorkflow()}>
          New Automation
        </Button>
      </div>
    </div>
  </Card>

  <!-- Workflows Table -->
  <Card
    title={`Configured Pipelines (${filteredWorkflows.length})`}
    subtitle="Declarative WHEN → IF → TRANSFORM → THEN automation execution rules."
    padding="none"
  >
    {#if loading && workflows.length === 0}
      <div class="table-loading">Loading automation pipelines...</div>
    {:else if filteredWorkflows.length > 0}
      <div class="table-container">
        <table class="tech-table">
          <thead>
            <tr>
              <th>Automation Rule</th>
              <th>WHEN Trigger</th>
              <th>IF Condition</th>
              <th>THEN Action(s)</th>
              <th>Status</th>
              <th>Retries</th>
              <th style="text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each filteredWorkflows as wf}
              <tr>
                <td>
                  <div class="wf-title">{wf.name}</div>
                  <div class="wf-desc">{wf.description || 'No description provided'}</div>
                  <div class="wf-id mono">ID: <code>{wf.id.slice(0, 8)}...</code></div>
                </td>

                <td>
                  <Badge variant="stage-when">WHEN</Badge>
                  <span class="trigger-name mono">{wf.trigger?.type}</span>
                  {#if wf.trigger?.event_types && wf.trigger.event_types[0] !== '*'}
                    <div class="trigger-events mono">[{wf.trigger.event_types.join(', ')}]</div>
                  {/if}
                  {#if wf.trigger?.type === 'schedule' && wf.trigger?.config}
                    <div class="schedule-cadence mono">
                      {(wf.trigger.config as any).cron ||
                        (wf.trigger.config as any).interval ||
                        'cadence'}
                    </div>
                  {/if}
                </td>

                <td>
                  {#if wf.conditions && wf.conditions.operator}
                    <Badge variant="stage-if">IF</Badge>
                    <span class="condition-expr mono">
                      {wf.conditions.field}
                      {wf.conditions.operator} "{String(wf.conditions.value ?? '')}"
                    </span>
                  {:else}
                    <span class="text-faint mono">— Always True —</span>
                  {/if}
                </td>

                <td>
                  {#if wf.actions && wf.actions.length > 0}
                    <div class="actions-list">
                      {#each wf.actions as act}
                        <div class="action-item mono">
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
                    {wf.enabled ? 'ACTIVE' : 'DISABLED'}
                  </Badge>
                </td>

                <td class="mono">{wf.max_retries || 3}x</td>

                <td style="text-align: right;">
                  <div class="row-actions">
                    <Button
                      variant="ghost"
                      size="sm"
                      onclick={() => openDispatchModal(wf.id, wf.name)}
                    >
                      Test
                    </Button>
                    <Button variant="ghost" size="sm" onclick={() => toggleWorkflow(wf)}>
                      {wf.enabled ? 'Disable' : 'Enable'}
                    </Button>
                    <Button variant="secondary" size="sm" onclick={() => openEditWorkflow(wf)}>
                      Edit
                    </Button>
                    <Button variant="danger" size="sm" onclick={() => confirmDelete(wf)}>
                      Delete
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
        title="No automation pipelines found"
        description={searchQuery.trim() || statusFilter !== 'all' || triggerFilter !== 'all'
          ? 'Try adjusting your filters or search keywords.'
          : 'Create your first event-driven automation pipeline to connect messaging networks and APIs.'}
        icon="workflow"
      >
        {#snippet action()}
          <Button variant="primary" size="sm" onclick={() => openCreateWorkflow()}>
            Create Automation
          </Button>
        {/snippet}
      </EmptyState>
    {/if}
  </Card>
</div>

<style>
  .workflows-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .filter-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }

  .filter-inputs {
    display: flex;
    align-items: center;
    gap: 10px;
    flex: 1;
    min-width: 280px;
  }

  :global(.search-field) {
    max-width: 320px;
  }
  :global(.status-select),
  :global(.trigger-select) {
    max-width: 160px;
  }

  .toolbar-actions {
    display: flex;
    align-items: center;
    gap: 8px;
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

  .wf-title {
    font-weight: 600;
    color: var(--text-primary);
    font-size: 13px;
  }

  .wf-desc {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 2px;
    max-width: 260px;
    line-height: 1.35;
  }

  .wf-id {
    font-size: 10px;
    color: var(--text-faint);
    margin-top: 4px;
  }

  .trigger-name {
    font-size: 11.5px;
    color: var(--text-primary);
    margin-left: 4px;
  }

  .trigger-events {
    font-size: 10px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  .schedule-cadence {
    font-size: 10px;
    color: var(--accent-cyan);
    margin-top: 2px;
  }

  .condition-expr {
    font-size: 11px;
    color: var(--text-primary);
    margin-left: 4px;
  }

  .actions-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .action-item {
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
