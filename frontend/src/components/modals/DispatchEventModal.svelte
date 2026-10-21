<script lang="ts">
  import {
    dispatchModal,
    closeDispatchModal,
    triggerRefresh,
    notifySuccess,
    notifyError
  } from '../../api/stores';
  import { api } from '../../api/client';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';
  import Input from '../common/Input.svelte';
  import JsonEditor from '../common/JsonEditor.svelte';
  import CodeBox from '../common/CodeBox.svelte';
  import Badge from '../common/Badge.svelte';

  let sourceId = $state('manual_tester');
  let eventType = $state('telegram.message');
  let payloadRaw = $state(`{\n  "text": "Urgent technical alert: deployment status update",\n  "title": "System Diagnostic Event",\n  "author": "ops_engineer"\n}`);
  let loading = $state(false);
  let executionResult = $state<unknown | null>(null);

  $effect(() => {
    if ($dispatchModal.open) {
      executionResult = null;
      if ($dispatchModal.workflowId) {
        sourceId = `manual_test_${$dispatchModal.workflowId.slice(0, 6)}`;
        eventType = 'telegram.message';
      } else {
        sourceId = 'manual_tester';
        eventType = 'event.manual';
      }
    }
  });

  async function handleDispatch() {
    let parsedPayload: Record<string, unknown> = {};
    try {
      parsedPayload = JSON.parse(payloadRaw);
    } catch (e: unknown) {
      notifyError('Payload must be valid JSON');
      return;
    }

    loading = true;
    executionResult = null;

    try {
      if ($dispatchModal.workflowId) {
        // Direct workflow test execution
        const res = await api.testWorkflow($dispatchModal.workflowId, {
          source_id: sourceId.trim(),
          event_type: eventType.trim(),
          payload: parsedPayload
        });
        executionResult = res;
        notifySuccess('Workflow test executed successfully');
      } else {
        // Platform event ingestion
        const res = await api.ingestEvent({
          source_id: sourceId.trim(),
          external_event_id: `evt_manual_${Date.now()}`,
          event_type: eventType.trim(),
          payload: parsedPayload
        });
        executionResult = res;
        notifySuccess('Event ingested into Kerob pipeline');
      }
      triggerRefresh();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Dispatch failed';
      notifyError(msg);
      executionResult = { error: msg };
    } finally {
      loading = false;
    }
  }
</script>

<Dialog
  open={$dispatchModal.open}
  title={$dispatchModal.workflowId ? `Test Automation: ${$dispatchModal.workflowName || $dispatchModal.workflowId}` : 'Inject Event into Pipeline'}
  subtitle="Inject a normalized event payload to test trigger evaluation, conditions, template transforms, and downstream actions."
  maxWidth="680px"
  onclose={closeDispatchModal}
>
  <div class="dispatch-form">
    <div class="fields-row">
      <Input
        label="Source Identifier"
        placeholder="e.g. telegram_scraper or github_webhook"
        bind:value={sourceId}
        mono
        required
      />
      <Input
        label="Event Type"
        placeholder="e.g. telegram.message or webhook.event"
        bind:value={eventType}
        mono
        required
      />
    </div>

    <JsonEditor
      label="Event Payload (JSON)"
      bind:value={payloadRaw}
      minHeight="110px"
      hint="Key-value fields are available as &#123;&#123;payload.key&#125;&#125; inside workflow templates."
      required
    />

    {#if executionResult}
      <div class="result-section">
        <div class="result-header">
          <span class="result-title">EXECUTION AUDIT RESULT</span>
          {#if (executionResult as any)?.status === 'success'}
            <Badge variant="success">TEST SUCCESS</Badge>
          {:else if (executionResult as any)?.error}
            <Badge variant="failed">ERROR</Badge>
          {:else}
            <Badge variant="neutral">INGESTED</Badge>
          {/if}
        </div>
        <CodeBox content={executionResult} maxHeight="200px" />
      </div>
    {/if}
  </div>

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeDispatchModal}>
      Close
    </Button>
    <Button
      variant="primary"
      size="sm"
      {loading}
      icon="play"
      onclick={handleDispatch}
    >
      {$dispatchModal.workflowId ? 'Execute Workflow Test' : 'Dispatch Event'}
    </Button>
  {/snippet}
</Dialog>

<style>
  .dispatch-form {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .fields-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  .result-section {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 4px;
  }

  .result-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .result-title {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    letter-spacing: 0.05em;
    font-family: var(--font-mono);
  }
</style>
