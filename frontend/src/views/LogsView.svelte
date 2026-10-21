<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../api/client';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Input from '../components/common/Input.svelte';
  import Select from '../components/common/Select.svelte';
  import Switch from '../components/common/Switch.svelte';

  interface LogEntry {
    id: string;
    timestamp: Date;
    level: 'INFO' | 'WARN' | 'ERROR';
    source: string;
    message: string;
    meta?: Record<string, unknown>;
  }

  let logs = $state<LogEntry[]>([]);
  let levelFilter = $state('ALL');
  let searchFilter = $state('');
  let autoRefresh = $state(true);
  let pollInterval = $state('3000');
  let timerId: any = null;

  async function pollLogs() {
    try {
      const [execs, failures, events] = await Promise.all([
        api.listExecutions({ limit: 30 }).catch(() => []),
        api.listFailures(true).catch(() => []),
        api.listEvents({ limit: 30 }).catch(() => [])
      ]);

      const entries: LogEntry[] = [];

      for (const e of execs) {
        entries.push({
          id: `exec-${e.id}`,
          timestamp: new Date(e.started_at),
          level: e.status === 'failed' ? 'ERROR' : e.status === 'retrying' ? 'WARN' : 'INFO',
          source: `workflow:${e.workflow_id.slice(0, 8)}`,
          message: `Execution ${e.id.slice(0, 8)}: status=${e.status} duration=${e.duration_ms}ms attempts=${e.attempts}`,
          meta: e.error ? { error: e.error } : undefined
        });
      }

      for (const f of failures) {
        entries.push({
          id: `fail-${f.id}`,
          timestamp: new Date(f.failed_at),
          level: 'ERROR',
          source: `dead-letter`,
          message: `Delivery failed exhausted retries (${f.attempts}x): ${f.error}`,
          meta: { execution_id: f.execution_id, workflow_id: f.workflow_id }
        });
      }

      for (const ev of events) {
        entries.push({
          id: `evt-${ev.id}`,
          timestamp: new Date(ev.created_at),
          level: 'INFO',
          source: `event:${ev.source_id}`,
          message: `Ingested ${ev.event_type} (ext: ${ev.external_event_id})`,
          meta: ev.payload
        });
      }

      // Sort chronological descending
      entries.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
      logs = entries;
    } catch (e) {
      console.error(e);
    }
  }

  function startPolling() {
    stopPolling();
    pollLogs();
    if (autoRefresh) {
      timerId = setInterval(pollLogs, parseInt(pollInterval, 10));
    }
  }

  function stopPolling() {
    if (timerId) {
      clearInterval(timerId);
      timerId = null;
    }
  }

  $effect(() => {
    if (autoRefresh) {
      startPolling();
    } else {
      stopPolling();
    }
  });

  onMount(() => {
    startPolling();
  });

  onDestroy(() => {
    stopPolling();
  });

  let filteredLogs = $derived.by(() => {
    return logs.filter((l) => {
      if (levelFilter !== 'ALL' && l.level !== levelFilter) return false;
      if (searchFilter.trim()) {
        const q = searchFilter.toLowerCase();
        const matchesMsg = l.message.toLowerCase().includes(q);
        const matchesSource = l.source.toLowerCase().includes(q);
        if (!matchesMsg && !matchesSource) return false;
      }
      return true;
    });
  });

  function clearBuffer() {
    logs = [];
  }
</script>

<div class="logs-view">
  <!-- Controls Toolbar -->
  <Card padding="sm">
    <div class="logs-toolbar">
      <div class="filter-controls">
        <Select
          bind:value={levelFilter}
          options={[
            { value: 'ALL', label: 'All Levels' },
            { value: 'INFO', label: 'INFO Only' },
            { value: 'WARN', label: 'WARN Only' },
            { value: 'ERROR', label: 'ERROR Only' }
          ]}
          class="level-select"
        />

        <Input
          placeholder="Filter log messages..."
          bind:value={searchFilter}
          mono
          class="search-input"
        />
      </div>

      <div class="toolbar-options">
        <Switch
          label="Live Stream"
          bind:checked={autoRefresh}
          description={autoRefresh ? 'Streaming (3s)' : 'Paused'}
        />

        <Button variant="ghost" size="sm" onclick={clearBuffer}>
          Clear
        </Button>

        <Button variant="secondary" size="sm" icon="refresh" onclick={pollLogs}>
          Poll Now
        </Button>
      </div>
    </div>
  </Card>

  <!-- Terminal Log Console -->
  <Card
    title={`Live Engine Logs (${filteredLogs.length})`}
    subtitle="Structured timeline of ingested events, execution dispatches, and delivery status updates."
    padding="none"
  >
    <div class="terminal-container">
      <div class="terminal-body">
        {#if filteredLogs.length > 0}
          {#each filteredLogs as log (log.id)}
            <div class="terminal-line {log.level.toLowerCase()}">
              <span class="log-time">{log.timestamp.toLocaleTimeString()}</span>
              <span class="log-badge {log.level.toLowerCase()}">{log.level}</span>
              <span class="log-source">[{log.source}]</span>
              <span class="log-msg">{log.message}</span>
              {#if log.meta}
                <span class="log-meta">{JSON.stringify(log.meta)}</span>
              {/if}
            </div>
          {/each}
        {:else}
          <div class="terminal-empty">No log records matching active filters.</div>
        {/if}
      </div>
    </div>
  </Card>
</div>

<style>
  .logs-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .logs-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    flex-wrap: wrap;
  }

  .filter-controls {
    display: flex;
    align-items: center;
    gap: 10px;
    flex: 1;
    min-width: 280px;
  }

  :global(.level-select) {
    max-width: 140px;
  }
  :global(.search-input) {
    max-width: 280px;
  }

  .toolbar-options {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .terminal-container {
    background: #06070a;
    border-top: 1px solid var(--border-default);
    font-family: var(--font-mono);
    min-height: 480px;
    max-height: calc(100vh - 280px);
    overflow-y: auto;
  }

  .terminal-body {
    padding: 12px 16px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .terminal-line {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    font-size: 11px;
    line-height: 1.6;
    padding: 2px 4px;
    border-radius: var(--radius-xs);
    white-space: pre-wrap;
    word-break: break-all;
  }

  .terminal-line:hover {
    background: rgba(255, 255, 255, 0.03);
  }

  .log-time {
    color: var(--text-faint);
    flex-shrink: 0;
  }

  .log-badge {
    font-size: 9.5px;
    font-weight: 700;
    padding: 0 4px;
    border-radius: 2px;
    flex-shrink: 0;
  }
  .log-badge.info {
    color: var(--accent-cyan);
    background: var(--accent-cyan-dim);
  }
  .log-badge.warn {
    color: var(--accent-amber);
    background: var(--accent-amber-dim);
  }
  .log-badge.error {
    color: var(--accent-crimson);
    background: var(--accent-crimson-dim);
  }

  .log-source {
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .log-msg {
    color: var(--text-primary);
  }

  .log-meta {
    color: var(--text-faint);
    font-size: 10px;
    margin-left: 6px;
  }

  .terminal-empty {
    padding: 40px;
    text-align: center;
    color: var(--text-faint);
    font-size: 12px;
  }
</style>
