import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import DispatchEventModal from './DispatchEventModal.svelte';
import { openDispatchModal, closeDispatchModal, notifications } from '../../api/stores';
import { api } from '../../api/client';
import type { EventRecord, WorkflowTestResponse } from '../../api/types';

describe('DispatchEventModal Component', () => {
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
    closeDispatchModal();
  });

  it('handles event injection and workflow test execution', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(DispatchEventModal, { target });

    // 1. General ingestion mode
    openDispatchModal();
    await new Promise((r) => setTimeout(r, 20));

    expect(target.querySelector('.dialog-panel')).toBeTruthy();

    const mockIngest = vi.spyOn(api, 'ingestEvent').mockResolvedValue({
      id: 'ev-test-123',
      source_id: 'manual_tester',
      external_event_id: 'ext-123',
      event_type: 'event.manual',
      payload: { status: 'received' },
      created_at: new Date().toISOString()
    } as EventRecord);

    const submitBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Dispatch Event')
    );
    expect(submitBtn).toBeTruthy();

    submitBtn?.click();
    await new Promise((r) => setTimeout(r, 30));

    expect(mockIngest).toHaveBeenCalledTimes(1);

    // 2. Specific workflow test mode
    openDispatchModal('wf-999', 'Sync GitHub Issues');
    await new Promise((r) => setTimeout(r, 20));

    const mockTest = vi.spyOn(api, 'testWorkflow').mockResolvedValue({
      status: 'success',
      execution: {
        id: 'exec-test-1',
        workflow_id: 'wf-999',
        event_id: 'ev-test-123',
        status: 'success',
        attempts: 1,
        max_attempts: 3,
        duration_ms: 10,
        next_attempt_at: new Date().toISOString(),
        started_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString()
      }
    } as WorkflowTestResponse);

    const testBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Execute Workflow Test')
    );
    expect(testBtn).toBeTruthy();

    testBtn?.click();
    await new Promise((r) => setTimeout(r, 30));

    expect(mockTest).toHaveBeenCalledTimes(1);
  });
});
