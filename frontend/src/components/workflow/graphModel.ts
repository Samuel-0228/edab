import type {
  Workflow,
  WorkflowCreateInput,
  TriggerConfig,
  ConditionRule,
  TransformationConfig,
  WorkflowActionStep,
  BranchCase
} from '../../api/types';

export type NodeType =
  'trigger' | 'condition' | 'transformation' | 'action' | 'branch' | 'loop' | 'delay' | 'parallel';

export interface Port {
  id: string;
  name: string;
  type: 'input' | 'output';
  dataType: 'event' | 'condition' | 'data' | 'boolean' | 'flow';
}

export interface VisualNode {
  id: string;
  type: NodeType;
  name: string;
  x: number;
  y: number;
  data: Record<string, any>;
  inputs: Port[];
  outputs: Port[];
  errors?: string[];
}

export interface VisualEdge {
  id: string;
  fromNode: string;
  fromPort: string;
  toNode: string;
  toPort: string;
}

export interface WorkflowDraft {
  name: string;
  description: string;
  enabled: boolean;
  max_retries: number;
  nodes: VisualNode[];
  edges: VisualEdge[];
}

export interface ValidationError {
  nodeId?: string;
  field?: string;
  message: string;
}

export function createNode(
  type: NodeType,
  x: number,
  y: number,
  initialData?: Record<string, any>
): VisualNode {
  const id = `node_${type}_${Math.random().toString(36).slice(2, 8)}`;

  switch (type) {
    case 'trigger':
      return {
        id,
        type: 'trigger',
        name: 'Trigger (Event)',
        x,
        y,
        data: {
          triggerType: initialData?.triggerType || 'telegram',
          eventTypes: initialData?.eventTypes || ['*'],
          scheduleInterval: initialData?.scheduleInterval || '5m',
          scheduleCron: initialData?.scheduleCron || '',
          ...initialData
        },
        inputs: [],
        outputs: [{ id: 'out', name: 'Event', type: 'output', dataType: 'event' }]
      };

    case 'condition':
      return {
        id,
        type: 'condition',
        name: 'Condition (Filter)',
        x,
        y,
        data: {
          field: initialData?.field || '',
          operator: initialData?.operator || 'equals',
          value: initialData?.value ?? '',
          ...initialData
        },
        inputs: [{ id: 'in', name: 'In', type: 'input', dataType: 'event' }],
        outputs: [
          { id: 'pass', name: 'Pass (True)', type: 'output', dataType: 'boolean' },
          { id: 'fail', name: 'Fail (False)', type: 'output', dataType: 'boolean' }
        ]
      };

    case 'transformation':
      return {
        id,
        type: 'transformation',
        name: 'Transformation',
        x,
        y,
        data: {
          transformType: initialData?.transformType || 'passthrough',
          template: initialData?.template || '',
          ...initialData
        },
        inputs: [{ id: 'in', name: 'In', type: 'input', dataType: 'data' }],
        outputs: [{ id: 'out', name: 'Transformed', type: 'output', dataType: 'data' }]
      };

    case 'action':
      return {
        id,
        type: 'action',
        name: initialData?.name || 'Action Step',
        x,
        y,
        data: {
          integration: initialData?.integration || 'discord',
          action_type: initialData?.action_type || 'send_message',
          credential_id: initialData?.credential_id || '',
          configRaw: initialData?.configRaw || '{\n  "content": "{{payload.text}}"\n}',
          on_error: initialData?.on_error || 'fail',
          timeout_seconds: initialData?.timeout_seconds || 30,
          max_retries: initialData?.max_retries || 3,
          ...initialData
        },
        inputs: [{ id: 'in', name: 'In', type: 'input', dataType: 'flow' }],
        outputs: [{ id: 'out', name: 'Next', type: 'output', dataType: 'flow' }]
      };

    case 'branch':
      return {
        id,
        type: 'branch',
        name: 'Branch (If / Else)',
        x,
        y,
        data: {
          field: initialData?.field || 'payload.type',
          operator: initialData?.operator || 'equals',
          value: initialData?.value ?? 'alert',
          ...initialData
        },
        inputs: [{ id: 'in', name: 'In', type: 'input', dataType: 'flow' }],
        outputs: [
          { id: 'case_match', name: 'Matched', type: 'output', dataType: 'flow' },
          { id: 'default', name: 'Default', type: 'output', dataType: 'flow' }
        ]
      };

    case 'loop':
      return {
        id,
        type: 'loop',
        name: 'Loop (For Each)',
        x,
        y,
        data: {
          items: initialData?.items || 'payload.items',
          item_var: initialData?.item_var || 'item',
          max_iterations: initialData?.max_iterations || 50,
          ...initialData
        },
        inputs: [{ id: 'in', name: 'In', type: 'input', dataType: 'flow' }],
        outputs: [
          { id: 'body', name: 'Each Item', type: 'output', dataType: 'flow' },
          { id: 'after', name: 'Complete', type: 'output', dataType: 'flow' }
        ]
      };

    case 'delay':
      return {
        id,
        type: 'delay',
        name: 'Delay (Wait)',
        x,
        y,
        data: {
          duration: initialData?.duration || '5m',
          ...initialData
        },
        inputs: [{ id: 'in', name: 'In', type: 'input', dataType: 'flow' }],
        outputs: [{ id: 'out', name: 'After Delay', type: 'output', dataType: 'flow' }]
      };

    case 'parallel':
      return {
        id,
        type: 'parallel',
        name: 'Parallel (Fan-Out)',
        x,
        y,
        data: {
          concurrency: initialData?.concurrency || 4,
          ...initialData
        },
        inputs: [{ id: 'in', name: 'In', type: 'input', dataType: 'flow' }],
        outputs: [
          { id: 'branch_a', name: 'Branch A', type: 'output', dataType: 'flow' },
          { id: 'branch_b', name: 'Branch B', type: 'output', dataType: 'flow' }
        ]
      };
  }
}

