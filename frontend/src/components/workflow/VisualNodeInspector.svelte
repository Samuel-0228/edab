<script lang="ts">
  import type { VisualNode } from './graphModel';
  import type { IntegrationMetadata, Credential, IntegrationJSONSchema } from '../../api/types';
  import { conditionOperators, transformOptions } from './workflowConstants';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import Badge from '../common/Badge.svelte';
  import Icon from '../icons/Icon.svelte';
  import SchemaConfigForm from './SchemaConfigForm.svelte';
  import { api } from '../../api/client';

  interface Props {
    node: VisualNode;
    integrations: IntegrationMetadata[];
    credentials: Credential[];
    onchange: (updatedData: Record<string, any>) => void;
    ondelete: (nodeId: string) => void;
    onclose: () => void;
  }

  let { node, integrations = [], credentials = [], onchange, ondelete, onclose }: Props = $props();

  let activeSchema = $state<IntegrationJSONSchema | null>(null);
  let jsonFormatError = $state<string | null>(null);

  // Trigger options derived
  let triggerOptions = $derived.by(() => {
    const list = integrations
      .filter(
        (i) => (i.triggers && i.triggers.length > 0) || i.kind === 'trigger' || i.kind === 'both'
      )
      .map((i) => ({ value: i.id, label: `${i.name} (${i.description})` }));
    if (!list.some((item) => item.value === 'telegram')) {
      list.push({ value: 'telegram', label: 'Telegram (Bot Messages)' });
    }
    if (!list.some((item) => item.value === 'webhook')) {
      list.push({ value: 'webhook', label: 'Inbound Webhook' });
    }
    if (!list.some((item) => item.value === 'schedule')) {
      list.push({ value: 'schedule', label: 'Schedule (Cron / Interval)' });
    }
    list.push({ value: 'all', label: 'Any Ingested Event' });
    return list;
  });

  // Action integrations derived
  let actionIntegrations = $derived.by(() => {
    const list = integrations
      .filter(
        (i) => (i.actions && i.actions.length > 0) || i.kind === 'action' || i.kind === 'both'
      )
      .map((i) => ({ value: i.id, label: `${i.name}` }));
    if (!list.some((item) => item.value === 'discord')) {
      list.push({ value: 'discord', label: 'Discord' });
    }
    if (!list.some((item) => item.value === 'telegram')) {
      list.push({ value: 'telegram', label: 'Telegram' });
    }
    if (!list.some((item) => item.value === 'http')) {
      list.push({ value: 'http', label: 'Generic HTTP (SSRF Safe)' });
    }
    if (!list.some((item) => item.value === 'github')) {
      list.push({ value: 'github', label: 'GitHub' });
    }
    return list;
  });

  let actionTypeOptions = $derived.by(() => {
    const integ = integrations.find((i) => i.id === node.data.integration);
    if (integ && integ.actions && integ.actions.length > 0) {
      return integ.actions.map((act) => ({
        value: act.type,
        label: `${act.name} (${act.type})`
      }));
    }
    if (node.data.integration === 'discord') {
      return [
        { value: 'send_message', label: 'send_message' },
        { value: 'send_embed', label: 'send_embed' }
      ];
    }
    if (node.data.integration === 'telegram') {
      return [
        { value: 'send_message', label: 'send_message' },
        { value: 'copy_message', label: 'copy_message' }
      ];
    }
    if (node.data.integration === 'http') {
      return [{ value: 'http_request', label: 'http_request' }];
    }
    if (node.data.integration === 'github') {
      return [{ value: 'create_issue', label: 'create_issue' }];
    }
    return [{ value: 'send_message', label: 'send_message' }];
  });

  async function loadSchema(integ: string, actionType: string) {
    if (!integ || !actionType) {
      activeSchema = null;
      return;
    }
    try {
      const resp = await api.getIntegrationSchema(integ, actionType);
      activeSchema = resp.config_schema || null;
    } catch {
      activeSchema = null;
    }
  }

  $effect(() => {
    if (node.type === 'action' && node.data.integration && node.data.action_type) {
      loadSchema(node.data.integration, node.data.action_type);
    } else {
      activeSchema = null;
    }
  });

  function updateField(key: string, val: any) {
    onchange({
      ...node.data,
      [key]: val
    });
  }

  function handleJsonConfigChange(val: string) {
    updateField('configRaw', val);
    try {
      if (val.trim()) {
        JSON.parse(val);
      }
      jsonFormatError = null;
    } catch (err: unknown) {
      jsonFormatError = err instanceof Error ? err.message : 'Invalid JSON format';
    }
  }
</script>

