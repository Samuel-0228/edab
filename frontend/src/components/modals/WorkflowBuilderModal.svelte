<script lang="ts">
  import {
    workflowBuilderModal,
    closeWorkflowBuilder,
    triggerRefresh,
    notifySuccess,
    notifyError
  } from '../../api/stores';
  import { api } from '../../api/client';
  import type { Credential, WorkflowCreateInput } from '../../api/types';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import JsonEditor from '../common/JsonEditor.svelte';
  import Badge from '../common/Badge.svelte';

  let name = $state('');
  let description = $state('');
  let enabled = $state('true');
  let maxRetries = $state(3);

  // Trigger
  let triggerType = $state('telegram');
  let triggerEventType = $state('*');
  let scheduleInterval = $state('5m');
  let scheduleCron = $state('');

  // Condition
  let conditionField = $state('');
  let conditionOp = $state('');
  let conditionVal = $state('');

  // Transform
  let transformType = $state<'passthrough' | 'template' | 'mapping'>('passthrough');
  let transformTemplate = $state('');

  // Action
  let actionIntegration = $state('discord');
  let actionType = $state('send_message');
  let actionCredentialId = $state('');
  let actionConfigRaw = $state('');

  let credentialsList = $state<Credential[]>([]);
  let loading = $state(false);

  const triggerOptions = [
    { value: 'telegram', label: 'Telegram (Bot Messages & Channel Posts)' },
    { value: 'github', label: 'GitHub (Issues, Pull Requests, Push)' },
    { value: 'webhook', label: 'Inbound Webhook' },
    { value: 'rss', label: 'RSS / Atom Feed Poller' },
    { value: 'discord', label: 'Discord Interactions' },
    { value: 'schedule', label: 'Schedule (Periodic Cron / Interval)' },
    { value: 'all', label: 'Any Ingested Event' }
  ];

  const conditionOperators = [
    { value: '', label: '(No Condition — Passthrough Always True)' },
    { value: 'contains', label: 'contains' },
    { value: 'equals', label: 'equals' },
    { value: 'not_equals', label: 'not equals' },
    { value: 'not_contains', label: 'not contains' },
    { value: 'starts_with', label: 'starts with' },
    { value: 'ends_with', label: 'ends with' },
    { value: 'matches', label: 'regex matches' },
    { value: 'exists', label: 'field exists' },
    { value: 'greater_than', label: '> greater than' },
    { value: 'less_than', label: '< less than' }
  ];

  const transformOptions = [
    { value: 'passthrough', label: 'Passthrough (Preserve Raw Event)' },
    { value: 'template', label: 'Template String' },
    { value: 'mapping', label: 'Field Map Dictionary' }
  ];

  const actionIntegrations = [
    { value: 'discord', label: 'Discord (Channel Webhook / Embed)' },
    { value: 'telegram', label: 'Telegram (Send / Copy Message)' },
    { value: 'http', label: 'Generic REST HTTP API (SSRF Safe)' },
    { value: 'github', label: 'GitHub API (Create Issue / Comment)' }
  ];

  let actionTypeOptions = $derived.by(() => {
    if (actionIntegration === 'discord') {
      return [
        { value: 'send_message', label: 'send_message' },
        { value: 'send_embed', label: 'send_embed' }
      ];
    } else if (actionIntegration === 'telegram') {
      return [
        { value: 'send_message', label: 'send_message' },
        { value: 'copy_message', label: 'copy_message' }
      ];
    } else if (actionIntegration === 'http') {
      return [{ value: 'http_request', label: 'http_request' }];
    } else if (actionIntegration === 'github') {
      return [{ value: 'create_issue', label: 'create_issue' }];
    }
    return [{ value: 'send_message', label: 'send_message' }];
  });

  $effect(() => {
    if ($workflowBuilderModal.open) {
      loadCredentials();
      const wf = $workflowBuilderModal.workflow;
      if (wf) {
        // Edit mode
        name = wf.name || '';
        description = wf.description || '';
        enabled = wf.enabled ? 'true' : 'false';
        maxRetries = wf.max_retries || 3;

        triggerType = wf.trigger.type || 'telegram';
        triggerEventType =
          wf.trigger.event_types && wf.trigger.event_types.length > 0
            ? wf.trigger.event_types.join(',')
            : '*';

        if (wf.trigger.type === 'schedule' && wf.trigger.config) {
          const cfg = wf.trigger.config as any;
          scheduleInterval = cfg.interval || '5m';
          scheduleCron = cfg.cron || '';
        }

        if (wf.conditions) {
          conditionField = wf.conditions.field || '';
          conditionOp = wf.conditions.operator || '';
          conditionVal =
            wf.conditions.value !== undefined ? String(wf.conditions.value) : '';
        } else {
          conditionField = '';
          conditionOp = '';
          conditionVal = '';
        }

        if (wf.transformations) {
          transformType = wf.transformations.type || 'passthrough';
          transformTemplate = wf.transformations.template || '';
        } else {
          transformType = 'passthrough';
          transformTemplate = '';
        }

        if (wf.actions && wf.actions.length > 0) {
          const act = wf.actions[0];
          actionIntegration = act.integration;
          actionType = act.action_type;
          actionCredentialId = act.credential_id || '';
          actionConfigRaw = JSON.stringify(act.config || {}, null, 2);
        } else {
          setDefaultActionConfig();
        }
      } else {
        // Create mode
        name = '';
        description = '';
        enabled = 'true';
        maxRetries = 3;
        conditionField = '';
        conditionOp = '';
        conditionVal = '';
        transformType = 'passthrough';
        transformTemplate = '';
        actionCredentialId = '';

        if ($workflowBuilderModal.preselectedIntegration) {
          const pre = $workflowBuilderModal.preselectedIntegration;
          if (pre === 'schedule') {
            triggerType = 'schedule';
            triggerEventType = 'schedule.tick';
          } else if (pre === 'github') {
            triggerType = 'github';
            triggerEventType = 'github.issue.opened';
            actionIntegration = 'github';
          } else if (pre === 'telegram') {
            triggerType = 'telegram';
            triggerEventType = 'telegram.message';
            actionIntegration = 'telegram';
          } else if (pre === 'discord') {
            actionIntegration = 'discord';
          } else if (pre === 'http') {
            actionIntegration = 'http';
          }
        } else {
          triggerType = 'telegram';
          triggerEventType = '*';
          actionIntegration = 'discord';
        }
        updateTriggerDefaults();
        setDefaultActionConfig();
      }
    }
  });

  async function loadCredentials() {
    try {
      credentialsList = await api.listCredentials();
    } catch {
      credentialsList = [];
    }
  }

  function updateTriggerDefaults() {
    if (triggerType === 'telegram') triggerEventType = 'telegram.message';
    else if (triggerType === 'github') triggerEventType = 'github.issue.opened';
    else if (triggerType === 'rss') triggerEventType = 'rss.item';
    else if (triggerType === 'webhook') triggerEventType = 'webhook.received';
    else if (triggerType === 'schedule') triggerEventType = 'schedule.tick';
    else triggerEventType = '*';
  }

  function setDefaultActionConfig() {
    actionType = actionTypeOptions[0]?.value || 'send_message';
    if (actionIntegration === 'discord') {
      actionConfigRaw = `{\n  "webhook_url": "https://discord.com/api/webhooks/...",\n  "content": "{{payload.text}}"\n}`;
    } else if (actionIntegration === 'telegram') {
      actionConfigRaw = `{\n  "chat_id": "-100123456789",\n  "text": "{{payload.text}}"\n}`;
    } else if (actionIntegration === 'http') {
      actionConfigRaw = `{\n  "url": "https://api.external.com/v1/notify",\n  "method": "POST",\n  "headers": {\n    "Content-Type": "application/json"\n  },\n  "body": "{\\"event\\": \\"{{event.id}}\\"}"\n}`;
    } else if (actionIntegration === 'github') {
      actionConfigRaw = `{\n  "owner": "organization",\n  "repo": "repository",\n  "title": "{{payload.title}}",\n  "body": "{{payload.text}}"\n}`;
    }
  }

  async function handleSave() {
    if (!name.trim()) {
      notifyError('Automation name is required');
      return;
    }

    let parsedConfig: Record<string, unknown> = {};
    if (actionConfigRaw.trim()) {
      try {
        parsedConfig = JSON.parse(actionConfigRaw);
      } catch (e: unknown) {
        notifyError('Action Configuration JSON is invalid');
        return;
      }
    }

    let triggerConfig: Record<string, unknown> | undefined = undefined;
    if (triggerType === 'schedule') {
      triggerConfig = {};
      if (scheduleCron.trim()) {
        triggerConfig.cron = scheduleCron.trim();
      } else {
        triggerConfig.interval = scheduleInterval.trim() || '5m';
      }
    }

    let conditions = null;
    if (conditionOp && conditionField.trim()) {
      conditions = {
        field: conditionField.trim(),
        operator: conditionOp,
        value: conditionVal.trim()
      };
    }

    let transformations = null;
    if (transformType !== 'passthrough') {
      transformations = {
        type: transformType,
        template: transformTemplate.trim()
      };
    }

    const payload: WorkflowCreateInput = {
      name: name.trim(),
      description: description.trim() || undefined,
      enabled: enabled === 'true',
      max_retries: Number(maxRetries) || 3,
      trigger: {
        type: triggerType,
        event_types: triggerEventType.split(',').map((s) => s.trim()).filter(Boolean),
        config: triggerConfig
      },
      conditions,
      transformations,
      actions: [
        {
          integration: actionIntegration,
          action_type: actionType,
          credential_id: actionCredentialId.trim() || undefined,
          config: parsedConfig
        }
      ]
    };

    loading = true;
    try {
      if ($workflowBuilderModal.mode === 'edit' && $workflowBuilderModal.workflow?.id) {
        await api.updateWorkflow($workflowBuilderModal.workflow.id, payload);
        notifySuccess(`Automation "${payload.name}" updated`);
      } else {
        await api.createWorkflow(payload);
        notifySuccess(`Automation "${payload.name}" created`);
      }
      triggerRefresh();
      closeWorkflowBuilder();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to save automation';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }
</script>

<Dialog
  open={$workflowBuilderModal.open}
  title={$workflowBuilderModal.mode === 'edit' ? 'Edit Automation Pipeline' : 'Create Automation Pipeline'}
  subtitle="Build a declarative event processing rule: WHEN (Trigger) → IF (Condition) → TRANSFORM (Format) → THEN (Action)."
  maxWidth="780px"
  onclose={closeWorkflowBuilder}
>
  <div class="builder-container">
    <!-- Lifecycle Stage Indicator Bar -->
    <div class="lifecycle-bar">
      <div class="stage-step">
        <Badge variant="stage-when">WHEN</Badge>
        <span class="stage-label">Trigger</span>
      </div>
      <span class="stage-arrow">→</span>
      <div class="stage-step">
        <Badge variant="stage-if">IF</Badge>
        <span class="stage-label">Condition</span>
      </div>
      <span class="stage-arrow">→</span>
      <div class="stage-step">
        <Badge variant="stage-transform">TRANSFORM</Badge>
        <span class="stage-label">Format</span>
      </div>
      <span class="stage-arrow">→</span>
      <div class="stage-step">
        <Badge variant="stage-then">THEN</Badge>
        <span class="stage-label">Action</span>
      </div>
    </div>

    <!-- General Settings -->
    <div class="general-grid">
      <Input
        label="Automation Name"
        placeholder="e.g. Sync Telegram Alerts to Discord"
        bind:value={name}
        required
      />

      <Select
        label="Operational Status"
        bind:value={enabled}
        options={[
          { value: 'true', label: 'Active (Enabled)' },
          { value: 'false', label: 'Inactive (Disabled)' }
        ]}
      />
    </div>

    <Input
      label="Description"
      placeholder="Filter incoming messages, transform payload structure, and dispatch to destination."
      bind:value={description}
    />

    <!-- STAGE 1: WHEN TRIGGER -->
    <div class="stage-card">
      <div class="stage-header">
        <Badge variant="stage-when">WHEN</Badge>
        <span class="stage-title">Trigger Event Ingestion</span>
      </div>

      <div class="stage-fields-grid">
        <Select
          label="Service Adapter"
          options={triggerOptions}
          bind:value={triggerType}
          onchange={updateTriggerDefaults}
        />

        <Input
          label="Event Type Pattern"
          placeholder="e.g. * or telegram.message"
          bind:value={triggerEventType}
          mono
          hint="Use * to match all events from this trigger adapter."
        />
      </div>

      {#if triggerType === 'schedule'}
        <div class="schedule-config-box">
          <Input
            label="Tick Interval (e.g. 1m, 5m, 1h, 24h)"
            placeholder="5m"
            bind:value={scheduleInterval}
            mono
          />
          <Input
            label="Or Standard Cron Expression (optional)"
            placeholder="*/5 * * * *"
            bind:value={scheduleCron}
            mono
            hint="5-field cron expression evaluated by the engine background scheduler."
          />
        </div>
      {/if}
    </div>

    <!-- STAGE 2: IF CONDITIONS -->
    <div class="stage-card">
      <div class="stage-header">
        <Badge variant="stage-if">IF</Badge>
        <span class="stage-title">Evaluation Conditions (Optional)</span>
      </div>

      <div class="condition-fields-grid">
        <Input
          label="Payload Field Path"
          placeholder="e.g. payload.text or payload.title"
          bind:value={conditionField}
          mono
        />

        <Select
          label="Operator"
          options={conditionOperators}
          bind:value={conditionOp}
        />

        <Input
          label="Target Value"
          placeholder="e.g. urgent or true"
          bind:value={conditionVal}
          mono
        />
      </div>
    </div>

    <!-- STAGE 3: TRANSFORM -->
    <div class="stage-card">
      <div class="stage-header">
        <Badge variant="stage-transform">TRANSFORM</Badge>
        <span class="stage-title">Deterministic Data Formatting (Optional)</span>
      </div>

      <div class="transform-fields-grid">
        <Select
          label="Transform Mode"
          options={transformOptions}
          bind:value={transformType}
        />

        <Input
          label="Template Expression / Field Map"
          placeholder="[event.source_id] payload.title — payload.author"
          bind:value={transformTemplate}
          mono
          hint="Interpolate fields into the output payload before dispatching."
        />
      </div>
    </div>

    <!-- STAGE 4: THEN ACTION -->
    <div class="stage-card">
      <div class="stage-header">
        <Badge variant="stage-then">THEN</Badge>
        <span class="stage-title">Action Execution</span>
      </div>

      <div class="stage-fields-grid">
        <Select
          label="Integration Adapter"
          options={actionIntegrations}
          bind:value={actionIntegration}
          onchange={setDefaultActionConfig}
        />

        <Select
          label="Action Operation"
          options={actionTypeOptions}
          bind:value={actionType}
        />
      </div>

      <JsonEditor
        label="Action Configuration (JSON Parameters)"
        bind:value={actionConfigRaw}
        minHeight="110px"
        hint="Template placeholders like &#123;&#123;payload.text&#125;&#125; are resolved during workflow execution."
        required
      />

      <div class="stage-fields-grid" style="margin-top: 4px;">
        <Select
          label="Encrypted Credential (Optional)"
          bind:value={actionCredentialId}
          options={[
            { value: '', label: '(None / Inline Credentials)' },
            ...credentialsList.map((c) => ({
              value: c.id,
              label: `${c.name} (${c.integration})`
            }))
          ]}
        />

        <Input
          type="number"
          label="Max Retries"
          bind:value={maxRetries}
          mono
          hint="Exponential backoff retries before execution lands in Dead-Letter Queue."
        />
      </div>
    </div>
  </div>

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeWorkflowBuilder} disabled={loading}>
      Cancel
    </Button>
    <Button variant="primary" size="sm" {loading} onclick={handleSave}>
      {$workflowBuilderModal.mode === 'edit' ? 'Update Automation' : 'Save Automation'}
    </Button>
  {/snippet}
</Dialog>

<style>
  .builder-container {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .lifecycle-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
  }

  .stage-step {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .stage-label {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .stage-arrow {
    color: var(--text-faint);
    font-family: var(--font-mono);
  }

  .general-grid {
    display: grid;
    grid-template-columns: 2fr 1fr;
    gap: 12px;
  }

  .stage-card {
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .stage-header {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .stage-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .stage-fields-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  .schedule-config-box {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    background: var(--bg-canvas);
    padding: 10px;
    border-radius: var(--radius-sm);
    border: 1px dashed var(--border-strong);
  }

  .condition-fields-grid {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 10px;
  }

  .transform-fields-grid {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: 10px;
  }
</style>
