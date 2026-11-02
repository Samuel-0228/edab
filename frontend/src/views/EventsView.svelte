<script lang="ts">
  import { onMount } from 'svelte';
  import { refreshTick, openDispatchModal, notifyError, notifySuccess } from '../api/stores';
  import { api } from '../api/client';
  import type {
    EventRecord,
    EventSchema,
    StreamDeadLetterEvent,
    StreamConsumerOffset
  } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import Input from '../components/common/Input.svelte';
  import Select from '../components/common/Select.svelte';
  import EmptyState from '../components/common/EmptyState.svelte';
  import CodeBox from '../components/common/CodeBox.svelte';
  import Dialog from '../components/common/Dialog.svelte';

  type ViewTab = 'events' | 'schemas' | 'streaming';
  let activeTab = $state<ViewTab>('events');

  // Events tab state
  let events = $state<EventRecord[]>([]);
  let loadingEvents = $state(true);
  let sourceIdFilter = $state('');
  let eventTypeFilter = $state('');

  // Schemas tab state
  let schemas = $state<EventSchema[]>([]);
  let loadingSchemas = $state(false);
  let showRegisterModal = $state(false);
  let newSchemaName = $state('');
  let newSchemaType = $state<'json_schema' | 'protobuf'>('json_schema');
  let newSchemaCompat = $state<'BACKWARD' | 'FORWARD' | 'FULL' | 'NONE'>('BACKWARD');
  let newSchemaDesc = $state('');
  let newSchemaDef = $state('{\n  "type": "object",\n  "required": ["event_type"],\n  "properties": {\n    "event_type": { "type": "string" }\n  }\n}');
  let registeringSchema = $state(false);
  let selectedSchema = $state<EventSchema | null>(null);

  // Streaming DLQ & Offsets state
  let dlqEvents = $state<StreamDeadLetterEvent[]>([]);
  let offsets = $state<StreamConsumerOffset[]>([]);
  let loadingDLQ = $state(false);
  let showReplayModal = $state(false);
  let replayTopic = $state('');
  let replayEventType = $state('');
  let replaySourceId = $state('');
  let replaySinceMinutes = $state('60');
  let replayBypassIdempotency = $state(false);
  let replayingStream = $state(false);
  let retryingDLQId = $state<string | null>(null);

  async function loadEvents() {
    loadingEvents = true;
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
      loadingEvents = false;
    }
  }

  async function loadSchemas() {
    loadingSchemas = true;
    try {
      schemas = await api.listSchemas();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load schemas';
      notifyError(msg);
    } finally {
      loadingSchemas = false;
    }
  }

  async function loadStreamingData() {
    loadingDLQ = true;
    try {
      const [dlq, offs] = await Promise.all([
        api.listStreamDLQ({ limit: 50 }),
        api.getStreamOffsets()
      ]);
      dlqEvents = dlq;
      offsets = offs;
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load streaming status';
      notifyError(msg);
    } finally {
      loadingDLQ = false;
    }
  }

  function handleTabChange(tab: ViewTab) {
    activeTab = tab;
    if (tab === 'events') {
      loadEvents();
    } else if (tab === 'schemas') {
      loadSchemas();
    } else if (tab === 'streaming') {
      loadStreamingData();
    }
  }

  async function handleRegisterSchema() {
    if (!newSchemaName.trim()) {
      notifyError('Schema name is required');
      return;
    }
    if (!newSchemaDef.trim()) {
      notifyError('Schema definition is required');
      return;
    }

    registeringSchema = true;
    try {
      await api.registerSchema({
        name: newSchemaName.trim(),
        schema_type: newSchemaType,
        compatibility: newSchemaCompat,
        description: newSchemaDesc.trim() || undefined,
        definition: newSchemaDef
      });
      notifySuccess(`Schema '${newSchemaName.trim()}' registered successfully`);
      showRegisterModal = false;
      newSchemaName = '';
      newSchemaDesc = '';
      await loadSchemas();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to register schema';
      notifyError(msg);
    } finally {
      registeringSchema = false;
    }
  }

  async function handleRetryDLQ(id: string) {
    retryingDLQId = id;
    try {
      const res = await api.retryStreamDLQ(id);
      notifySuccess(`DLQ event ${id} retried successfully (${res.status})`);
      await loadStreamingData();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to retry DLQ event';
      notifyError(msg);
    } finally {
      retryingDLQId = null;
    }
  }

  async function handleTriggerReplay() {
    replayingStream = true;
    try {
      const sinceMins = parseInt(replaySinceMinutes, 10);
      const sinceDate = !isNaN(sinceMins) && sinceMins > 0
        ? new Date(Date.now() - sinceMins * 60 * 1000).toISOString()
        : undefined;

      const res = await api.replayStream({
        topic: replayTopic.trim() || undefined,
        event_type: replayEventType.trim() || undefined,
        source_id: replaySourceId.trim() || undefined,
        since: sinceDate,
        bypass_idempotency: replayBypassIdempotency
      });

      notifySuccess(`Replayed ${res.replayed} / ${res.total_matched} events successfully`);
      showReplayModal = false;
      await loadStreamingData();
      if (activeTab === 'events') {
        await loadEvents();
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Stream replay failed';
      notifyError(msg);
    } finally {
      replayingStream = false;
    }
  }

  $effect(() => {
    void $refreshTick;
    if (activeTab === 'events') {
      loadEvents();
    } else if (activeTab === 'schemas') {
      loadSchemas();
    } else if (activeTab === 'streaming') {
      loadStreamingData();
    }
  });

  onMount(() => {
    loadEvents();
  });
</script>

<div class="events-view">
  <!-- Sub Navigation Tabs -->
  <div class="tabs-nav">
    <button
      class="tab-btn {activeTab === 'events' ? 'active' : ''}"
      onclick={() => handleTabChange('events')}
    >
      Ingested Events
    </button>
    <button
      class="tab-btn {activeTab === 'schemas' ? 'active' : ''}"
      onclick={() => handleTabChange('schemas')}
    >
      Schema Registry
    </button>
    <button
      class="tab-btn {activeTab === 'streaming' ? 'active' : ''}"
      onclick={() => handleTabChange('streaming')}
    >
      Stream DLQ & Offsets
    </button>
  </div>

  <!-- TAB 1: Ingested Events -->
  {#if activeTab === 'events'}
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

    <Card
      title={`Normalized Events Stream (${events.length})`}
      subtitle="Immutable real-time event log ingested into Kerob before workflow routing and condition matching."
      padding="none"
    >
      {#if loadingEvents && events.length === 0}
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
  {/if}

  <!-- TAB 2: Schema Registry -->
  {#if activeTab === 'schemas'}
    <Card padding="sm">
      <div class="filter-toolbar">
        <div class="section-desc">
          <span>Manage event schemas with JSON Schema / Protobuf validation and backward/forward compatibility guarantees.</span>
        </div>
        <div class="toolbar-actions">
          <Button variant="secondary" size="sm" icon="refresh" onclick={loadSchemas}>Refresh</Button>
          <Button variant="primary" size="sm" onclick={() => (showRegisterModal = true)}>
            + Register Schema
          </Button>
        </div>
      </div>
    </Card>

    <Card
      title={`Registered Schemas (${schemas.length})`}
      subtitle="Event types validated against these schemas are protected from poison pills and breaking payload drift."
      padding="none"
    >
      {#if loadingSchemas && schemas.length === 0}
        <div class="table-loading">Loading schemas...</div>
      {:else if schemas.length > 0}
        <div class="table-container">
          <table class="tech-table">
            <thead>
              <tr>
                <th>Subject / Name</th>
                <th>Version</th>
                <th>Type</th>
                <th>Compatibility</th>
                <th>Description</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each schemas as s (s.id)}
                <tr>
                  <td>
                    <strong>{s.name}</strong>
                  </td>
                  <td>
                    <Badge variant="neutral">v{s.version}</Badge>
                  </td>
                  <td>
                    <Badge variant="stage-when">{s.schema_type}</Badge>
                  </td>
                  <td>
                    <Badge variant={s.compatibility === 'BACKWARD' ? 'success' : s.compatibility === 'FULL' ? 'warning' : 'neutral'}>
                      {s.compatibility}
                    </Badge>
                  </td>
                  <td class="text-muted">{s.description || '—'}</td>
                  <td class="text-muted timestamp-cell">{new Date(s.created_at).toLocaleString()}</td>
                  <td>
                    <Button
                      variant="secondary"
                      size="sm"
                      onclick={() => (selectedSchema = selectedSchema?.id === s.id ? null : s)}
                    >
                      {selectedSchema?.id === s.id ? 'Hide' : 'View'}
                    </Button>
                  </td>
                </tr>
                {#if selectedSchema?.id === s.id}
                  <tr class="details-row">
                    <td colspan="7">
                      <div class="schema-definition-box">
                        <div class="def-header">
                          <span>Schema Definition (Version {s.version})</span>
                        </div>
                        <CodeBox content={s.definition} maxHeight="200px" />
                      </div>
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <EmptyState
          title="No schemas registered"
          description="Register JSON Schema or Protobuf schemas to enforce validation and evolution checks on incoming stream events."
          icon="event"
        >
          {#snippet action()}
            <Button variant="primary" size="sm" onclick={() => (showRegisterModal = true)}>
              Register First Schema
            </Button>
          {/snippet}
        </EmptyState>
      {/if}
    </Card>
  {/if}

  <!-- TAB 3: Stream DLQ & Offsets -->
  {#if activeTab === 'streaming'}
    <!-- Offsets overview -->
    <Card padding="sm">
      <div class="filter-toolbar">
        <div class="section-desc">
          <span>Active streaming consumers, partition offsets, and dead-letter records rejected during ingestion or validation.</span>
        </div>
        <div class="toolbar-actions">
          <Button variant="secondary" size="sm" icon="refresh" onclick={loadStreamingData}>Refresh</Button>
          <Button variant="primary" size="sm" onclick={() => (showReplayModal = true)}>
            Replay Stream
          </Button>
        </div>
      </div>
    </Card>

    {#if offsets.length > 0}
      <Card title="Consumer Partition Offsets" subtitle="Committed high-water mark offsets across consumer groups" padding="none">
        <div class="table-container">
          <table class="tech-table">
            <thead>
              <tr>
                <th>Consumer Group</th>
                <th>Topic</th>
                <th>Partition</th>
                <th>Committed Offset</th>
                <th>Committed At</th>
              </tr>
            </thead>
            <tbody>
              {#each offsets as off (off.group_id + off.topic + off.partition)}
                <tr>
                  <td><code class="mono-id">{off.group_id}</code></td>
                  <td><Badge variant="neutral">{off.topic}</Badge></td>
                  <td>#{off.partition}</td>
                  <td><strong>{off.offset}</strong></td>
                  <td class="text-muted timestamp-cell">{new Date(off.committed_at).toLocaleTimeString()}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </Card>
    {/if}

    <Card
      title={`Stream Dead-Letter Queue (${dlqEvents.length})`}
      subtitle="Failed or schema-invalid stream records stored durably for triage, repair, and replay."
      padding="none"
    >
      {#if loadingDLQ && dlqEvents.length === 0}
        <div class="table-loading">Loading stream dead-letter events...</div>
      {:else if dlqEvents.length > 0}
        <div class="table-container">
          <table class="tech-table">
            <thead>
              <tr>
                <th>Topic / Part / Off</th>
                <th>Event Type</th>
                <th>Ordering Key</th>
                <th>Reason</th>
                <th>Status</th>
                <th>Retries</th>
                <th>Payload</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each dlqEvents as d (d.id)}
                <tr>
                  <td class="mono-id">
                    {d.topic} [P{d.partition}:#{d.offset}]
                  </td>
                  <td>
                    <Badge variant="neutral">{d.event_type}</Badge>
                  </td>
                  <td class="mono-id text-muted">
                    {d.ordering_key || '—'}
                  </td>
                  <td class="error-reason-cell">
                    <span class="error-badge">{d.error_reason}</span>
                  </td>
                  <td>
                    <Badge variant={d.resolved ? 'success' : 'danger'}>
                      {d.resolved ? 'Resolved' : 'Pending'}
                    </Badge>
                  </td>
                  <td>{d.retry_count}</td>
                  <td class="payload-cell">
                    <CodeBox content={d.payload} maxHeight="70px" />
                  </td>
                  <td>
                    <Button
                      variant="primary"
                      size="sm"
                      disabled={retryingDLQId === d.id}
                      onclick={() => handleRetryDLQ(d.id)}
                    >
                      {retryingDLQId === d.id ? 'Retrying...' : 'Retry'}
                    </Button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <EmptyState
          title="Stream dead-letter queue is clear"
          description="No poisoned or malformed streaming records detected. All ingested events passed validation and workflow execution."
          icon="event"
        />
      {/if}
    </Card>
  {/if}
</div>

<!-- Register Schema Modal -->
<Dialog
  open={showRegisterModal}
  title="Register Event Schema"
  subtitle="Enforce structural typing and backward compatibility on incoming streaming payloads"
  maxWidth="640px"
  onclose={() => (showRegisterModal = false)}
>
  {#snippet children()}
    <div class="modal-form">
      <Input
        label="Schema Name / Subject"
        placeholder="e.g. user.signup or order.completed"
        bind:value={newSchemaName}
        required
      />

      <div class="form-row">
        <Select
          label="Schema Type"
          bind:value={newSchemaType}
          options={[
            { value: 'json_schema', label: 'JSON Schema' },
            { value: 'protobuf', label: 'Protobuf (proto3)' }
          ]}
        />
        <Select
          label="Compatibility Mode"
          bind:value={newSchemaCompat}
          options={[
            { value: 'BACKWARD', label: 'BACKWARD (Consumers read new payloads)' },
            { value: 'FORWARD', label: 'FORWARD (Old consumers read new payloads)' },
            { value: 'FULL', label: 'FULL (Backward & Forward)' },
            { value: 'NONE', label: 'NONE (No evolution checks)' }
          ]}
        />
      </div>

      <Input
        label="Description (optional)"
        placeholder="Brief description of this schema's purpose"
        bind:value={newSchemaDesc}
      />

      <div class="definition-field">
        <label for="schema-def-input" class="field-label">Schema Definition <span class="required-star">*</span></label>
        <textarea
          id="schema-def-input"
          class="tech-textarea font-mono"
          bind:value={newSchemaDef}
          rows="8"
          placeholder="Enter JSON Schema or Protobuf IDL"
        ></textarea>
      </div>
    </div>
  {/snippet}

  {#snippet footer()}
    <div class="modal-actions">
      <Button variant="secondary" onclick={() => (showRegisterModal = false)}>Cancel</Button>
      <Button
        variant="primary"
        disabled={registeringSchema || !newSchemaName.trim() || !newSchemaDef.trim()}
        onclick={handleRegisterSchema}
      >
        {registeringSchema ? 'Registering...' : 'Register Schema'}
      </Button>
    </div>
  {/snippet}
</Dialog>

<!-- Replay Stream Modal -->
<Dialog
  open={showReplayModal}
  title="Historical Stream Replay"
  subtitle="Replay streaming events across partitions into workflows with optional idempotency bypass"
  maxWidth="540px"
  onclose={() => (showReplayModal = false)}
>
  {#snippet children()}
    <div class="modal-form">
      <Input
        label="Topic (optional)"
        placeholder="e.g. kerob.events (leave blank for default)"
        bind:value={replayTopic}
      />
      <div class="form-row">
        <Input
          label="Event Type (optional)"
          placeholder="e.g. payment.processed"
          bind:value={replayEventType}
        />
        <Input
          label="Source ID (optional)"
          placeholder="e.g. stripe_webhook"
          bind:value={replaySourceId}
        />
      </div>

      <Select
        label="Time Window (Since)"
        bind:value={replaySinceMinutes}
        options={[
          { value: '15', label: 'Last 15 minutes' },
          { value: '60', label: 'Last 1 hour' },
          { value: '360', label: 'Last 6 hours' },
          { value: '1440', label: 'Last 24 hours' },
          { value: '0', label: 'Beginning of Stream (All)' }
        ]}
      />

      <div class="checkbox-row">
        <label class="checkbox-label">
          <input type="checkbox" bind:checked={replayBypassIdempotency} />
          <span><strong>Bypass Idempotency</strong> (forces workflow execution even if event was previously executed)</span>
        </label>
      </div>
    </div>
  {/snippet}

  {#snippet footer()}
    <div class="modal-actions">
      <Button variant="secondary" onclick={() => (showReplayModal = false)}>Cancel</Button>
      <Button
        variant="primary"
        disabled={replayingStream}
        onclick={handleTriggerReplay}
      >
        {replayingStream ? 'Replaying...' : 'Execute Replay'}
      </Button>
    </div>
  {/snippet}
</Dialog>

<style>
  .events-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .tabs-nav {
    display: flex;
    gap: 8px;
    border-bottom: 1px solid var(--border-default, #272a30);
    padding-bottom: 8px;
  }

  .tab-btn {
    background: none;
    border: none;
    color: var(--text-muted, #71767b);
    font-size: 13px;
    font-weight: 500;
    padding: 6px 14px;
    cursor: pointer;
    border-radius: var(--radius-sm, 4px);
    transition: all 0.15s ease;
  }
  .tab-btn:hover {
    color: var(--text-primary, #e6edf3);
    background: var(--bg-surface-elevated, #1c2128);
  }
  .tab-btn.active {
    color: var(--text-primary, #ffffff);
    background: var(--bg-surface-elevated, #21262d);
    font-weight: 600;
  }

  .filter-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .section-desc {
    font-size: 12px;
    color: var(--text-muted, #71767b);
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

  .details-row td {
    background: var(--bg-surface-elevated, #1c2128);
    padding: 12px 16px;
  }

  .schema-definition-box {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .def-header {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .timestamp-cell {
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .payload-cell {
    min-width: 260px;
  }

  .error-reason-cell {
    max-width: 240px;
  }

  .error-badge {
    color: var(--status-error, #f85149);
    font-family: var(--font-mono);
    font-size: 11px;
    word-break: break-all;
  }

  .mono-id {
    font-size: 11px;
    font-family: var(--font-mono);
  }

  .text-muted {
    color: var(--text-muted);
  }

  .modal-form {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .form-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  .definition-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .field-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary, #8b949e);
  }

  .required-star {
    color: var(--status-error, #f85149);
  }

  .tech-textarea {
    width: 100%;
    padding: 8px 10px;
    background: var(--bg-surface-elevated, #161b22);
    border: 1px solid var(--border-default, #30363d);
    border-radius: var(--radius-sm, 4px);
    color: var(--text-primary, #c9d1d9);
    font-size: 12px;
    line-height: 1.4;
    resize: vertical;
  }

  .tech-textarea:focus {
    outline: none;
    border-color: var(--color-primary, #58a6ff);
  }

  .checkbox-row {
    margin-top: 4px;
  }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-secondary, #8b949e);
    cursor: pointer;
  }

  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
