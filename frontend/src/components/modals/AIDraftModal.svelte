<script lang="ts">
  import {
    aiDraftModal,
    closeAIDraftModal,
    triggerRefresh,
    notifySuccess,
    notifyError
  } from '../../api/stores';
  import { api } from '../../api/client';
  import type { AIProposal } from '../../api/types';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';
  import Badge from '../common/Badge.svelte';
  import CodeBox from '../common/CodeBox.svelte';

  let prompt = $state('Forward Stripe failed payment events to our Telegram alerts channel');
  let connectors = $state('http, telegram, webhook');
  let loading = $state(false);
  let deciding = $state(false);
  let proposal = $state<AIProposal | null>(null);

  $effect(() => {
    if (!$aiDraftModal) {
      proposal = null;
    }
  });

  async function handleDraft() {
    if (!prompt.trim()) {
      notifyError('Please enter natural language instructions for the workflow');
      return;
    }

    loading = true;
    try {
      const allowed = connectors
        .split(',')
        .map((c) => c.trim())
        .filter(Boolean);
      proposal = await api.draftWorkflowAI(prompt.trim(), allowed.length ? allowed : undefined);
      notifySuccess('Workflow proposal generated. Review and approve before instantiation.');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'AI drafting failed';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }

  async function handleAccept() {
    if (!proposal) return;
    deciding = true;
    try {
      const res = await api.acceptAIProposal(proposal.id, true);
      notifySuccess(`Proposal accepted! Instantiated workflow ${res.applied_workflow_id || ''}`);
      triggerRefresh();
      closeAIDraftModal();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Acceptance failed';
      notifyError(msg);
    } finally {
      deciding = false;
    }
  }

  async function handleReject() {
    if (!proposal) return;
    deciding = true;
    try {
      await api.rejectAIProposal(proposal.id, 'User declined proposal');
      notifySuccess('Proposal rejected');
      triggerRefresh();
      closeAIDraftModal();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Rejection failed';
      notifyError(msg);
    } finally {
      deciding = false;
    }
  }
</script>

<Dialog
  open={$aiDraftModal}
  title="Controlled AI Workflow Assistant"
  subtitle="Autonomous models propose changes requiring strict human review and policy validation before instantiation."
  maxWidth="750px"
  onclose={closeAIDraftModal}
>
  <div class="ai-modal-content">
    <div class="form-group">
      <label for="ai-prompt" class="form-label">Natural Language Instructions</label>
      <textarea
        id="ai-prompt"
        class="form-textarea"
        rows="3"
        bind:value={prompt}
        placeholder="e.g. When a GitHub release is published, post a formatted announcement to Telegram."
      ></textarea>
      <span class="form-hint">
        Secrets and sensitive fields are automatically redacted before external model requests.
      </span>
    </div>

    <div class="form-group">
      <label for="ai-connectors" class="form-label">Allowed Connectors (comma-separated)</label>
      <input
        id="ai-connectors"
        class="form-input mono"
        type="text"
        bind:value={connectors}
        placeholder="http, telegram, github, discord"
      />
    </div>

    <div class="draft-action-row">
      <Button variant="secondary" size="sm" loading={loading} onclick={handleDraft}>
        Draft Workflow Proposal
      </Button>
    </div>

    {#if proposal}
      <div class="proposal-card">
        <div class="proposal-header">
          <div class="proposal-title-row">
            <span class="proposal-title">AI Proposal: {proposal.task}</span>
            <Badge variant={proposal.status === 'accepted' ? 'success' : 'warning'}>
              {proposal.status.toUpperCase()}
            </Badge>
          </div>
          <div class="proposal-meta mono">
            Tokens used: {proposal.tokens_used} | Model: {proposal.model_used}
          </div>
        </div>

        <div class="proposal-section">
          <div class="section-title">Model Rationale</div>
          <p class="explanation-text">{proposal.explanation || 'No rationale provided.'}</p>
        </div>

        <div class="proposal-section">
          <div class="section-title-row">
            <span class="section-title">Deterministic Safety & Policy Validation</span>
            {#if proposal.validation?.valid}
              <Badge variant="success">PASSED</Badge>
            {:else}
              <Badge variant="failed">VIOLATIONS DETECTED</Badge>
            {/if}
          </div>

          {#if proposal.validation?.errors && proposal.validation.errors.length > 0}
            <div class="validation-box error">
              <div class="box-title">Safety Violations (Preventing Acceptance):</div>
              <ul>
                {#each proposal.validation.errors as err}
                  <li>{err}</li>
                {/each}
              </ul>
            </div>
          {/if}

          {#if proposal.validation?.warnings && proposal.validation.warnings.length > 0}
            <div class="validation-box warning">
              <div class="box-title">Audit Warnings:</div>
              <ul>
                {#each proposal.validation.warnings as warn}
                  <li>{warn}</li>
                {/each}
              </ul>
            </div>
          {/if}
        </div>

        {#if proposal.content}
          <div class="proposal-section">
            <div class="section-title">Proposed Workflow Definition</div>
            <CodeBox content={proposal.content} maxHeight="220px" />
          </div>
        {/if}
      </div>
    {/if}
  </div>

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeAIDraftModal}>Cancel</Button>
    {#if proposal && proposal.status === 'pending_approval'}
      <Button variant="danger" size="sm" loading={deciding} onclick={handleReject}>
        Reject
      </Button>
      <Button
        variant="primary"
        size="sm"
        loading={deciding}
        disabled={!proposal.validation?.valid}
        onclick={handleAccept}
      >
        Approve & Instantiate Workflow
      </Button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .ai-modal-content {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .form-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-normal);
  }

  .form-textarea,
  .form-input {
    width: 100%;
    padding: 8px 12px;
    background: var(--bg-surface-2);
    border: 1px solid var(--border-subtle);
    border-radius: 6px;
    color: var(--text-normal);
    font-size: 13px;
  }

  .form-textarea:focus,
  .form-input:focus {
    outline: none;
    border-color: var(--accent-primary);
  }

  .form-hint {
    font-size: 11px;
    color: var(--text-muted);
  }

  .draft-action-row {
    display: flex;
    justify-content: flex-end;
  }

  .proposal-card {
    background: var(--bg-surface-2);
    border: 1px solid var(--border-subtle);
    border-radius: 8px;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    margin-top: 8px;
  }

  .proposal-header {
    border-bottom: 1px solid var(--border-subtle);
    padding-bottom: 10px;
  }

  .proposal-title-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .proposal-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-normal);
  }

  .proposal-meta {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 4px;
  }

  .proposal-section {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .section-title-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .section-title {
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }

  .explanation-text {
    font-size: 13px;
    color: var(--text-normal);
    line-height: 1.5;
    margin: 0;
  }

  .validation-box {
    padding: 10px 14px;
    border-radius: 6px;
    font-size: 12px;
  }

  .validation-box.error {
    background: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.3);
    color: #ef4444;
  }

  .validation-box.warning {
    background: rgba(245, 158, 11, 0.1);
    border: 1px solid rgba(245, 158, 11, 0.3);
    color: #f59e0b;
  }

  .validation-box ul {
    margin: 6px 0 0 18px;
    padding: 0;
  }

  .box-title {
    font-weight: 600;
  }
</style>
