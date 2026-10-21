<script lang="ts">
  import {
    executionDetailModal,
    closeExecutionDetail,
    triggerRefresh,
    notifySuccess,
    notifyError
  } from '../../api/stores';
  import { api } from '../../api/client';
  import type { Execution } from '../../api/types';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';
  import Badge from '../common/Badge.svelte';
  import CodeBox from '../common/CodeBox.svelte';

  let execution = $state<Execution | null>(null);
  let loading = $state(false);
  let retrying = $state(false);

  $effect(() => {
    if ($executionDetailModal.open && $executionDetailModal.executionId) {
      loadExecution($executionDetailModal.executionId);
    } else {
      execution = null;
    }
  });

  async function loadExecution(id: string) {
    loading = true;
    try {
      execution = await api.getExecution(id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load execution';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  async function handleRetry() {
    if (!execution) return;
    retrying = true;
    try {
      await api.retryExecution(execution.id);
      notifySuccess('Retry triggered for execution run');
      triggerRefresh();
      await loadExecution(execution.id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Retry failed';
      notifyError(msg);
    } finally {
      retrying = false;
    }
  }
</script>

<Dialog
  open={$executionDetailModal.open}
  title="Execution Lifecycle Audit"
  subtitle={execution ? `Run ${execution.id} against Workflow ${execution.workflow_id.slice(0, 8)}...` : 'Loading audit record...'}
  maxWidth="760px"
  onclose={closeExecutionDetail}
>
  {#if loading && !execution}
    <div class="state-loading">Loading execution audit trail...</div>
  {:else if execution}
    <div class="audit-wrapper">
      <!-- Top Meta Banner -->
      <div class="meta-banner">
        <div class="meta-col">
          <span class="meta-label">EXECUTION ID</span>
          <code class="meta-val">{execution.id}</code>
        </div>
        <div class="meta-col">
          <span class="meta-label">WORKFLOW ID</span>
          <code class="meta-val">{execution.workflow_id}</code>
        </div>
        <div class="meta-col">
          <span class="meta-label">STATUS</span>
          <div>
            {#if execution.status === 'success'}
              <Badge variant="success">SUCCESS</Badge>
            {:else if execution.status === 'failed'}
              <Badge variant="failed">FAILED</Badge>
            {:else if execution.status === 'running'}
              <Badge variant="running">RUNNING</Badge>
            {:else}
              <Badge variant="warning">{execution.status}</Badge>
            {/if}
          </div>
        </div>
        <div class="meta-col">
          <span class="meta-label">DURATION</span>
          <span class="meta-val mono">{execution.duration_ms} ms</span>
        </div>
      </div>

      <!-- Error Card if present -->
      {#if execution.error}
        <div class="error-card">
          <div class="error-title">EXECUTION ERROR</div>
          <div class="error-message">{execution.error}</div>
        </div>
      {/if}

      <!-- Step Lifecycle Timeline -->
      <div class="steps-section">
        <h4 class="steps-title">GRANULAR STAGES TIMELINE</h4>

        {#if execution.steps && execution.steps.length > 0}
          <div class="timeline-container">
            {#each execution.steps as step, idx}
              <div class="timeline-item">
                <div class="timeline-rail">
                  <div class="timeline-marker {step.status}"></div>
                  {#if idx < (execution.steps?.length || 0) - 1}
                    <div class="timeline-line"></div>
                  {/if}
                </div>

                <div class="timeline-content">
                  <div class="step-header">
                    <div class="step-title-row">
                      {#if step.step_type === 'trigger'}
                        <Badge variant="stage-when">WHEN TRIGGER</Badge>
                      {:else if step.step_type === 'condition'}
                        <Badge variant="stage-if">IF CONDITION</Badge>
                      {:else if step.step_type === 'transformation'}
                        <Badge variant="stage-transform">TRANSFORM</Badge>
                      {:else}
                        <Badge variant="stage-then">THEN ACTION</Badge>
                      {/if}
                      <span class="step-name">{step.name}</span>
                    </div>

                    <div class="step-meta-row">
                      <span class="step-duration mono">{step.duration_ms} ms</span>
                      {#if step.status === 'success'}
                        <Badge variant="success">PASS</Badge>
                      {:else if step.status === 'skipped'}
                        <Badge variant="skipped">SKIPPED</Badge>
                      {:else}
                        <Badge variant="failed">FAIL</Badge>
                      {/if}
                    </div>
                  </div>

                  {#if step.error}
                    <div class="step-error">{step.error}</div>
                  {/if}

                  {#if step.output}
                    <div class="step-output-box">
                      <CodeBox content={step.output} maxHeight="140px" />
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {:else}
          <div class="empty-steps">No granular steps recorded for this execution.</div>
        {/if}
      </div>
    </div>
  {/if}

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeExecutionDetail}>
      Close
    </Button>
    {#if execution && execution.status === 'failed'}
      <Button
        variant="primary"
        size="sm"
        icon="refresh"
        loading={retrying}
        onclick={handleRetry}
      >
        Retry Execution
      </Button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .state-loading {
    padding: 30px;
    text-align: center;
    color: var(--text-muted);
    font-size: 12px;
  }

  .audit-wrapper {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .meta-banner {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 12px 14px;
  }

  .meta-col {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }

  .meta-label {
    font-family: var(--font-mono);
    font-size: 9.5px;
    color: var(--text-muted);
    letter-spacing: 0.05em;
  }

  .meta-val {
    font-size: 11.5px;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .error-card {
    background: var(--accent-crimson-dim);
    border: 1px solid var(--accent-crimson-border);
    border-radius: var(--radius-sm);
    padding: 12px 14px;
  }

  .error-title {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 700;
    color: var(--accent-crimson);
    letter-spacing: 0.05em;
  }

  .error-message {
    font-family: var(--font-mono);
    font-size: 11.5px;
    color: var(--text-primary);
    margin-top: 4px;
    word-break: break-all;
  }

  .steps-section {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .steps-title {
    font-family: var(--font-mono);
    font-size: 10.5px;
    font-weight: 600;
    color: var(--text-muted);
    letter-spacing: 0.06em;
  }

  .timeline-container {
    display: flex;
    flex-direction: column;
  }

  .timeline-item {
    display: flex;
    gap: 12px;
  }

  .timeline-rail {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 14px;
    flex-shrink: 0;
  }

  .timeline-marker {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-top: 7px;
    flex-shrink: 0;
  }
  .timeline-marker.success {
    background: var(--accent-emerald);
    box-shadow: 0 0 6px var(--accent-emerald);
  }
  .timeline-marker.skipped {
    background: var(--accent-amber);
  }
  .timeline-marker.failed {
    background: var(--accent-crimson);
    box-shadow: 0 0 6px var(--accent-crimson);
  }

  .timeline-line {
    width: 1px;
    flex: 1;
    background: var(--border-default);
    margin: 4px 0;
  }

  .timeline-content {
    flex: 1;
    padding-bottom: 16px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }

  .step-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .step-title-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .step-name {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .step-meta-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .step-duration {
    font-size: 11px;
    color: var(--text-muted);
  }

  .step-error {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--accent-crimson);
  }

  .step-output-box {
    margin-top: 2px;
  }

  .empty-steps {
    padding: 16px;
    color: var(--text-muted);
    font-size: 12px;
    text-align: center;
    background: var(--bg-surface-elevated);
    border-radius: var(--radius-sm);
  }
</style>
