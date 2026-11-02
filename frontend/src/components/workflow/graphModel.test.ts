import { describe, it, expect } from 'vitest';
import {
  createNode,
  detectCycles,
  validateConnection,
  validateWorkflowDraft,
  workflowToGraph,
  graphToWorkflow,
  exportWorkflowJson,
  importWorkflowJson,
  type VisualEdge,
  type WorkflowDraft
} from './graphModel';
import type { Workflow } from '../../api/types';

describe('Workflow Graph Model & Validation', () => {
  it('creates typed nodes with correct input and output ports', () => {
    const trigger = createNode('trigger', 30, 160, { triggerType: 'telegram' });
    expect(trigger.type).toBe('trigger');
    expect(trigger.x).toBe(30);
    expect(trigger.inputs.length).toBe(0);
    expect(trigger.outputs.length).toBe(1);

    const condition = createNode('condition', 320, 160, { field: 'payload.text' });
    expect(condition.type).toBe('condition');
    expect(condition.inputs.length).toBe(1);
    expect(condition.outputs.length).toBe(2); // pass, fail

    const branch = createNode('branch', 610, 160);
    expect(branch.type).toBe('branch');
    expect(branch.outputs.length).toBe(2); // matched, default

    const loop = createNode('loop', 610, 160);
    expect(loop.type).toBe('loop');
    expect(loop.outputs.length).toBe(2); // body, after

    const delay = createNode('delay', 610, 160, { duration: '10s' });
    expect(delay.type).toBe('delay');
    expect(delay.data.duration).toBe('10s');

    const parallel = createNode('parallel', 610, 160, { concurrency: 6 });
    expect(parallel.type).toBe('parallel');
    expect(parallel.data.concurrency).toBe(6);
  });

  it('detects cycles and circular references accurately', () => {
    const nodeA = createNode('action', 100, 100);
    const nodeB = createNode('action', 300, 100);
    const nodeC = createNode('action', 500, 100);

    const edges: VisualEdge[] = [
      { id: 'e1', fromNode: nodeA.id, fromPort: 'out', toNode: nodeB.id, toPort: 'in' },
      { id: 'e2', fromNode: nodeB.id, fromPort: 'out', toNode: nodeC.id, toPort: 'in' }
    ];

    // Linear chain has no cycle
    const noCycle = detectCycles([nodeA, nodeB, nodeC], edges);
    expect(noCycle.hasCycle).toBe(false);

    // Adding edge from nodeC back to nodeA creates a cycle
    const candidateEdge: VisualEdge = {
      id: 'e3_cycle',
      fromNode: nodeC.id,
      fromPort: 'out',
      toNode: nodeA.id,
      toPort: 'in'
    };
    const cycleDetected = detectCycles([nodeA, nodeB, nodeC], edges, candidateEdge);
    expect(cycleDetected.hasCycle).toBe(true);
    expect(cycleDetected.message).toContain('Cycle detected');
  });

  it('validates connection rules correctly', () => {
    const trigger = createNode('trigger', 30, 160);
    const actionA = createNode('action', 300, 160);
    const actionB = createNode('action', 600, 160);

    // Cannot connect to itself
    const selfConn = validateConnection(actionA, 'out', actionA, 'in', []);
    expect(selfConn.valid).toBe(false);
    expect(selfConn.error).toContain('Cannot connect a node to itself');

    // Cannot connect to trigger (triggers cannot have inputs)
    const triggerConn = validateConnection(actionA, 'out', trigger, 'in', []);
    expect(triggerConn.valid).toBe(false);
    expect(triggerConn.error).toContain('Trigger nodes cannot have incoming connections');

    // Valid forward connection
    const validConn = validateConnection(trigger, 'out', actionA, 'in', []);
    expect(validConn.valid).toBe(true);

    // Existing input port cannot receive a second connection
    const existingEdges: VisualEdge[] = [
      { id: 'e1', fromNode: trigger.id, fromPort: 'out', toNode: actionA.id, toPort: 'in' }
    ];
    const duplicateInput = validateConnection(actionB, 'out', actionA, 'in', existingEdges);
    expect(duplicateInput.valid).toBe(false);
    expect(duplicateInput.error).toContain('already has an incoming connection');
  });

  it('validates draft requirements and produces readable error messages', () => {
    const emptyDraft: WorkflowDraft = {
      name: '',
      description: '',
      enabled: true,
      max_retries: 3,
      nodes: [],
      edges: []
    };

    const res1 = validateWorkflowDraft(emptyDraft);
    expect(res1.valid).toBe(false);
    expect(res1.errors.some((e) => e.field === 'name')).toBe(true);
    expect(res1.errors.some((e) => e.message.includes('Trigger node'))).toBe(true);

    // Valid draft
    const validDraft = workflowToGraph({
      name: 'Valid Pipeline',
      trigger: { type: 'telegram' },
      actions: [{ integration: 'discord', action_type: 'send_message', config: { content: 'hi' } }]
    });

    const res2 = validateWorkflowDraft(validDraft);
    expect(res2.valid).toBe(true);
    expect(res2.errors.length).toBe(0);
  });

  it('converts workflow to visual graph starting from left end (x=30) and back to workflow', () => {
    const originalWorkflow: Workflow = {
      id: 'wf-test-1',
      name: 'Incident Alert Flow',
      description: 'Notify Discord on alert',
      enabled: true,
      max_retries: 4,
      trigger: {
        type: 'webhook',
        event_types: ['alert.incident'],
        config: { secret: 'abc' }
      },
      conditions: {
        field: 'payload.severity',
        operator: 'equals',
        value: 'CRITICAL'
      },
      transformations: {
        type: 'template',
        template: 'Alert: {{payload.title}}'
      },
      actions: [
        {
          id: 'step_1',
          name: 'Discord Post',
          type: 'action',
          integration: 'discord',
          action_type: 'send_message',
          config: { content: 'hello' },
          on_error: 'continue',
          timeout_seconds: 15,
          max_retries: 2
        },
        {
          id: 'step_delay',
          type: 'delay',
          duration: '3m'
        }
      ]
    };

    // Convert to Graph
    const graph = workflowToGraph(originalWorkflow);
    expect(graph.name).toBe('Incident Alert Flow');
    expect(graph.nodes.length).toBe(5); // trigger + condition + transformation + 2 actions

    // Verify left end start position
    const triggerNode = graph.nodes.find((n) => n.type === 'trigger');
    expect(triggerNode?.x).toBe(30);

    // Verify nodes are placed left to right
    for (let i = 1; i < graph.nodes.length; i++) {
      expect(graph.nodes[i].x).toBeGreaterThan(graph.nodes[i - 1].x);
    }

    // Convert back to workflow
    const restored = graphToWorkflow(graph);
    expect(restored.name).toBe('Incident Alert Flow');
    expect(restored.trigger.type).toBe('webhook');
    expect(restored.conditions?.field).toBe('payload.severity');
    expect(restored.transformations?.type).toBe('template');
    expect(restored.actions.length).toBe(2);
    expect(restored.actions[0].integration).toBe('discord');
    expect(restored.actions[1].type).toBe('delay');
    expect(restored.actions[1].duration).toBe('3m');
  });

  it('exports and imports JSON cleanly with error handling', () => {
    const draft = workflowToGraph({
      name: 'Exportable Pipeline',
      trigger: { type: 'schedule', config: { interval: '10m' } },
      actions: [{ integration: 'telegram', action_type: 'send_message', config: { text: 'tick' } }]
    });

    const jsonStr = exportWorkflowJson(draft);
    expect(jsonStr).toContain('Exportable Pipeline');
    expect(jsonStr).toContain('schedule');

    const importRes = importWorkflowJson(jsonStr);
    expect(importRes.draft).toBeDefined();
    expect(importRes.draft?.name).toBe('Exportable Pipeline');
    expect(importRes.error).toBeUndefined();

    // Invalid JSON
    const badRes = importWorkflowJson('{ not json }');
    expect(badRes.error).toBeDefined();
    expect(badRes.draft).toBeUndefined();
  });
});