<div class="inspector-panel">
  <!-- Header -->
  <div class="inspector-header">
    <div class="header-info">
      <div class="node-badge-row">
        {#if node.type === 'trigger'}
          <Badge variant="stage-when">WHEN TRIGGER</Badge>
        {:else if node.type === 'condition'}
          <Badge variant="stage-if">IF CONDITION</Badge>
        {:else if node.type === 'transformation'}
          <Badge variant="stage-transform">TRANSFORM</Badge>
        {:else if node.type === 'action'}
          <Badge variant="stage-then">THEN ACTION</Badge>
        {:else if node.type === 'branch'}
          <Badge variant="warning">BRANCH</Badge>
        {:else if node.type === 'loop'}
          <Badge variant="neutral">FOR EACH</Badge>
        {:else if node.type === 'delay'}
          <Badge variant="pending">DELAY</Badge>
        {:else if node.type === 'parallel'}
          <Badge variant="running">PARALLEL</Badge>
        {/if}
        <code class="node-id-tag">{node.id}</code>
      </div>
      <h3 class="inspector-title">{node.name}</h3>
    </div>

    <div class="header-actions">
      {#if node.type !== 'trigger'}
        <button class="danger-icon-btn" title="Delete Node" onclick={() => ondelete(node.id)}>
          <Icon name="trash" size={14} />
        </button>
      {/if}
      <button class="close-icon-btn" onclick={onclose} aria-label="Close Inspector">
        <Icon name="close" size={14} />
      </button>
    </div>
  </div>

  <div class="inspector-scroll">
    <!-- TRIGGER CONFIGURATION -->
    {#if node.type === 'trigger'}
      <div class="section-group">
        <div class="section-header">EVENT SOURCE INGESTION</div>
        <Select
          label="Trigger Integration"
          value={node.data.triggerType}
          options={triggerOptions}
          onchange={(e) => {
            const val = (e.target as HTMLSelectElement).value;
            let defEvent = '*';
            if (val === 'telegram') defEvent = 'telegram.message';
            else if (val === 'github') defEvent = 'github.issue.opened';
            else if (val === 'rss') defEvent = 'rss.item';
            else if (val === 'schedule') defEvent = 'schedule.tick';
            onchange({
              ...node.data,
              triggerType: val,
              eventTypes: [defEvent]
            });
          }}
        />

        <Input
          label="Matched Event Types (comma-separated or * for all)"
          value={Array.isArray(node.data.eventTypes)
            ? node.data.eventTypes.join(', ')
            : node.data.eventTypes || '*'}
          placeholder="e.g. telegram.message, github.push"
          mono
          onchange={(e) => {
            const val = (e.target as HTMLInputElement).value;
            updateField(
              'eventTypes',
              val
                .split(',')
                .map((s: string) => s.trim())
                .filter(Boolean)
            );
          }}
        />

        {#if node.data.triggerType === 'schedule'}
          <div class="schedule-box">
            <span class="box-subtitle">Periodic Interval or Cron Expression</span>
            <div class="grid-2col">
              <Input
                label="Interval"
                value={node.data.scheduleInterval}
                placeholder="5m, 1h, 30s"
                mono
                onchange={(e) =>
                  updateField('scheduleInterval', (e.target as HTMLInputElement).value)}
              />
              <Input
                label="Cron Expression (optional)"
                value={node.data.scheduleCron}
                placeholder="*/15 * * * *"
                mono
                onchange={(e) => updateField('scheduleCron', (e.target as HTMLInputElement).value)}
              />
            </div>
          </div>
        {/if}
      </div>

      <!-- CONDITION CONFIGURATION -->
    {:else if node.type === 'condition'}
      <div class="section-group">
        <div class="section-header">RULE EXPRESSION EVALUATOR</div>
        <Input
          label="Field Path (JSONPath)"
          value={node.data.field}
          placeholder="e.g. payload.text or payload.amount"
          mono
          onchange={(e) => updateField('field', (e.target as HTMLInputElement).value)}
        />

        <Select
          label="Comparison Operator"
          value={node.data.operator}
          options={conditionOperators.filter((o) => o.value !== '')}
          onchange={(e) => updateField('operator', (e.target as HTMLSelectElement).value)}
        />

        <Input
          label="Expected Value"
          value={node.data.value}
          placeholder="e.g. critical or 100"
          mono
          onchange={(e) => updateField('value', (e.target as HTMLInputElement).value)}
        />
      </div>

      <!-- TRANSFORMATION CONFIGURATION -->
    {:else if node.type === 'transformation'}
      <div class="section-group">
        <div class="section-header">PAYLOAD FORMATTER & TEMPLATES</div>
        <Select
          label="Transformation Mode"
          value={node.data.transformType}
          options={transformOptions}
          onchange={(e) => updateField('transformType', (e.target as HTMLSelectElement).value)}
        />

        {#if node.data.transformType === 'template'}
          <div class="field-wrap">
            <label class="field-label" for="transform-template">Template String</label>
            <textarea
              id="transform-template"
              class="code-textarea"
              rows={5}
              placeholder={'e.g. Alert: [{{payload.severity | upper}}] {{payload.title}}'}
              value={node.data.template}
              oninput={(e) => updateField('template', (e.target as HTMLTextAreaElement).value)}
            ></textarea>
          </div>
          <span class="field-hint">
            Supports pipes: <code>| upper</code>, <code>| lower</code>, <code>| truncate:20</code>,
            <code>| date_format:"2006-01-02"</code>
          </span>
        {/if}
      </div>

      <!-- ACTION CONFIGURATION -->
    {:else if node.type === 'action'}
      <div class="section-group">
        <div class="section-header">DISPATCH TARGET & INTEGRATION</div>
        <div class="grid-2col">
          <Select
            label="Integration"
            value={node.data.integration}
            options={actionIntegrations}
            onchange={(e) => {
              const v = (e.target as HTMLSelectElement).value;
              updateField('integration', v);
              const opts = actionTypeOptions;
              updateField('action_type', opts[0]?.value || 'send_message');
            }}
          />

          <Select
            label="Action Type"
            value={node.data.action_type}
            options={actionTypeOptions}
            onchange={(e) => updateField('action_type', (e.target as HTMLSelectElement).value)}
          />
        </div>

        {#if credentials.length > 0}
          <Select
            label="Vault Credentials (Optional)"
            value={node.data.credential_id || ''}
            options={[
              { value: '', label: '(None / Inline Secrets)' },
              ...credentials.map((c) => ({ value: c.id, label: `${c.name} (${c.integration})` }))
            ]}
            onchange={(e) => updateField('credential_id', (e.target as HTMLSelectElement).value)}
          />
        {/if}

        {#if activeSchema}
          <div class="schema-helper">
            <SchemaConfigForm schema={activeSchema} bind:value={node.data.configRaw} />
          </div>
        {/if}

        <div class="field-wrap">
          <label class="field-label" for="action-config">Action Configuration (JSON)</label>
          <textarea
            id="action-config"
            class="code-textarea {jsonFormatError ? 'has-error' : ''}"
            rows={6}
            value={node.data.configRaw}
            oninput={(e) => handleJsonConfigChange((e.target as HTMLTextAreaElement).value)}
          ></textarea>
          {#if jsonFormatError}
            <div class="error-msg">{jsonFormatError}</div>
          {/if}
        </div>

        <div class="section-header" style="margin-top: 14px;">EXECUTION POLICIES</div>
        <div class="grid-3col">
          <Select
            label="On Error"
            value={node.data.on_error || 'fail'}
            options={[
              { value: 'fail', label: 'Fail Run' },
              { value: 'continue', label: 'Continue Run' },
              { value: 'skip_rest', label: 'Skip Rest' }
            ]}
            onchange={(e) => updateField('on_error', (e.target as HTMLSelectElement).value)}
          />

          <Input
            label="Timeout (sec)"
            type="number"
            value={String(node.data.timeout_seconds || 30)}
            onchange={(e) =>
              updateField('timeout_seconds', Number((e.target as HTMLInputElement).value) || 30)}
          />

          <Input
            label="Max Retries"
            type="number"
            value={String(node.data.max_retries ?? 3)}
            onchange={(e) =>
              updateField('max_retries', Number((e.target as HTMLInputElement).value) || 3)}
          />
        </div>
      </div>

      <!-- BRANCH CONFIGURATION -->
    {:else if node.type === 'branch'}
      <div class="section-group">
        <div class="section-header">CONDITIONAL FORK</div>
        <Input
          label="Branch Field Path"
          value={node.data.field}
          placeholder="payload.type"
          mono
          onchange={(e) => updateField('field', (e.target as HTMLInputElement).value)}
        />
        <Select
          label="Branch Operator"
          value={node.data.operator}
          options={conditionOperators.filter((o) => o.value !== '')}
          onchange={(e) => updateField('operator', (e.target as HTMLSelectElement).value)}
        />
        <Input
          label="Match Value"
          value={node.data.value}
          placeholder="critical"
          mono
          onchange={(e) => updateField('value', (e.target as HTMLInputElement).value)}
        />
        <span class="field-hint">
          Connect output <b>Matched</b> to the steps for this condition, and <b>Default</b> for fallback
          steps.
        </span>
      </div>

      <!-- LOOP CONFIGURATION -->
    {:else if node.type === 'loop'}
      <div class="section-group">
        <div class="section-header">ARRAY ITERATOR (FOR EACH)</div>
        <Input
          label="Items Array Expression"
          value={node.data.items}
          placeholder="payload.recipients"
          mono
          onchange={(e) => updateField('items', (e.target as HTMLInputElement).value)}
        />
        <div class="grid-2col">
          <Input
            label="Item Variable Name"
            value={node.data.item_var || 'item'}
            placeholder="item"
            mono
            onchange={(e) => updateField('item_var', (e.target as HTMLInputElement).value)}
          />
          <Input
            label="Max Iterations Cap"
            type="number"
            value={String(node.data.max_iterations || 50)}
            onchange={(e) =>
              updateField('max_iterations', Number((e.target as HTMLInputElement).value) || 50)}
          />
        </div>
        <span class="field-hint">
          Steps connected to <b>Each Item</b> can access <code>{'{{item.field}}'}</code> during iteration.
        </span>
      </div>

      <!-- DELAY CONFIGURATION -->
    {:else if node.type === 'delay'}
      <div class="section-group">
        <div class="section-header">DURABLE ASYNC DELAY</div>
        <Input
          label="Duration Expression"
          value={node.data.duration}
          placeholder="e.g. 10s, 5m, 1h"
          mono
          onchange={(e) => updateField('duration', (e.target as HTMLInputElement).value)}
        />
        <span class="field-hint">
          Execution pauses durable state in PostgreSQL without occupying worker threads or database
          connections.
        </span>
      </div>

      <!-- PARALLEL CONFIGURATION -->
    {:else if node.type === 'parallel'}
      <div class="section-group">
        <div class="section-header">CONCURRENT FAN-OUT</div>
        <Input
          label="Concurrency Limit"
          type="number"
          value={String(node.data.concurrency || 4)}
          onchange={(e) =>
            updateField('concurrency', Number((e.target as HTMLInputElement).value) || 4)}
        />
        <span class="field-hint">
          Executes child branches concurrently with bounded goroutine concurrency.
        </span>
      </div>
    {/if}
  </div>
</div>

<style>
  .inspector-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: var(--bg-surface-elevated, #16181d);
    border-left: 1px solid var(--border-default, #272a30);
    overflow: hidden;
  }

  .inspector-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 16px;
    background: var(--bg-surface, #121417);
    border-bottom: 1px solid var(--border-default, #272a30);
    gap: 12px;
  }

  .header-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .node-badge-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .node-id-tag {
    font-size: 10px;
    color: var(--text-muted, #71767b);
  }

  .inspector-title {
    margin: 0;
    font-size: 13.5px;
    font-weight: 600;
    color: var(--text-primary, #e6edf3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .close-icon-btn,
  .danger-icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    background: transparent;
    border: 1px solid var(--border-subtle, #272a30);
    border-radius: var(--radius-sm, 4px);
    color: var(--text-secondary, #9aa0a6);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .close-icon-btn:hover {
    background: var(--bg-surface-hover, #202328);
    color: var(--text-primary, #fff);
  }

  .danger-icon-btn:hover {
    background: rgba(239, 68, 68, 0.15);
    border-color: #ef4444;
    color: #ef4444;
  }

  .inspector-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .section-group {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .section-header {
    font-family: var(--font-mono, monospace);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.08em;
    color: var(--text-muted, #8b949e);
    border-bottom: 1px solid var(--border-subtle, #272a30);
    padding-bottom: 4px;
  }

  .grid-2col {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }

  .grid-3col {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 8px;
  }

  .schedule-box {
    background: rgba(0, 0, 0, 0.2);
    border: 1px solid var(--border-subtle, #272a30);
    border-radius: 6px;
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .box-subtitle {
    font-size: 11px;
    color: var(--text-muted, #8b949e);
  }

  .field-wrap {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .field-label {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-secondary, #9aa0a6);
  }

  .field-hint {
    font-size: 11px;
    color: var(--text-muted, #71767b);
    line-height: 1.4;
  }

  .field-hint code,
  .field-hint b {
    color: var(--accent-amber, #f59e0b);
  }

  .code-textarea {
    width: 100%;
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    background: var(--bg-surface, #0d1117);
    color: var(--text-primary, #e6edf3);
    border: 1px solid var(--border-default, #30363d);
    border-radius: 4px;
    padding: 8px;
    resize: vertical;
    box-sizing: border-box;
  }

  .code-textarea:focus {
    outline: none;
    border-color: var(--accent-blue, #58a6ff);
  }

  .code-textarea.has-error {
    border-color: #ef4444;
  }

  .error-msg {
    font-size: 11px;
    color: #f87171;
  }

  .schema-helper {
    background: rgba(88, 166, 255, 0.04);
    border: 1px solid rgba(88, 166, 255, 0.15);
    border-radius: 6px;
    padding: 10px;
  }
</style>