/**
 * Cycle detection on directed graph. Returns whether proposed or current edges create a cycle.
 */
export function detectCycles(
  nodes: VisualNode[],
  edges: VisualEdge[],
  candidateEdge?: VisualEdge
): { hasCycle: boolean; cyclePath?: string[]; message?: string } {
  const adj = new Map<string, string[]>();
  for (const n of nodes) {
    adj.set(n.id, []);
  }

  const allEdges = candidateEdge ? [...edges, candidateEdge] : edges;
  for (const e of allEdges) {
    const list = adj.get(e.fromNode);
    if (list) {
      list.push(e.toNode);
    }
  }

  const visited = new Set<string>();
  const inStack = new Set<string>();
  const path: string[] = [];

  function dfs(curr: string): boolean {
    visited.add(curr);
    inStack.add(curr);
    path.push(curr);

    const neighbors = adj.get(curr) || [];
    for (const next of neighbors) {
      if (!visited.has(next)) {
        if (dfs(next)) return true;
      } else if (inStack.has(next)) {
        path.push(next);
        return true;
      }
    }

    inStack.delete(curr);
    path.pop();
    return false;
  }

  for (const n of nodes) {
    if (!visited.has(n.id)) {
      if (dfs(n.id)) {
        return {
          hasCycle: true,
          cyclePath: [...path],
          message: `Cycle detected: node connection creates a circular loop (${path.join(' → ')})`
        };
      }
    }
  }

  return { hasCycle: false };
}

/**
 * Validates whether a connection can be established between two ports.
 */
export function validateConnection(
  fromNode: VisualNode,
  fromPort: string,
  toNode: VisualNode,
  toPort: string,
  existingEdges: VisualEdge[]
): { valid: boolean; error?: string } {
  if (fromNode.id === toNode.id) {
    return { valid: false, error: 'Cannot connect a node to itself.' };
  }

  if (toNode.type === 'trigger') {
    return { valid: false, error: 'Trigger nodes cannot have incoming connections.' };
  }

  // Prevent multiple edges into the same input port unless port is fan-in capable
  const alreadyConnectedInput = existingEdges.some(
    (e) => e.toNode === toNode.id && e.toPort === toPort
  );
  if (alreadyConnectedInput) {
    return { valid: false, error: `Port "${toPort}" already has an incoming connection.` };
  }

  // Check for duplicate edge
  const isDuplicate = existingEdges.some(
    (e) =>
      e.fromNode === fromNode.id &&
      e.fromPort === fromPort &&
      e.toNode === toNode.id &&
      e.toPort === toPort
  );
  if (isDuplicate) {
    return { valid: false, error: 'Connection already exists.' };
  }

  // Check cycle
  const proposedEdge: VisualEdge = {
    id: `temp_edge`,
    fromNode: fromNode.id,
    fromPort,
    toNode: toNode.id,
    toPort
  };
  const cycleResult = detectCycles([fromNode, toNode], existingEdges, proposedEdge);
  if (cycleResult.hasCycle) {
    return {
      valid: false,
      error: cycleResult.message || 'Cycle detected: this connection would cause an infinite loop.'
    };
  }

  return { valid: true };
}

/**
 * Comprehensive pre-publish validation of the workflow draft.
 */
