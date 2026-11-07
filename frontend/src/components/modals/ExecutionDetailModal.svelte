<script lang="ts">
  import {
    executionDetailModal,
    closeExecutionDetail,
    triggerRefresh,
    notifySuccess,
    notifyError,
    currentIdentity,
    canOperateActions,
    latestOperationalEvent
  } from '../../api/stores';
  import { api } from '../../api/client';
  import type { Execution, Approval } from '../../api/types';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';
  import Badge from '../common/Badge.svelte';
  import CodeBox from '../common/CodeBox.svelte';

  let execution = $state<Execution | null>(null);
  let pendingApproval = $state<Approval | null>(null);
  let approvalNote = $state('');
  let loading = $state(false);
  let retrying = $state(false);
  let operating = $state(false);
  let explaining = $state(false);
  let aiDiagnosis = $state<string | null>(null);

  $effect(() => {
    if ($executionDetailModal.open && $executionDetailModal.executionId) {
      loadExecution($executionDetailModal.executionId);
    } else {
      execution = null;
      pendingApproval = null;
      approvalNote = '';
      aiDiagnosis = null;
    }
  });

  $effect(() => {
    const ev = $latestOperationalEvent;
    if (ev && execution && ev.execution_id === execution.id) {
      loadExecution(execution.id);
    }
  });

  async function loadExecution(id: string) {
    loading = true;
    try {
      execution = await api.getExecution(id);
      if (execution.status === 'waiting_approval') {
        const apps = await api.listApprovals({ execution_id: id, status: 'pending' });
        pendingApproval = apps[0] || null;
      } else {
        pendingApproval = null;
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load execution';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  async function handleApprove() {
    if (!pendingApproval) return;
    operating = true;
    try {
      await api.approveApproval(
        pendingApproval.id,
        approvalNote || 'Approved via execution audit modal'
      );
      notifySuccess('Approval granted — workflow execution resumed');
      triggerRefresh();
      if (execution) await loadExecution(execution.id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Approval failed';
      notifyError(msg);
    } finally {
      operating = false;
    }
  }

  async function handleReject() {
    if (!pendingApproval) return;
    operating = true;
    try {
      await api.rejectApproval(
        pendingApproval.id,
        approvalNote || 'Rejected via execution audit modal'
      );
      notifySuccess('Approval rejected — saga compensation rollback initiated');
      triggerRefresh();
      if (execution) await loadExecution(execution.id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Rejection failed';
      notifyError(msg);
    } finally {
      operating = false;
    }
  }

  async function handleCompensate() {
    if (!execution) return;
    if (
      !confirm(
        'Initiate saga rollback compensation for this execution? Completed steps with defined compensation actions will be undone.'
      )
    ) {
      return;
    }
    operating = true;
    try {
      await api.compensateExecution(execution.id, 'Operator-initiated rollback');
      notifySuccess('Saga compensation completed');
      triggerRefresh();
      await loadExecution(execution.id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Compensation failed';
      notifyError(msg);
    } finally {
      operating = false;
    }
  }

  async function handleExplainFailure() {
    if (!execution) return;
    explaining = true;
    try {
      const p = await api.explainFailureAI({
        workflow_id: execution.workflow_id,
        execution_id: execution.id,
        error: execution.error || 'Execution encountered an error'
      });
      aiDiagnosis = p.explanation;
      notifySuccess('AI failure analysis generated');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'AI diagnosis failed';
      notifyError(msg);
    } finally {
      explaining = false;
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

  async function handlePause() {
    if (!execution) return;
    operating = true;
    try {
      await api.pauseExecution(execution.id);
      notifySuccess('Execution paused');
      triggerRefresh();
      await loadExecution(execution.id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Pause failed';
      notifyError(msg);
    } finally {
      operating = false;
    }
  }

  async function handleResume() {
    if (!execution) return;
    operating = true;
    try {
      await api.resumeExecution(execution.id);
      notifySuccess('Execution resumed');
      triggerRefresh();
      await loadExecution(execution.id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Resume failed';
      notifyError(msg);
    } finally {
      operating = false;
    }
  }

  async function handleCancel() {
    if (!execution) return;
    operating = true;
    try {
      await api.cancelExecution(execution.id);
      notifySuccess('Execution cancelled');
      triggerRefresh();
      await loadExecution(execution.id);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Cancel failed';
      notifyError(msg);
    } finally {
      operating = false;
    }
  }
</script>

<Dialog
  open={$executionDetailModal.open}
  title="Execution Lifecycle Audit"
  subtitle={execution
    ? `Run ${execution.id} against Workflow ${execution.workflow_id.slice(0, 8)}...`
    : 'Loading audit record...'}
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
            {:else if execution.status === 'waiting_approval'}
              <Badge variant="warning">WAITING APPROVAL</Badge>
            {:else if execution.status === 'compensating'}
              <Badge variant="running">COMPENSATING</Badge>
            {:else if execution.status === 'compensated'}
              <Badge variant="neutral">COMPENSATED</Badge>
            {:else if execution.status === 'compensation_failed'}
              <Badge variant="failed">COMP FAILED</Badge>
            {:else if execution.status === 'paused'}
              <Badge variant="warning">PAUSED</Badge>
            {:else if execution.status === 'cancelled'}
              <Badge variant="neutral">CANCELLED</Badge>
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

      <!-- Pending Human Approval Banner -->
      {#if pendingApproval}
        <div class="approval-box">
          <div class="approval-box-header">
            <Badge variant="warning">ACTION REQUIRED: HUMAN APPROVAL</Badge>
            <span class="approval-box-title">{pendingApproval.title}</span>
          </div>
          {#if pendingApproval.description}
            <div class="approval-box-desc">{pendingApproval.description}</div>
          {/if}
          <div class="approval-box-meta">
            <span>Step ID: <code class="mono-id">{pendingApproval.step_id}</code></span>
            {#if pendingApproval.approver_roles?.length}
              <span>Required Roles: {pendingApproval.approver_roles.join(', ')}</span>
            {/if}
            <span>Expires: {new Date(pendingApproval.expires_at).toLocaleString()}</span>
          </div>

          <div class="approval-box-action-row">
            <input
              type="text"
              class="approval-note-input"
              placeholder="Optional decision note (e.g. Verified by Risk team)..."
              bind:value={approvalNote}
            />
            {#if canOperateActions($currentIdentity?.role)}
              <Button variant="primary" size="sm" loading={operating} onclick={handleApprove}>
                Approve & Resume
              </Button>
              <Button variant="danger" size="sm" loading={operating} onclick={handleReject}>
                Reject & Rollback
              </Button>
            {/if}
          </div>
        </div>
      {/if}

      <!-- Error Card if present -->
      {#if execution.error}
        <div class="error-card">
          <div class="error-title">EXECUTION ERROR</div>
          <div class="error-message">{execution.error}</div>
        </div>
      {/if}

      <!-- AI Diagnosis Card if present -->
      {#if aiDiagnosis}
        <div class="ai-diagnosis-card">
          <div class="ai-diagnosis-header">
            <span class="ai-diagnosis-title">AI Failure Diagnosis</span>
            <Badge variant="stage-if">ASSIST</Badge>
          </div>
          <div class="ai-diagnosis-body">{aiDiagnosis}</div>
        </div>
      {/if}

      <!-- Step Lifecycle Timeline -->
      <div class="steps-section">
        <h4 class="steps-title">GRANULAR STAGES TIMELINE</h4>

        {#if execution.steps && execution.steps.length > 0}
          <div class="timeline-container">
            {#each execution.steps as step, idx (step.id || idx)}
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
                      {:else if step.step_type === 'approval'}
                        <Badge variant="stage-if">APPROVAL STEP</Badge>
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
                      {:else if step.status === 'waiting_approval'}
                        <Badge variant="warning">WAITING</Badge>
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
    <Button variant="ghost" size="sm" onclick={closeExecutionDetail}>Close</Button>
    {#if execution && canOperateActions($currentIdentity?.role)}
      {#if execution.status === 'failed'}
        <Button variant="secondary" size="sm" loading={explaining} onclick={handleExplainFailure}>
          Diagnose with AI
        </Button>
        <Button variant="primary" size="sm" icon="refresh" loading={retrying} onclick={handleRetry}>
          Retry Execution
        </Button>
        <Button variant="danger" size="sm" loading={operating} onclick={handleCompensate}>
          Rollback (Compensate)
        </Button>
      {:else if execution.status === 'waiting_approval'}
        <Button variant="danger" size="sm" loading={operating} onclick={handleCompensate}>
          Rollback (Compensate)
        </Button>
      {:else if execution.status === 'running' || execution.status === 'pending' || execution.status === 'retrying'}
        <Button variant="secondary" size="sm" loading={operating} onclick={handlePause}>
          Pause Execution
        </Button>
        <Button variant="ghost" size="sm" loading={operating} onclick={handleCancel}>
          Cancel Execution
        </Button>
      {:else if execution.status === 'paused'}
        <Button variant="primary" size="sm" loading={operating} onclick={handleResume}>
          Resume Execution
        </Button>
        <Button variant="ghost" size="sm" loading={operating} onclick={handleCancel}>
          Cancel Execution
        </Button>
      {/if}
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

  .approval-box {
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-subtle);
    border-left: 3px solid #eab308;
    border-radius: var(--radius-sm);
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .approval-box-header {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .approval-box-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .approval-box-desc {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .approval-box-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    font-size: 11px;
    color: var(--text-muted);
  }

  .approval-box-action-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 4px;
  }

  .approval-note-input {
    flex: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    padding: 6px 10px;
    font-size: 12px;
    color: var(--text-primary);
  }

  .approval-note-input:focus {
    outline: none;
    border-color: var(--accent-cyan);
  }

  .error-card {
    background: var(--accent-crimson-dim);
    border: 1px solid var(--accent-crimson-border);
    border-radius: var(--radius-sm);
    padding: 12px 14px;
  }

  .ai-diagnosis-card {
    background: rgba(14, 165, 233, 0.08);
    border: 1px solid rgba(14, 165, 233, 0.25);
    border-radius: var(--radius-sm);
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 10px;
  }

  .ai-diagnosis-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .ai-diagnosis-title {
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.05em;
    color: var(--accent-cyan);
    text-transform: uppercase;
  }

  .ai-diagnosis-body {
    font-size: 13px;
    line-height: 1.5;
    color: var(--text-primary);
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
