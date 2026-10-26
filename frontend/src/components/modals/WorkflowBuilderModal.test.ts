import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import WorkflowBuilderModal from './WorkflowBuilderModal.svelte';
import {
  openCreateWorkflow,
  openEditWorkflow,
  closeWorkflowBuilder,
  notifications
} from '../../api/stores';
import { api } from '../../api/client';
import type { Workflow } from '../../api/types';

describe('WorkflowBuilderModal Component', () => {
  let target: HTMLElement;
  let instance: any = null;

  beforeEach(() => {
    notifications.set([]);
    vi.spyOn(api, 'listCredentials').mockResolvedValue([]);
  });

  afterEach(() => {
    if (instance) {
      unmount(instance);
      instance = null;
    }
    if (target && target.parentNode) {
      target.parentNode.removeChild(target);
    }
    closeWorkflowBuilder();
  });

  it('validates name, creates workflow, and supports edit mode', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(WorkflowBuilderModal, { target });

    // 1. Create Mode validation
    openCreateWorkflow('discord');
    await new Promise((r) => setTimeout(r, 40));

    expect(target.querySelector('.dialog-panel')).toBeTruthy();

    const saveBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Save Automation')
    );
    expect(saveBtn).toBeTruthy();

    // Click without name -> validation error notification
    saveBtn?.click();
    await new Promise((r) => setTimeout(r, 30));

    let currentNotifications: any[] = [];
    notifications.subscribe((n) => (currentNotifications = n))();
    expect(
      currentNotifications.some((n) => n.message.includes('Automation name is required'))
    ).toBe(true);

    // Enter name
    const nameInput = target.querySelector<HTMLInputElement>('input');
    if (nameInput) {
      nameInput.value = 'Discord Alert Pipeline';
      nameInput.dispatchEvent(new Event('input'));
    }

    const mockCreate = vi.spyOn(api, 'createWorkflow').mockResolvedValue({
      id: 'wf-created-1',
      name: 'Discord Alert Pipeline',
      enabled: true,
      max_retries: 3,
      trigger: { type: 'telegram' },
      actions: [{ integration: 'discord', action_type: 'send_message', config: {} }],
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString()
    });

    saveBtn?.click();
    await new Promise((r) => setTimeout(r, 40));

    expect(mockCreate).toHaveBeenCalledTimes(1);

    // 2. Edit Mode
    const existingWorkflow: Workflow = {
      id: 'wf-edit-1',
      name: 'Existing Bot Mirror',
      description: 'Production mirror pipeline',
      enabled: true,
      max_retries: 5,
      trigger: {
        type: 'schedule',
        event_types: ['schedule.tick'],
        config: { interval: '10m' }
      },
      conditions: {
        field: 'payload.text',
        operator: 'contains',
        value: 'URGENT'
      },
      transformations: {
        type: 'template',
        template: 'Alert: {{payload.text}}'
      },
      actions: [
        {
          integration: 'discord',
          action_type: 'send_message',
          config: { content: 'hello' }
        }
      ],
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString()
    };

    openEditWorkflow(existingWorkflow);
    await new Promise((r) => setTimeout(r, 40));

    const editSaveBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Update Automation')
    );
    expect(editSaveBtn).toBeTruthy();

    const mockUpdate = vi.spyOn(api, 'updateWorkflow').mockResolvedValue(existingWorkflow);

    editSaveBtn?.click();
    await new Promise((r) => setTimeout(r, 40));

    expect(mockUpdate).toHaveBeenCalledWith('wf-edit-1', expect.anything());
  });
});