export function validateWorkflowDraft(draft: WorkflowDraft): {
  valid: boolean;
  errors: ValidationError[];
} {
  const errors: ValidationError[] = [];

  if (!draft.name || !draft.name.trim()) {
    errors.push({ field: 'name', message: 'Automation name is required.' });
  }

  const triggerNodes = draft.nodes.filter((n) => n.type === 'trigger');
  if (triggerNodes.length === 0) {
    errors.push({ message: 'Pipeline must define a Trigger node.' });
  } else if (triggerNodes.length > 1) {
    errors.push({
      nodeId: triggerNodes[1].id,
      message: 'Only one root Trigger node is supported per workflow.'
    });
  } else {
    const tr = triggerNodes[0];
    if (!tr.data.triggerType) {
      errors.push({ nodeId: tr.id, message: 'Trigger type must be selected.' });
    }
  }

  const actionNodes = draft.nodes.filter(
    (n) =>
      n.type === 'action' ||
      n.type === 'branch' ||
      n.type === 'loop' ||
      n.type === 'delay' ||
      n.type === 'parallel'
  );
  if (actionNodes.length === 0) {
    errors.push({ message: 'Pipeline must contain at least one action or control flow step.' });
  }

  // Cycle check
  const cycleCheck = detectCycles(draft.nodes, draft.edges);
  if (cycleCheck.hasCycle) {
    errors.push({ message: cycleCheck.message || 'Graph contains circular cycles.' });
  }

  // Node-specific checks
  for (const node of draft.nodes) {
    switch (node.type) {
      case 'action':
        if (!node.data.integration) {
          errors.push({ nodeId: node.id, message: 'Action integration is required.' });
        }
        if (!node.data.action_type) {
          errors.push({ nodeId: node.id, message: 'Action type is required.' });
        }
        if (node.data.configRaw) {
          try {
            JSON.parse(node.data.configRaw);
          } catch {
            errors.push({ nodeId: node.id, message: 'Action configuration must be valid JSON.' });
          }
        }
        break;

      case 'condition':
        if (!node.data.field?.trim()) {
          errors.push({ nodeId: node.id, message: 'Condition field path is required.' });
        }
        if (!node.data.operator) {
          errors.push({ nodeId: node.id, message: 'Condition operator must be selected.' });
        }
        break;

      case 'branch':
        if (!node.data.field?.trim()) {
          errors.push({ nodeId: node.id, message: 'Branch condition field is required.' });
        }
        break;

      case 'loop':
        if (!node.data.items?.trim()) {
          errors.push({
            nodeId: node.id,
            message: 'Loop items expression (e.g. payload.items) is required.'
          });
        }
        break;

      case 'delay':
        if (
          !node.data.duration?.trim() &&
          (!node.data.duration_seconds || node.data.duration_seconds <= 0)
        ) {
          errors.push({ nodeId: node.id, message: 'Delay duration (e.g. 10s, 5m) is required.' });
        }
        break;

      case 'parallel':
        if (!node.data.concurrency || node.data.concurrency < 1) {
          errors.push({ nodeId: node.id, message: 'Parallel concurrency must be at least 1.' });
        }
        break;
    }
  }

  return {
    valid: errors.length === 0,
    errors
  };
}

/**
 * Converts a persisted Workflow into a Visual Graph Draft starting from the left end (x = 30).
 */
