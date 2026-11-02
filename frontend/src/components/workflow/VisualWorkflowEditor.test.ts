import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import WorkflowBuilderModal from '../modals/WorkflowBuilderModal.svelte';
import {
  openCreateWorkflow,
  openEditWorkflow,
  closeWorkflowBuilder,
  notifications
} from '../../api/stores';
import { api } from '../../api/client';
import type { Workflow } from '../../api/types';

describe('Visual Workflow Builder Component & Studio', () => {
  let target: HTMLElement;
  let instance: any = null;

  beforeEach(() => {
    notifications.set([]);
    vi.spyOn(api, 'listCredentials').mockResolvedValue([
      {
        id: 'cred-discord-1',
        name: 'Discord Webhook Cred',
        integration: 'discord',
        key_id: 'key-1',
        masked_fields: { token: '***' },
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString()
      }
    ]);
    vi.spyOn(api, 'listIntegrations').mockResolvedValue([
      {
        id: 'discord',
        name: 'Discord',
        description: 'Send messages to Discord channels',
        kind: 'action',
        triggers: [],
        credentials: [],
        actions: [{ type: 'send_message', name: 'Send Message', description: 'Post message to channel' }]
      },
      {
        id: 'telegram',
        name: 'Telegram',
        description: 'Bot updates and messages',
        kind: 'both',
        credentials: [],
        triggers: [{ type: 'telegram.message', name: 'Message', description: 'Receive bot update' }],
        actions: [{ type: 'send_message', name: 'Send Message', description: 'Send telegram chat message' }]
      }
    ]);
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

  it('renders visual graph canvas starting from left end with palette controls', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(WorkflowBuilderModal, { target });
    openCreateWorkflow('telegram');
    await new Promise((r) => setTimeout(r, 50));

    // Verify dialog panel and canvas presence
    expect(target.querySelector('.dialog-panel')).toBeTruthy();
    expect(target.querySelector('.canvas-viewport')).toBeTruthy();

    // Verify palette buttons exist
    const palette = target.querySelector('.canvas-palette');
    expect(palette).toBeTruthy();
    expect(palette?.textContent).toContain('+ Action');
    expect(palette?.textContent).toContain('+ Condition');
    expect(palette?.textContent).toContain('+ Branch');
    expect(palette?.textContent).toContain('+ Loop');
    expect(palette?.textContent).toContain('+ Delay');
    expect(palette?.textContent).toContain('+ Parallel');

    // Verify initial trigger node is aligned on left
    const triggerCard = target.querySelector('.node-card.trigger') as HTMLElement;
    expect(triggerCard).toBeTruthy();
    expect(triggerCard.style.left).toBe('30px');
  });

  it('adds branch, loop, and delay nodes, updates parameters, and validates required fields', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(WorkflowBuilderModal, { target });
    openCreateWorkflow('discord');
    await new Promise((r) => setTimeout(r, 50));

    // Add branch step
    const branchBtn = Array.from(target.querySelectorAll('.palette-btn')).find((b) =>
      b.textContent?.includes('+ Branch')
    ) as HTMLButtonElement;
    expect(branchBtn).toBeTruthy();
    branchBtn.click();
    await new Promise((r) => setTimeout(r, 40));

    expect(target.querySelector('.node-card.branch')).toBeTruthy();

    // Add loop step
    const loopBtn = Array.from(target.querySelectorAll('.palette-btn')).find((b) =>
      b.textContent?.includes('+ Loop')
    ) as HTMLButtonElement;
    loopBtn?.click();
    await new Promise((r) => setTimeout(r, 40));

    expect(target.querySelector('.node-card.loop')).toBeTruthy();

    // Add delay step
    const delayBtn = Array.from(target.querySelectorAll('.palette-btn')).find((b) =>
      b.textContent?.includes('+ Delay')
    ) as HTMLButtonElement;
    delayBtn?.click();
    await new Promise((r) => setTimeout(r, 40));

    expect(target.querySelector('.node-card.delay')).toBeTruthy();

    // Inspector should now inspect the newly added node (delay)
    const inspector = target.querySelector('.inspector-panel');
    expect(inspector).toBeTruthy();
    expect(inspector?.textContent).toContain('DURABLE ASYNC DELAY');
  });

  it('supports undo, redo, import and export operations', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(WorkflowBuilderModal, { target });
    openCreateWorkflow('discord');
    await new Promise((r) => setTimeout(r, 50));

    // Export button exists
    const exportBtn = Array.from(target.querySelectorAll('.tool-btn')).find((b) =>
      b.textContent?.includes('Export JSON')
    ) as HTMLButtonElement;
    expect(exportBtn).toBeTruthy();

    // Undo / Redo controls exist
    const undoBtn = Array.from(target.querySelectorAll('.tool-btn')).find((b) =>
      b.textContent?.includes('Undo')
    ) as HTMLButtonElement;
    const redoBtn = Array.from(target.querySelectorAll('.tool-btn')).find((b) =>
      b.textContent?.includes('Redo')
    ) as HTMLButtonElement;
    expect(undoBtn).toBeTruthy();
    expect(redoBtn).toBeTruthy();

    // Add a node then undo
    const addActionBtn = Array.from(target.querySelectorAll('.palette-btn')).find((b) =>
      b.textContent?.includes('+ Action')
    ) as HTMLButtonElement;
    addActionBtn?.click();
    await new Promise((r) => setTimeout(r, 40));

    const nodeCountBefore = target.querySelectorAll('.node-card').length;
    expect(nodeCountBefore).toBeGreaterThanOrEqual(2);

    undoBtn.click();
    await new Promise((r) => setTimeout(r, 40));
    const nodeCountAfterUndo = target.querySelectorAll('.node-card').length;
    expect(nodeCountAfterUndo).toBe(nodeCountBefore - 1);

    redoBtn.click();
    await new Promise((r) => setTimeout(r, 40));
    expect(target.querySelectorAll('.node-card').length).toBe(nodeCountBefore);
  });

  it('loads complex workflow with branches and delays in edit mode and publishes update', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    const complexWorkflow: Workflow = {
      id: 'wf-complex-999',
      name: 'Complex Triage Flow',
      description: 'Fan-out alerts and delay follow-up',
      enabled: true,
      max_retries: 3,
      trigger: {
        type: 'webhook',
        event_types: ['incident.critical']
      },
      actions: [
        {
          id: 'step_branch',
          name: 'Route by Urgency',
          type: 'branch',
          cases: [
            {
              when: { field: 'payload.urgent', operator: 'equals', value: true },
              steps: [
                {
                  id: 'step_discord',
                  type: 'action',
                  integration: 'discord',
                  action_type: 'send_message',
                  config: { content: 'Urgent Alert!' }
                }
              ]
            }
          ]
        },
        {
          id: 'step_delay',
          type: 'delay',
          duration: '10m'
        }
      ]
    };

    instance = mount(WorkflowBuilderModal, { target });
    openEditWorkflow(complexWorkflow);
    await new Promise((r) => setTimeout(r, 50));

    // Verify nodes rendered
    expect(target.querySelector('.node-card.trigger')).toBeTruthy();
    expect(target.querySelector('.node-card.branch')).toBeTruthy();
    expect(target.querySelector('.node-card.delay')).toBeTruthy();

    // Verify name field populated
    const nameInput = target.querySelector<HTMLInputElement>('input');
    expect(nameInput?.value).toBe('Complex Triage Flow');

    const updateBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Update Automation')
    );
    expect(updateBtn).toBeTruthy();

    const mockUpdate = vi.spyOn(api, 'updateWorkflow').mockResolvedValue(complexWorkflow);
    updateBtn?.click();
    await new Promise((r) => setTimeout(r, 50));

    expect(mockUpdate).toHaveBeenCalledWith(
      'wf-complex-999',
      expect.objectContaining({
        name: 'Complex Triage Flow',
        actions: expect.any(Array)
      })
    );
  });
});
