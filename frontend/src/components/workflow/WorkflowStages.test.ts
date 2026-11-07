import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import WorkflowTriggerStage from './WorkflowTriggerStage.svelte';
import WorkflowActionStage from './WorkflowActionStage.svelte';
import WorkflowConditionStage from './WorkflowConditionStage.svelte';
import WorkflowTransformStage from './WorkflowTransformStage.svelte';

describe('Workflow Stage Components', () => {
  let target: HTMLElement;
  let instance: Record<string, unknown> | null = null;

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
  });

  afterEach(() => {
    if (instance) {
      unmount(instance);
      instance = null;
    }
    if (target && target.parentNode) {
      target.parentNode.removeChild(target);
    }
  });

  it('renders WorkflowTriggerStage and toggles schedule fields', async () => {
    const onchange = vi.fn();
    instance = mount(WorkflowTriggerStage, {
      target,
      props: {
        triggerType: 'schedule',
        triggerEventType: '*',
        scheduleInterval: '5m',
        scheduleCron: '*/5 * * * *',
        options: [
          { value: 'telegram', label: 'Telegram' },
          { value: 'schedule', label: 'Schedule' }
        ],
        onchange
      }
    });

    expect(target.textContent).toContain('Trigger Event Ingestion');
    expect(target.textContent).toContain('WHEN');
    expect(target.textContent).toContain('Tick Interval');
  });

  it('renders WorkflowActionStage with credentials', () => {
    const onIntegrationChange = vi.fn();
    const onTypeChange = vi.fn();

    instance = mount(WorkflowActionStage, {
      target,
      props: {
        actionIntegration: 'discord',
        actionType: 'send_message',
        actionCredentialId: 'cred-1',
        actionConfigRaw: '{"channel":"general"}',
        actionSchema: null,
        maxRetries: 3,
        integrations: [{ value: 'discord', label: 'Discord' }],
        actionTypes: [{ value: 'send_message', label: 'Send Message' }],
        credentials: [
          {
            id: 'cred-1',
            name: 'Discord Bot',
            integration: 'discord',
            key_id: 'k1',
            masked_fields: {},
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString()
          }
        ],
        onIntegrationChange,
        onTypeChange
      }
    });

    expect(target.textContent).toContain('Action Execution');
    expect(target.textContent).toContain('THEN');
    expect(target.textContent).toContain('Max Retries');
  });

  it('renders WorkflowConditionStage', () => {
    instance = mount(WorkflowConditionStage, {
      target,
      props: {
        conditionField: 'payload.amount',
        conditionOp: 'greater_than',
        conditionVal: '100',
        operators: [{ value: 'greater_than', label: 'Greater Than (>)' }]
      }
    });

    expect(target.textContent).toContain('Evaluation Conditions');
    expect(target.textContent).toContain('IF');
    expect(target.textContent).toContain('Payload Field Path');
  });

  it('renders WorkflowTransformStage', () => {
    instance = mount(WorkflowTransformStage, {
      target,
      props: {
        transformType: 'template',
        transformTemplate: 'Hello {{name}}',
        options: [
          { value: 'passthrough', label: 'Passthrough' },
          { value: 'template', label: 'Template' }
        ]
      }
    });

    expect(target.textContent).toContain('Deterministic Data Formatting');
    expect(target.textContent).toContain('TRANSFORM');
    expect(target.textContent).toContain('Template Expression');
  });
});