export function workflowToGraph(wf?: Partial<Workflow> | null): WorkflowDraft {
  const nodes: VisualNode[] = [];
  const edges: VisualEdge[] = [];

  const name = wf?.name || '';
  const description = wf?.description || '';
  const enabled = wf?.enabled ?? true;
  const max_retries = wf?.max_retries ?? 3;

  // Horizontal layout starting from the left end
  let currentX = 30;
  const defaultY = 160;

  // 1. Trigger Node (Left End: x = 30)
  const triggerType = wf?.trigger?.type || 'telegram';
  const eventTypes = wf?.trigger?.event_types || ['*'];
  const triggerConfig = (wf?.trigger?.config as Record<string, any>) || {};

  const triggerNode = createNode('trigger', currentX, defaultY, {
    triggerType,
    eventTypes,
    scheduleInterval: triggerConfig.interval || '5m',
    scheduleCron: triggerConfig.cron || ''
  });
  nodes.push(triggerNode);
  let lastNodeId = triggerNode.id;
  let lastPortId = 'out';

  // 2. Condition Node
  if (wf?.conditions && wf.conditions.field) {
    currentX += 290;
    const condNode = createNode('condition', currentX, defaultY, {
      field: wf.conditions.field,
      operator: wf.conditions.operator,
      value: wf.conditions.value
    });
    nodes.push(condNode);
    edges.push({
      id: `edge_${lastNodeId}_${condNode.id}`,
      fromNode: lastNodeId,
      fromPort: lastPortId,
      toNode: condNode.id,
      toPort: 'in'
    });
    lastNodeId = condNode.id;
    lastPortId = 'pass';
  }

  // 3. Transformation Node
  if (wf?.transformations && wf.transformations.type && wf.transformations.type !== 'passthrough') {
    currentX += 290;
    const transNode = createNode('transformation', currentX, defaultY, {
      transformType: wf.transformations.type,
      template: wf.transformations.template || ''
    });
    nodes.push(transNode);
    edges.push({
      id: `edge_${lastNodeId}_${transNode.id}`,
      fromNode: lastNodeId,
      fromPort: lastPortId,
      toNode: transNode.id,
      toPort: 'in'
    });
    lastNodeId = transNode.id;
    lastPortId = 'out';
  }

  // 4. Action Steps & Control Flow
  const actions = wf?.actions || [];
  if (actions.length === 0) {
    // Default placeholder action
    currentX += 290;
    const actionNode = createNode('action', currentX, defaultY, {
      integration: 'discord',
      action_type: 'send_message',
      configRaw: '{\n  "content": "{{payload.text}}"\n}'
    });
    nodes.push(actionNode);
    edges.push({
      id: `edge_${lastNodeId}_${actionNode.id}`,
      fromNode: lastNodeId,
      fromPort: lastPortId,
      toNode: actionNode.id,
      toPort: 'in'
    });
  } else {
    for (let i = 0; i < actions.length; i++) {
      const act = actions[i];
      currentX += 290;
      const stepType = (act.type || 'action') as string;
      let newNode: VisualNode;

      if (stepType === 'branch') {
        const firstCase = act.cases?.[0];
        const cond = firstCase?.when as ConditionRule | undefined;
        newNode = createNode('branch', currentX, defaultY, {
          field: cond?.field || 'payload.type',
          operator: cond?.operator || 'equals',
          value: cond?.value ?? 'alert',
          caseSteps: firstCase?.steps || [],
          defaultSteps: act.default || []
        });
      } else if (stepType === 'for_each') {
        newNode = createNode('loop', currentX, defaultY, {
          items: act.items || 'payload.items',
          item_var: act.item_var || 'item',
          max_iterations: act.max_iterations || 50,
          steps: act.steps || []
        });
      } else if (stepType === 'delay' || stepType === 'wait') {
        newNode = createNode('delay', currentX, defaultY, {
          duration: act.duration || (act.duration_seconds ? `${act.duration_seconds}s` : '5m'),
          duration_seconds: act.duration_seconds
        });
      } else if (stepType === 'parallel') {
        newNode = createNode('parallel', currentX, defaultY, {
          concurrency: act.concurrency || 4,
          steps: act.steps || []
        });
      } else {
        newNode = createNode('action', currentX, defaultY, {
          name: act.name || `Action Step ${i + 1}`,
          integration: act.integration || 'discord',
          action_type: act.action_type || 'send_message',
          credential_id: act.credential_id || '',
          configRaw: JSON.stringify(act.config || {}, null, 2),
          on_error: act.on_error || 'fail',
          timeout_seconds: act.timeout_seconds || 30,
          max_retries: act.max_retries ?? 3
        });
      }

      nodes.push(newNode);
      edges.push({
        id: `edge_${lastNodeId}_${newNode.id}`,
        fromNode: lastNodeId,
        fromPort: lastPortId,
        toNode: newNode.id,
        toPort: 'in'
      });
      lastNodeId = newNode.id;
      lastPortId = 'out';
    }
  }

  return {
    name,
    description,
    enabled,
    max_retries,
    nodes,
    edges
  };
}

/**
 * Converts a Visual Graph Draft into a persisted WorkflowCreateInput payload.
 */
