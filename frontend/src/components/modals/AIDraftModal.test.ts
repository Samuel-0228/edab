import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import AIDraftModal from './AIDraftModal.svelte';
import { openAIDraftModal, closeAIDraftModal, notifications } from '../../api/stores';
import { api } from '../../api/client';
import type { AIProposal } from '../../api/types';

describe('AIDraftModal Component', () => {
  let target: HTMLElement;
  let instance: any = null;

  beforeEach(() => {
    notifications.set([]);
  });

  afterEach(() => {
    if (instance) {
      unmount(instance);
      instance = null;
    }
    if (target && target.parentNode) {
      target.parentNode.removeChild(target);
    }
    closeAIDraftModal();
  });

  it('renders modal, calls draft API, and allows human approval', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(AIDraftModal, { target });

    openAIDraftModal();
    await new Promise((r) => setTimeout(r, 20));

    expect(target.querySelector('.dialog-panel')).toBeTruthy();

    const mockDraft = vi.spyOn(api, 'draftWorkflowAI').mockResolvedValue({
      id: 'prop-test-1',
      tenant_id: 'default',
      task: 'draft_workflow',
      status: 'pending_approval',
      prompt: 'Sync Github issues',
      explanation: 'Drafted pipeline with webhook trigger and telegram alert',
      validation: { valid: true },
      tokens_used: 120,
      model_used: 'gpt-4o',
      created_at: new Date().toISOString()
    } as AIProposal);

    const draftBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Draft Workflow Proposal')
    );
    expect(draftBtn).toBeTruthy();

    draftBtn?.click();
    await new Promise((r) => setTimeout(r, 30));

    expect(mockDraft).toHaveBeenCalledTimes(1);

    // After proposal is rendered, verify approve button is available
    await new Promise((r) => setTimeout(r, 20));
    const approveBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Approve & Instantiate Workflow')
    );
    expect(approveBtn).toBeTruthy();

    const mockAccept = vi.spyOn(api, 'acceptAIProposal').mockResolvedValue({
      proposal: {
        id: 'prop-test-1',
        tenant_id: 'default',
        task: 'draft_workflow',
        status: 'accepted',
        prompt: 'Sync Github issues',
        explanation: 'Approved',
        validation: { valid: true },
        tokens_used: 120,
        model_used: 'gpt-4o',
        created_at: new Date().toISOString()
      },
      applied_workflow_id: 'wf-created-123'
    });

    approveBtn?.click();
    await new Promise((r) => setTimeout(r, 30));

    expect(mockAccept).toHaveBeenCalledWith('prop-test-1', true);
  });
});
