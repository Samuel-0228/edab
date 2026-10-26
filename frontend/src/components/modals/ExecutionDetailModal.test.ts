import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import ExecutionDetailModal from './ExecutionDetailModal.svelte';
import { openExecutionDetail, closeExecutionDetail, notifications } from '../../api/stores';
import { api } from '../../api/client';
import type { Execution } from '../../api/types';

describe('ExecutionDetailModal Component', () => {
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
    closeExecutionDetail();
  });

  it('loads and displays execution details and triggers retry', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    const mockExecution: Execution = {
      id: 'exec-abc-123',
      workflow_id: 'wf-456-789',
      event_id: 'ev-111-222',
      status: 'failed',
      attempts: 3,
      max_attempts: 3,
      duration_ms: 245,
      next_attempt_at: new Date().toISOString(),
      started_at: new Date().toISOString(),
      error: 'Connection timed out reaching Discord API',
      steps: [
        {
          id: 'step-1',
          execution_id: 'exec-abc-123',
          step_type: 'trigger',
          name: 'Trigger: webhook',
          status: 'success',
          duration_ms: 5,
          started_at: new Date().toISOString(),
          completed_at: new Date().toISOString()
        },
        {
          id: 'step-2',
          execution_id: 'exec-abc-123',
          step_type: 'action',
          name: 'Action: Discord',
          status: 'failed',
          error: 'Connection timed out',
          duration_ms: 240,
          started_at: new Date().toISOString(),
          completed_at: new Date().toISOString()
        }
      ],
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString()
    };

    const mockGet = vi.spyOn(api, 'getExecution').mockResolvedValue(mockExecution);
    const mockRetry = vi.spyOn(api, 'retryExecution').mockResolvedValue(undefined);

    instance = mount(ExecutionDetailModal, { target });

    openExecutionDetail('exec-abc-123');
    await new Promise((r) => setTimeout(r, 40));

    expect(mockGet).toHaveBeenCalledWith('exec-abc-123');
    expect(target.textContent).toContain('exec-abc-123');
    expect(target.textContent).toContain('Connection timed out reaching Discord API');

    const retryBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Retry Execution')
    );
    expect(retryBtn).toBeTruthy();

    retryBtn?.click();
    await new Promise((r) => setTimeout(r, 40));

    expect(mockRetry).toHaveBeenCalledWith('exec-abc-123');
  });
});