export function graphToWorkflow(draft: WorkflowDraft): WorkflowCreateInput {
  const triggerNode = draft.nodes.find((n) => n.type === 'trigger') || draft.nodes[0];
  const triggerData = triggerNode?.data || {};

  let triggerConfig: Record<string, unknown> | undefined = undefined;
  if (triggerData.triggerType === 'schedule') {
    triggerConfig = {};
    if (triggerData.scheduleCron?.trim()) {
      triggerConfig.cron = triggerData.scheduleCron.trim();
    } else {
      triggerConfig.interval = triggerData.scheduleInterval?.trim() || '5m';
    }
  }

  const trigger: TriggerConfig = {
    type: triggerData.triggerType || 'telegram',
    event_types: Array.isArray(triggerData.eventTypes)
      ? triggerData.eventTypes
      : typeof triggerData.eventTypes === 'string'
        ? triggerData.eventTypes
            .split(',')
            .map((s: string) => s.trim())
            .filter(Boolean)
        : ['*'],
    config: triggerConfig
  };

  // Condition
  const conditionNode = draft.nodes.find((n) => n.type === 'condition');
  let conditions: ConditionRule | null = null;
  if (conditionNode && conditionNode.data.field?.trim()) {
    conditions = {
      field: conditionNode.data.field.trim(),
      operator: conditionNode.data.operator || 'equals',
      value: conditionNode.data.value ?? ''
    };
  }

  // Transformation
  const transformNode = draft.nodes.find((n) => n.type === 'transformation');
  let transformations: TransformationConfig | null = null;
  if (transformNode && transformNode.data.transformType !== 'passthrough') {
    transformations = {
      type: transformNode.data.transformType || 'passthrough',
      template: transformNode.data.template || ''
    };
  }

  // Action Steps (ordered by x position from left to right)
  const actionNodes = draft.nodes
    .filter(
      (n) =>
        n.type === 'action' ||
        n.type === 'branch' ||
        n.type === 'loop' ||
        n.type === 'delay' ||
        n.type === 'parallel'
    )
    .sort((a, b) => a.x - b.x);

  const actions: WorkflowActionStep[] = actionNodes.map((n, idx) => {
    const id = n.id || `step_${idx + 1}`;
    switch (n.type) {
      case 'branch': {
        const branchCases: BranchCase[] = [
          {
            when: {
              field: n.data.field || 'payload.type',
              operator: n.data.operator || 'equals',
              value: n.data.value ?? 'alert'
            },
            steps: n.data.caseSteps || []
          }
        ];
        return {
          id,
          name: n.name || 'Branch Step',
          type: 'branch',
          cases: branchCases,
          default: n.data.defaultSteps || []
        };
      }

      case 'loop':
        return {
          id,
          name: n.name || 'Loop Step',
          type: 'for_each',
          items: n.data.items || 'payload.items',
          item_var: n.data.item_var || 'item',
          max_iterations: Number(n.data.max_iterations) || 50,
          steps: n.data.steps || []
        };

      case 'delay': {
        const dur = n.data.duration?.trim() || '5m';
        return {
          id,
          name: n.name || 'Delay Step',
          type: 'delay',
          duration: dur
        };
      }

      case 'parallel':
        return {
          id,
          name: n.name || 'Parallel Step',
          type: 'parallel',
          concurrency: Number(n.data.concurrency) || 4,
          steps: n.data.steps || []
        };

      default: {
        let configObj: Record<string, unknown> = {};
        if (n.data.configRaw) {
          try {
            configObj = JSON.parse(n.data.configRaw);
          } catch {
            configObj = {};
          }
        }
        return {
          id,
          name: n.name || `Action Step ${idx + 1}`,
          type: 'action',
          integration: n.data.integration || 'discord',
          action_type: n.data.action_type || 'send_message',
          credential_id: n.data.credential_id || undefined,
          config: configObj,
          on_error: n.data.on_error || 'fail',
          timeout_seconds: Number(n.data.timeout_seconds) || 30,
          max_retries: Number(n.data.max_retries) || 3
        };
      }
    }
  });

  return {
    name: draft.name.trim(),
    description: draft.description?.trim() || undefined,
    enabled: draft.enabled,
    max_retries: Number(draft.max_retries) || 3,
    trigger,
    conditions,
    transformations,
    actions
  };
}

/**
 * Export workflow draft to formatted JSON string.
 */
export function exportWorkflowJson(draft: WorkflowDraft): string {
  const wf = graphToWorkflow(draft);
  return JSON.stringify(wf, null, 2);
}

/**
 * Import workflow from JSON string, validating structure.
 */
export function importWorkflowJson(jsonStr: string): { draft?: WorkflowDraft; error?: string } {
  try {
    const parsed = JSON.parse(jsonStr);
    if (!parsed || typeof parsed !== 'object') {
      return { error: 'Workflow JSON must be an object.' };
    }
    if (!parsed.name || typeof parsed.name !== 'string') {
      return { error: 'Workflow JSON missing required "name" property.' };
    }
    if (!parsed.trigger || typeof parsed.trigger !== 'object') {
      return { error: 'Workflow JSON missing required "trigger" configuration.' };
    }
    const draft = workflowToGraph(parsed);
    return { draft };
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : 'Invalid JSON format.';
    return { error: `Failed to parse workflow JSON: ${msg}` };
  }
}
