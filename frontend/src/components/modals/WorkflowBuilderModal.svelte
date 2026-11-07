<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../common/Dialog.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import Button from '../common/Button.svelte';
  import Icon from '../icons/Icon.svelte';
  import VisualCanvas from '../workflow/VisualCanvas.svelte';
  import VisualNodeInspector from '../workflow/VisualNodeInspector.svelte';
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
  import {
    type WorkflowDraft,
    type VisualEdge,
    type NodeType,
    workflowToGraph,
    graphToWorkflow,
    createNode,
    validateWorkflowDraft,
    exportWorkflowJson,
    importWorkflowJson
  } from '../workflow/graphModel';
  import { workflowBuilderModal, closeWorkflowBuilder } from '../../api/stores';
  import { api } from '../../api/client';
  import { notifySuccess, notifyError, notifyWarning } from '../../api/stores';
  import { triggerRefresh } from '../../api/stores';
  import type {
    Credential,
    IntegrationMetadata,
    IntegrationJSONSchema,
    WorkflowCreateInput
  } from '../../api/types';

  // General Metadata
  let name = $state('');
  let description = $state('');
  let enabled = $state('true');
  let maxRetries = $state(3);

  // Editor View Mode: Visual Graph vs Linear Form
  let editorMode = $state<'visual' | 'linear'>('visual');

  // Graph State
  let graphDraft = $state<WorkflowDraft>(workflowToGraph(null));
  let selectedNodeId = $state<string | null>(null);

  // Undo / Redo History Stack
  let historyStack = $state<string[]>([]);
  let historyPointer = $state<number>(-1);

  // Linear Form Fallback State
  let triggerType = $state('telegram');
  let triggerEventType = $state('*');
  let scheduleInterval = $state('5m');
  let scheduleCron = $state('');
  let conditionField = $state('');
  let conditionOp = $state('');
  let conditionVal = $state('');
  let transformType = $state<'passthrough' | 'template' | 'mapping'>('passthrough');
  let transformTemplate = $state('');
  let actionIntegration = $state('discord');
  let actionType = $state('send_message');
  let actionCredentialId = $state('');
  let actionConfigRaw = $state('{}');
  let actionSchema = $state<IntegrationJSONSchema | null>(null);

  let credentialsList = $state<Credential[]>([]);
  let integrationsCatalog = $state<IntegrationMetadata[]>([]);
  let loading = $state(false);
  let showImportModal = $state(false);
  let importJsonText = $state('');

  // Snapshot manager for Undo/Redo
  function pushHistorySnapshot() {
    const serialized = JSON.stringify({
      name,
      description,
      enabled,
      maxRetries,
      draft: graphDraft
    });
    // Truncate forward history if we made an edit after undo
    if (historyPointer < historyStack.length - 1) {
      historyStack = historyStack.slice(0, historyPointer + 1);
    }
    historyStack.push(serialized);
    if (historyStack.length > 50) historyStack.shift();
    historyPointer = historyStack.length - 1;
  }

  function handleUndo() {
    if (historyPointer > 0) {
      historyPointer--;
      const snapshot = JSON.parse(historyStack[historyPointer]);
      name = snapshot.name;
      description = snapshot.description;
      enabled = snapshot.enabled;
      maxRetries = snapshot.maxRetries;
      graphDraft = snapshot.draft;
      syncGraphToLinearForm();
    }
  }

  function handleRedo() {
    if (historyPointer < historyStack.length - 1) {
      historyPointer++;
      const snapshot = JSON.parse(historyStack[historyPointer]);
      name = snapshot.name;
      description = snapshot.description;
      enabled = snapshot.enabled;
      maxRetries = snapshot.maxRetries;
      graphDraft = snapshot.draft;
      syncGraphToLinearForm();
    }
  }

  // Selected node derived
  let selectedNode = $derived(graphDraft.nodes.find((n) => n.id === selectedNodeId) || null);

  // Live validation
  let validationResult = $derived(
    validateWorkflowDraft({
      name,
      description,
      enabled: enabled === 'true',
      max_retries: Number(maxRetries) || 3,
      nodes: graphDraft.nodes,
      edges: graphDraft.edges
    })
  );

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

  // Synchronize Graph draft into Linear state for fallback tabs
  function syncGraphToLinearForm() {
    const triggerNode = graphDraft.nodes.find((n) => n.type === 'trigger');
    if (triggerNode) {
      triggerType = triggerNode.data.triggerType || 'telegram';
      triggerEventType = Array.isArray(triggerNode.data.eventTypes)
        ? triggerNode.data.eventTypes.join(', ')
        : triggerNode.data.eventTypes || '*';
      scheduleInterval = triggerNode.data.scheduleInterval || '5m';
      scheduleCron = triggerNode.data.scheduleCron || '';
    }

    const conditionNode = graphDraft.nodes.find((n) => n.type === 'condition');
    if (conditionNode) {
      conditionField = conditionNode.data.field || '';
      conditionOp = conditionNode.data.operator || '';
      conditionVal = conditionNode.data.value !== undefined ? String(conditionNode.data.value) : '';
    }

    const transformNode = graphDraft.nodes.find((n) => n.type === 'transformation');
    if (transformNode) {
      transformType = transformNode.data.transformType || 'passthrough';
      transformTemplate = transformNode.data.template || '';
    }

    const actionNode = graphDraft.nodes.find((n) => n.type === 'action');
    if (actionNode) {
      actionIntegration = actionNode.data.integration || 'discord';
      actionType = actionNode.data.action_type || 'send_message';
      actionCredentialId = actionNode.data.credential_id || '';
      actionConfigRaw = actionNode.data.configRaw || '{}';
    }
  }

  // Synchronize Linear form into Graph Draft
  function syncLinearFormToGraph() {
    const triggerNode = graphDraft.nodes.find((n) => n.type === 'trigger');
    if (triggerNode) {
      triggerNode.data = {
        ...triggerNode.data,
        triggerType,
        eventTypes: triggerEventType
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
        scheduleInterval,
        scheduleCron
      };
    }

    let conditionNode = graphDraft.nodes.find((n) => n.type === 'condition');
    if (conditionOp && conditionField.trim()) {
      if (!conditionNode) {
        conditionNode = createNode('condition', 320, 160);
        graphDraft.nodes.push(conditionNode);
      }
      conditionNode.data = {
        ...conditionNode.data,
        field: conditionField.trim(),
        operator: conditionOp,
        value: conditionVal.trim()
      };
    }

    let transformNode = graphDraft.nodes.find((n) => n.type === 'transformation');
    if (transformType !== 'passthrough') {
      if (!transformNode) {
        transformNode = createNode('transformation', 610, 160);
        graphDraft.nodes.push(transformNode);
      }
      transformNode.data = {
        ...transformNode.data,
        transformType,
        template: transformTemplate.trim()
      };
    }

    let actionNode = graphDraft.nodes.find((n) => n.type === 'action');
    if (!actionNode) {
      actionNode = createNode('action', 900, 160);
      graphDraft.nodes.push(actionNode);
    }
    actionNode.data = {
      ...actionNode.data,
      integration: actionIntegration,
      action_type: actionType,
      credential_id: actionCredentialId.trim() || undefined,
      configRaw: actionConfigRaw
    };
  }

  $effect(() => {
    const modalState = $workflowBuilderModal;
    if (modalState.open) {
      untrack(() => {
        loadCatalogAndCredentials();
        const wf = modalState.workflow;
        if (wf) {
          name = wf.name || '';
          description = wf.description || '';
          enabled = wf.enabled ? 'true' : 'false';
          maxRetries = wf.max_retries || 3;
          graphDraft = workflowToGraph(wf);
          selectedNodeId = graphDraft.nodes[0]?.id || null;
          syncGraphToLinearForm();
        } else {
          name = '';
          description = '';
          enabled = 'true';
          maxRetries = 3;
          const pre = modalState.preselectedIntegration || '';
          graphDraft = workflowToGraph({
            name: '',
            trigger: {
              type: pre === 'schedule' ? 'schedule' : pre === 'github' ? 'github' : 'telegram'
            },
            actions: [
              {
                integration: pre === 'discord' ? 'discord' : 'discord',
                action_type: 'send_message',
                config: {}
              }
            ]
          });
          selectedNodeId = graphDraft.nodes[0]?.id || null;
          syncGraphToLinearForm();
        }
        historyStack = [];
        historyPointer = -1;
        pushHistorySnapshot();
      });
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
    syncLinearFormToGraph();
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
    syncLinearFormToGraph();
  }

  // --- Graph Node & Edge Manipulations ---
  function handleAddNode(type: NodeType) {
    // Find rightmost node to append next column starting from left
    let maxX = 30;
    for (const n of graphDraft.nodes) {
      if (n.x > maxX) maxX = n.x;
    }
    const newNode = createNode(type, maxX + 280, 160);
    graphDraft.nodes = [...graphDraft.nodes, newNode];

    // Connect from currently selected node if selected and has output
    if (selectedNode && selectedNode.outputs.length > 0 && newNode.inputs.length > 0) {
      const fromPort = selectedNode.outputs[0].id;
      const toPort = newNode.inputs[0].id;
      graphDraft.edges = [
        ...graphDraft.edges,
        {
          id: `edge_${selectedNode.id}_${fromPort}_to_${newNode.id}_${toPort}`,
          fromNode: selectedNode.id,
          fromPort,
          toNode: newNode.id,
          toPort
        }
      ];
    }
    selectedNodeId = newNode.id;
    pushHistorySnapshot();
    notifySuccess(`Added ${newNode.name}`);
  }

  function handleDeleteNode(nodeId: string) {
    const node = graphDraft.nodes.find((n) => n.id === nodeId);
    if (!node) return;
    if (node.type === 'trigger') {
      notifyWarning('Root Trigger node cannot be removed.');
      return;
    }
    graphDraft.nodes = graphDraft.nodes.filter((n) => n.id !== nodeId);
    graphDraft.edges = graphDraft.edges.filter((e) => e.fromNode !== nodeId && e.toNode !== nodeId);
    if (selectedNodeId === nodeId) {
      selectedNodeId = graphDraft.nodes[0]?.id || null;
    }
    pushHistorySnapshot();
  }

  function handleAddEdge(edge: VisualEdge) {
    graphDraft.edges = [...graphDraft.edges, edge];
    pushHistorySnapshot();
  }

  function handleDeleteEdge(edgeId: string) {
    graphDraft.edges = graphDraft.edges.filter((e) => e.id !== edgeId);
    pushHistorySnapshot();
  }

  function handleUpdateNodePos(nodeId: string, x: number, y: number) {
    const node = graphDraft.nodes.find((n) => n.id === nodeId);
    if (node) {
      node.x = x;
      node.y = y;
    }
  }

  function handleUpdateNodeData(updatedData: Record<string, any>) {
    if (!selectedNodeId) return;
    const node = graphDraft.nodes.find((n) => n.id === selectedNodeId);
    if (node) {
      node.data = updatedData;
      syncGraphToLinearForm();
      pushHistorySnapshot();
    }
  }

  // --- Export and Import ---
  function handleExportJson() {
    try {
      const jsonStr = exportWorkflowJson({
        name,
        description,
        enabled: enabled === 'true',
        max_retries: Number(maxRetries) || 3,
        nodes: graphDraft.nodes,
        edges: graphDraft.edges
      });
      navigator.clipboard.writeText(jsonStr);
      notifySuccess('Workflow JSON copied to clipboard');
    } catch {
      notifyError('Failed to export workflow JSON');
    }
  }

  function handleOpenImport() {
    importJsonText = '';
    showImportModal = true;
  }

  function handleConfirmImport() {
    if (!importJsonText.trim()) return;
    const res = importWorkflowJson(importJsonText);
    if (res.error || !res.draft) {
      notifyError(res.error || 'Failed to import JSON');
      return;
    }
    graphDraft = res.draft;
    name = res.draft.name;
    description = res.draft.description;
    enabled = res.draft.enabled ? 'true' : 'false';
    maxRetries = res.draft.max_retries;
    selectedNodeId = graphDraft.nodes[0]?.id || null;
    syncGraphToLinearForm();
    showImportModal = false;
    pushHistorySnapshot();
    notifySuccess('Workflow imported successfully');
  }

  // --- Pre-flight Validation & Publishing ---
  async function handleSave() {
    if (editorMode === 'linear') {
      syncLinearFormToGraph();
    }

    if (!name.trim()) {
      notifyError('Automation name is required');
      return;
    }

    const validation = validateWorkflowDraft({
      name,
      description,
      enabled: enabled === 'true',
      max_retries: Number(maxRetries) || 3,
      nodes: graphDraft.nodes,
      edges: graphDraft.edges
    });

    if (!validation.valid) {
      notifyError(validation.errors[0]?.message || 'Validation failed. Check step parameters.');
      return;
    }

    const payload: WorkflowCreateInput = graphToWorkflow({
      name,
      description,
      enabled: enabled === 'true',
      max_retries: Number(maxRetries) || 3,
      nodes: graphDraft.nodes,
      edges: graphDraft.edges
    });

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
    ? 'Workflow Studio — Edit Pipeline'
    : 'Workflow Studio — Visual Automation Builder'}
  subtitle="Production-grade graph-based visual automation editor. Flow runs from left to right."
  maxWidth="1380px"
  onclose={closeWorkflowBuilder}
>
  <div class="studio-wrapper">
    <!-- Top Toolbar -->
    <div class="studio-toolbar">
      <div class="toolbar-left">
        <div class="mode-pills">
          <button
            class="mode-pill {editorMode === 'visual' ? 'active' : ''}"
            onclick={() => (editorMode = 'visual')}
          >
            ● Visual Graph
          </button>
          <button
            class="mode-pill {editorMode === 'linear' ? 'active' : ''}"
            onclick={() => (editorMode = 'linear')}
          >
            ≡ Linear Form
          </button>
        </div>

        <div class="history-controls">
          <button
            class="tool-btn"
            title="Undo (Ctrl+Z)"
            disabled={historyPointer <= 0}
            onclick={handleUndo}
          >
            ↺ Undo
          </button>
          <button
            class="tool-btn"
            title="Redo (Ctrl+Y)"
            disabled={historyPointer >= historyStack.length - 1}
            onclick={handleRedo}
          >
            ↻ Redo
          </button>
        </div>
      </div>

      <div class="toolbar-right">
        <div class="validation-pill {validationResult.valid ? 'valid' : 'invalid'}">
          {#if validationResult.valid}
            <span class="dot green"></span>
            <span>Valid Pipeline</span>
          {:else}
            <span class="dot amber"></span>
            <span
              >{validationResult.errors.length} Issue{validationResult.errors.length > 1
                ? 's'
                : ''}</span
            >
          {/if}
        </div>

        <button
          class="tool-btn"
          onclick={handleExportJson}
          title="Export Workflow Definition as JSON"
        >
          Export JSON
        </button>
        <button class="tool-btn" onclick={handleOpenImport} title="Import Workflow from JSON">
          Import JSON
        </button>
      </div>
    </div>

    <!-- General Automation Metadata Grid -->
    <div class="meta-grid">
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

      <Input
        label="Description"
        placeholder="Filter incoming messages, transform payload structure, and dispatch to destination."
        bind:value={description}
      />
    </div>

    <!-- Main Workspace Area -->
    {#if editorMode === 'visual'}
      <div class="visual-workspace">
        <!-- Visual Graph Canvas starting strictly from left end -->
        <div class="canvas-container">
          <VisualCanvas
            nodes={graphDraft.nodes}
            edges={graphDraft.edges}
            {selectedNodeId}
            onselectnode={(id) => (selectedNodeId = id)}
            onaddnode={handleAddNode}
            ondeletenode={handleDeleteNode}
            onaddedge={handleAddEdge}
            ondeleteedge={handleDeleteEdge}
            onupdatenodepos={handleUpdateNodePos}
          />
        </div>

        <!-- Node Configuration Inspector -->
        <div class="inspector-container">
          {#if selectedNode}
            <VisualNodeInspector
              node={selectedNode}
              integrations={integrationsCatalog}
              credentials={credentialsList}
              onchange={handleUpdateNodeData}
              ondelete={handleDeleteNode}
              onclose={() => (selectedNodeId = null)}
            />
          {:else}
            <div class="empty-inspector">
              <Icon name="workflow" size={28} />
              <div class="empty-inspector-title">No Step Selected</div>
              <div class="empty-inspector-desc">
                Click any step on the left-to-right canvas to configure its triggers, conditions, or
                actions.
              </div>
            </div>
          {/if}
        </div>
      </div>
    {:else}
      <!-- Linear Form View (Compatibility Fallback) -->
      <div class="linear-forms-container">
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
          {actionSchema}
          bind:maxRetries
          integrations={actionIntegrations}
          actionTypes={actionTypeOptions}
          credentials={credentialsList}
          onIntegrationChange={onActionIntegrationChange}
          onTypeChange={onActionTypeChange}
        />
      </div>
    {/if}

    <!-- Validation Issues Banner (if any) -->
    {#if !validationResult.valid && validationResult.errors.length > 0}
      <div class="validation-banner">
        <div class="banner-title">PRE-FLIGHT VALIDATION ISSUES:</div>
        <ul class="banner-list">
          {#each validationResult.errors as err}
            <li>{err.message}</li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>

  <!-- Import Modal Drawer -->
  {#if showImportModal}
    <div class="import-overlay">
      <div class="import-card">
        <div class="import-header">
          <h4>Import Workflow JSON</h4>
          <button class="close-btn" onclick={() => (showImportModal = false)}>×</button>
        </div>
        <textarea
          class="import-textarea"
          rows={10}
          placeholder="Paste workflow JSON here..."
          bind:value={importJsonText}></textarea>
        <div class="import-actions">
          <Button variant="ghost" size="sm" onclick={() => (showImportModal = false)}>Cancel</Button
          >
          <Button variant="primary" size="sm" onclick={handleConfirmImport}>Load Graph</Button>
        </div>
      </div>
    </div>
  {/if}

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
  .studio-wrapper {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 4px 0;
  }

  .studio-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 12px;
    background: var(--bg-surface-elevated, #16181e);
    border: 1px solid var(--border-default, #272a30);
    border-radius: var(--radius-sm, 6px);
  }

  .toolbar-left,
  .toolbar-right {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .mode-pills {
    display: flex;
    align-items: center;
    background: var(--bg-surface, #101216);
    border: 1px solid var(--border-subtle, #272a30);
    border-radius: 4px;
    padding: 2px;
  }

  .mode-pill {
    background: transparent;
    border: none;
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary, #8b949e);
    padding: 4px 10px;
    border-radius: 3px;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .mode-pill.active {
    background: var(--bg-surface-elevated, #21262d);
    color: var(--text-primary, #e6edf3);
  }

  .history-controls {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .tool-btn {
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    font-weight: 600;
    padding: 4px 10px;
    background: var(--bg-surface, #121417);
    border: 1px solid var(--border-subtle, #30363d);
    border-radius: 4px;
    color: var(--text-secondary, #c9d1d9);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .tool-btn:hover:not(:disabled) {
    background: var(--bg-surface-hover, #21262d);
    color: var(--text-primary, #fff);
  }

  .tool-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .validation-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 8px;
    border-radius: 4px;
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    font-weight: 700;
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid var(--border-subtle, #272a30);
  }

  .validation-pill.valid {
    color: #34d399;
  }

  .validation-pill.invalid {
    color: #fbbf24;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
  }
  .dot.green {
    background: #10b981;
  }
  .dot.amber {
    background: #f59e0b;
  }

  .meta-grid {
    display: grid;
    grid-template-columns: 2fr 1fr 2fr;
    gap: 12px;
    background: rgba(255, 255, 255, 0.01);
    padding: 8px 12px;
    border: 1px solid var(--border-subtle, #20242b);
    border-radius: 6px;
  }

  .visual-workspace {
    display: grid;
    grid-template-columns: 1fr 360px;
    gap: 12px;
    height: 520px;
  }

  .canvas-container {
    height: 100%;
    min-width: 0;
  }

  .inspector-container {
    height: 100%;
    border-radius: var(--radius-md, 8px);
    overflow: hidden;
  }

  .empty-inspector {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    padding: 30px;
    text-align: center;
    background: var(--bg-surface-elevated, #16181e);
    border: 1px dashed var(--border-default, #272a30);
    border-radius: var(--radius-md, 8px);
    color: var(--text-muted, #71767b);
  }

  .empty-inspector-title {
    margin-top: 10px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary, #9aa0a6);
  }

  .empty-inspector-desc {
    margin-top: 6px;
    font-size: 11px;
    line-height: 1.4;
  }

  .linear-forms-container {
    display: flex;
    flex-direction: column;
    gap: 14px;
    max-height: 520px;
    overflow-y: auto;
    padding-right: 6px;
  }

  .validation-banner {
    background: rgba(245, 158, 11, 0.08);
    border: 1px solid rgba(245, 158, 11, 0.3);
    border-radius: 6px;
    padding: 10px 14px;
  }

  .banner-title {
    font-family: var(--font-mono, monospace);
    font-size: 10.5px;
    font-weight: 700;
    color: #fbbf24;
    letter-spacing: 0.05em;
  }

  .banner-list {
    margin: 4px 0 0 16px;
    padding: 0;
    font-size: 11.5px;
    color: var(--text-secondary, #d1d5db);
  }

  /* Import Modal */
  .import-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1100;
  }

  .import-card {
    width: 600px;
    background: var(--bg-surface-elevated, #16181e);
    border: 1px solid var(--border-default, #272a30);
    border-radius: 8px;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .import-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .import-header h4 {
    margin: 0;
    font-size: 14px;
    color: var(--text-primary, #fff);
  }

  .close-btn {
    background: transparent;
    border: none;
    font-size: 20px;
    color: var(--text-muted, #8b949e);
    cursor: pointer;
  }

  .import-textarea {
    width: 100%;
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    background: #0d1117;
    color: #e6edf3;
    border: 1px solid #30363d;
    border-radius: 4px;
    padding: 10px;
    box-sizing: border-box;
    resize: vertical;
  }

  .import-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
