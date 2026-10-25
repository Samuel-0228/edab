<script lang="ts">
  import { onMount } from 'svelte';
  import { refreshTick, openDispatchModal, notifyError } from '../api/stores';
  import { api } from '../api/client';
  import type { EventRecord } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import Input from '../components/common/Input.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';
  import CodeBox from '../components/common/CodeBox.svelte';

  let events = $state<EventRecord[]>([]);
  let loading = $state(true);
  let sourceIdFilter = $state('');
  let eventTypeFilter = $state('');

  async function loadEvents() {
    loading = true;
    try {
      events = await api.listEvents({
        source_id: sourceIdFilter.trim() || undefined,
        event_type: eventTypeFilter.trim() || undefined,
        limit: 50
      });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load events stream';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    loadEvents();
  });

  onMount(() => {
    loadEvents();
  });
</script>

<div class="events-view">
  <!-- Top Filter Toolbar -->
  <Card padding="sm">
    <div class="filter-toolbar">
      <div class="filter-inputs">
        <Input
          placeholder="Filter by Source ID..."
          bind:value={sourceIdFilter}
          onchange={loadEvents}
          mono
          class="filter-field"
        />

        <Input
          placeholder="Filter by Event Type..."
          bind:value={eventTypeFilter}
          onchange={loadEvents}
          mono
          class="filter-field"
        />
      </div>

      <div class="toolbar-actions">
        <Button variant="secondary" size="sm" icon="refresh" onclick={loadEvents}>Refresh</Button>
        <Button variant="primary" size="sm" icon="play" onclick={() => openDispatchModal()}>
          + Inject Event
        </Button>
      </div>
    </div>
  </Card>

  <!-- Events Stream Table -->
  <Card
    title={`Normalized Events Stream (${events.length})`}
    subtitle="Immutable real-time event log ingested into Kerob before workflow routing and condition matching."
    padding="none"
  >
    {#if loading && events.length === 0}
      <div class="table-loading">Loading ingested events stream...</div>
    {:else if events.length > 0}
      <div class="table-container">
        <table class="tech-table">
          <thead>
            <tr>
              <th>Timestamp</th>
              <th>Event Type</th>
              <th>Source Identifier</th>
              <th>External Event ID</th>
              <th>Normalized Payload</th>
            </tr>
          </thead>
          <tbody>
            {#each events as ev (ev.id)}
              <tr>
                <td class="text-muted timestamp-cell">
                  {new Date(ev.created_at).toLocaleTimeString()}
                </td>
                <td>
                  <Badge variant="neutral">{ev.event_type}</Badge>
                </td>
                <td>
                  <code class="mono-id">{ev.source_id}</code>
                </td>
                <td>
                  <code class="mono-id">{ev.external_event_id}</code>
                </td>
                <td class="payload-cell">
                  <CodeBox content={ev.payload} maxHeight="80px" />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <EmptyState
        title="No events recorded in log"
        description="Incoming events from Telegram, GitHub, inbound webhooks, or scheduled triggers will appear here in chronological order."
        icon="event"
      >
        {#snippet action()}
          <Button variant="primary" size="sm" onclick={() => openDispatchModal()}>
            Inject Test Event
          </Button>
        {/snippet}
      </EmptyState>
    {/if}
  </Card>
</div>

<style>
  .events-view {
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

  :global(.filter-field) {
    max-width: 220px;
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
    padding: 10px 14px;
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-secondary);
    vertical-align: top;
  }

  .tech-table tr:hover td {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
  }

  .timestamp-cell {
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .payload-cell {
    min-width: 280px;
  }

  .mono-id {
    font-size: 11px;
  }

  .text-muted {
    color: var(--text-muted);
  }
</style>
