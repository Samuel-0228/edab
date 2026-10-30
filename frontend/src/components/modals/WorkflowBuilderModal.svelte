<script lang="ts">
  import Dialog from '../common/Dialog.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import Button from '../common/Button.svelte';
  import Badge from '../common/Badge.svelte';
  import WorkflowTriggerStage from '../workflow/WorkflowTriggerStage.svelte';
  import WorkflowConditionStage from '../workflow/WorkflowConditionStage.svelte';
  import WorkflowTransformStage from '../workflow/WorkflowTransformStage.svelte';
  import WorkflowActionStage from '../workflow/WorkflowActionStage.svelte';
  import {
    fallbackTriggers,
    fallbackActions,
    conditionOperators,
    transformOptions
  } from '../workflow/workflowConstants';
  import { workflowBuilderModal, closeWorkflowBuilder } from '../../api/stores';
  import { api } from '../../api/client';
  import { notifySuccess, notifyError } from '../../api/stores';
  import { triggerRefresh } from '../../api/stores';
  import type {
    Credential,
    IntegrationMetadata,
    IntegrationJSONSchema,
    WorkflowCreateInput
  } from '../../api/types';

  // Form State
  let name = $state('');
  let description = $state('');
  let enabled = $state('true');
  let maxRetries = $state(3);

  // Trigger
  let triggerType = $state('telegram');
  let triggerEventType = $state('*');
  let scheduleInterval = $state('5m');
  let scheduleCron = $state('');

  // Conditions
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
  let actionConfigRaw = $state('{}');
  let actionSchema = $state<IntegrationJSONSchema | null>(null);

  let credentialsList = $state<Credential[]>([]);
  let integrationsCatalog = $state<IntegrationMetadata[]>([]);
  let loading = $state(false);

  let triggerOptions = $derived.by(() => {
    if (integrationsCatalog.length === 0) return fallbackTriggers;
    const items = integrationsCatalog
      .filter((i) => i.triggers && i.triggers.length > 0)
      .map((i) => ({ value: i.id, label: `${i.name} (${i.description})` }));
    items.push({ value: 'all', label: 'Any Ingested Event' });
    return items;
  });

  let actionIntegrations = $derived.by(() => {
    if (integrationsCatalog.length === 0) return fallbackActions;
    const items = integrationsCatalog
      .filter((i) => (i.actions && i.actions.length > 0) || i.kind === 'action')
      .map((i) => ({ value: i.id, label: `${i.name} — ${i.description}` }));
    return items.length > 0 ? items : fallbackActions;
  });

  let actionTypeOptions = $derived.by(() => {
    const found = integrationsCatalog.find((i) => i.id === actionIntegration);
    if (found && found.actions && found.actions.length > 0) {
      return found.actions.map((act) => ({
        value: act.type,
        label: `${act.name} (${act.type})`
      }));
    }
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

  async function loadIntegrationSchema(integId: string, opType: string) {
    try {
      const resp = await api.getIntegrationSchema(integId, opType);
      actionSchema = resp.config_schema || null;
    } catch {
      actionSchema = null;
    }
  }

  $effect(() => {
    if ($workflowBuilderModal.open) {
      loadCatalogAndCredentials();
      const wf = $workflowBuilderModal.workflow;
      if (wf) {
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
          const cfg = wf.trigger.config;
          scheduleInterval = typeof cfg.interval === 'string' ? cfg.interval : '5m';
          scheduleCron = typeof cfg.cron === 'string' ? cfg.cron : '';
        }

        if (wf.conditions) {
          conditionField = wf.conditions.field || '';
          conditionOp = wf.conditions.operator || '';
          conditionVal = wf.conditions.value !== undefined ? String(wf.conditions.value) : '';
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
          onActionIntegrationChange();
        }
      } else {
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
        onActionIntegrationChange();
      }
    }
  });

  async function loadCatalogAndCredentials() {
    try {
      const [cats, creds] = await Promise.all([api.listIntegrations(), api.listCredentials()]);
      integrationsCatalog = cats;
      credentialsList = creds;
    } catch {
      // Keep fallbacks
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

  function onActionIntegrationChange() {
    const opts = actionTypeOptions;
    actionType = opts[0]?.value || 'send_message';
    onActionTypeChange();
  }

  function onActionTypeChange() {
    loadIntegrationSchema(actionIntegration, actionType);
    if (!actionConfigRaw || actionConfigRaw === '{}') {
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
      } catch {
        notifyError('Action Configuration is invalid JSON');
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
        event_types: triggerEventType
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
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
  title={$workflowBuilderModal.mode === 'edit'
    ? 'Edit Automation Pipeline'
    : 'Create Automation Pipeline'}
  subtitle="Build a declarative event processing rule: WHEN (Trigger) → IF (Condition) → TRANSFORM (Format) → THEN (Action)."
  maxWidth="780px"
  onclose={closeWorkflowBuilder}
>
  <div class="builder-container">
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

    <WorkflowTriggerStage
      bind:triggerType
      bind:triggerEventType
      bind:scheduleInterval
      bind:scheduleCron
      options={triggerOptions}
      onchange={updateTriggerDefaults}
    />

    <WorkflowConditionStage
      bind:conditionField
      bind:conditionOp
      bind:conditionVal
      operators={conditionOperators}
    />

    <WorkflowTransformStage
      bind:transformType
      bind:transformTemplate
      options={transformOptions}
    />

    <WorkflowActionStage
      bind:actionIntegration
      bind:actionType
      bind:actionCredentialId
      bind:actionConfigRaw
      actionSchema={actionSchema}
      bind:maxRetries
      integrations={actionIntegrations}
      actionTypes={actionTypeOptions}
      credentials={credentialsList}
      onIntegrationChange={onActionIntegrationChange}
      onTypeChange={onActionTypeChange}
    />
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
    padding: 4px 0;
  }

  .lifecycle-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: var(--card-bg-subtle, rgba(255, 255, 255, 0.02));
    border: 1px solid var(--border-color, #27272a);
    border-radius: 8px;
    padding: 8px 16px;
  }

  .stage-step {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .stage-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary, #a1a1aa);
  }

  .stage-arrow {
    font-size: 14px;
    color: var(--text-tertiary, #52525b);
  }

  .general-grid {
    display: grid;
    grid-template-columns: 2fr 1fr;
    gap: 12px;
  }
</style>
