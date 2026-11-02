<script lang="ts">
  import type { VisualNode, VisualEdge, NodeType } from './graphModel';
  import { validateConnection } from './graphModel';
  import Badge from '../common/Badge.svelte';
  import { notifyWarning } from '../../api/stores';

  interface Props {
    nodes: VisualNode[];
    edges: VisualEdge[];
    selectedNodeId: string | null;
    onselectnode: (nodeId: string | null) => void;
    onaddnode: (type: NodeType) => void;
    ondeletenode: (nodeId: string) => void;
    onaddedge: (edge: VisualEdge) => void;
    ondeleteedge: (edgeId: string) => void;
    onupdatenodepos: (nodeId: string, x: number, y: number) => void;
  }

  let {
    nodes = [],
    edges = [],
    selectedNodeId = null,
    onselectnode,
    onaddnode,
    ondeletenode,
    onaddedge,
    ondeleteedge,
    onupdatenodepos
  }: Props = $props();

  // State for interactive edge creation
  let connectingFrom = $state<{ nodeId: string; portId: string; x: number; y: number } | null>(null);
  let mousePos = $state({ x: 0, y: 0 });

  // State for dragging nodes
  let draggingNode = $state<{ id: string; startMouseX: number; startMouseY: number; startNodeX: number; startNodeY: number } | null>(null);

  // SVG dimensions derived from furthest node
  let canvasWidth = $derived.by(() => {
    let maxX = 1200;
    for (const n of nodes) {
      if (n.x + 360 > maxX) maxX = n.x + 360;
    }
    return maxX;
  });

  let canvasHeight = $derived.by(() => {
    let maxY = 700;
    for (const n of nodes) {
      if (n.y + 260 > maxY) maxY = n.y + 260;
    }
    return maxY;
  });

  function getPortPosition(node: VisualNode, portId: string, isOutput: boolean): { x: number; y: number } {
    const nodeWidth = 240;
    const nodeHeaderHeight = 44;
    const x = isOutput ? node.x + nodeWidth : node.x;

    // Approximate vertical port positions
    const ports = isOutput ? node.outputs : node.inputs;
    const idx = ports.findIndex((p) => p.id === portId);
    const offset = idx >= 0 ? 30 * (idx + 1) : 30;
    const y = node.y + nodeHeaderHeight + offset;

    return { x, y };
  }

  function handleMouseDownCanvas(e: MouseEvent) {
    if ((e.target as HTMLElement).closest('.node-card') === null) {
      onselectnode(null);
    }
    if (connectingFrom) {
      connectingFrom = null;
    }
  }

  function handleMouseMove(e: MouseEvent) {
    const canvas = (e.currentTarget as HTMLElement).getBoundingClientRect();
    mousePos = {
      x: e.clientX - canvas.left + (e.currentTarget as HTMLElement).scrollLeft,
      y: e.clientY - canvas.top + (e.currentTarget as HTMLElement).scrollTop
    };

    if (draggingNode) {
      const dx = e.clientX - draggingNode.startMouseX;
      const dy = e.clientY - draggingNode.startMouseY;
      const newX = Math.max(20, draggingNode.startNodeX + dx);
      const newY = Math.max(20, draggingNode.startNodeY + dy);
      onupdatenodepos(draggingNode.id, newX, newY);
    }
  }

  function handleMouseUp() {
    if (draggingNode) {
      draggingNode = null;
    }
  }

  function handleStartDragNode(node: VisualNode, e: MouseEvent) {
    if ((e.target as HTMLElement).closest('.port-dot') || (e.target as HTMLElement).closest('button')) {
      return;
    }
    onselectnode(node.id);
    draggingNode = {
      id: node.id,
      startMouseX: e.clientX,
      startMouseY: e.clientY,
      startNodeX: node.x,
      startNodeY: node.y
    };
  }

  function handleStartConnect(node: VisualNode, portId: string, e: MouseEvent) {
    e.stopPropagation();
    const pos = getPortPosition(node, portId, true);
    connectingFrom = {
      nodeId: node.id,
      portId,
      x: pos.x,
      y: pos.y
    };
  }

  function handleCompleteConnect(targetNode: VisualNode, targetPortId: string, e: MouseEvent) {
    e.stopPropagation();
    if (!connectingFrom) return;

    const sourceNode = nodes.find((n) => n.id === connectingFrom!.nodeId);
    if (!sourceNode) {
      connectingFrom = null;
      return;
    }

    const validation = validateConnection(
      sourceNode,
      connectingFrom.portId,
      targetNode,
      targetPortId,
      edges
    );

    if (!validation.valid) {
      notifyWarning(validation.error || 'Connection invalid.');
      connectingFrom = null;
      return;
    }

    const newEdge: VisualEdge = {
      id: `edge_${sourceNode.id}_${connectingFrom.portId}_to_${targetNode.id}_${targetPortId}`,
      fromNode: sourceNode.id,
      fromPort: connectingFrom.portId,
      toNode: targetNode.id,
      toPort: targetPortId
    };

    onaddedge(newEdge);
    connectingFrom = null;
  }

  function getNodeBadge(type: NodeType): { label: string; variant: any } {
    switch (type) {
      case 'trigger':
        return { label: 'WHEN TRIGGER', variant: 'stage-when' };
      case 'condition':
        return { label: 'IF CONDITION', variant: 'stage-if' };
      case 'transformation':
        return { label: 'TRANSFORM', variant: 'stage-transform' };
      case 'action':
        return { label: 'THEN ACTION', variant: 'stage-then' };
      case 'branch':
        return { label: 'BRANCH', variant: 'warning' };
      case 'loop':
        return { label: 'FOR EACH', variant: 'neutral' };
      case 'delay':
        return { label: 'DELAY', variant: 'pending' };
      case 'parallel':
        return { label: 'PARALLEL', variant: 'running' };
    }
  }

  function getNodeSummary(node: VisualNode): string {
    switch (node.type) {
      case 'trigger':
        return `${node.data.triggerType || 'telegram'} (${Array.isArray(node.data.eventTypes) ? node.data.eventTypes[0] || '*' : '*'})`;
      case 'condition':
        return `${node.data.field || 'field'} ${node.data.operator || 'equals'} "${node.data.value ?? ''}"`;
      case 'transformation':
        return `${node.data.transformType || 'passthrough'}`;
      case 'action':
        return `${node.data.integration || 'discord'} : ${node.data.action_type || 'send_message'}`;
      case 'branch':
        return `IF ${node.data.field || 'type'} == "${node.data.value || 'alert'}"`;
      case 'loop':
        return `items in ${node.data.items || 'payload.items'}`;
      case 'delay':
        return `pause for ${node.data.duration || '5m'}`;
      case 'parallel':
        return `concurrency limit: ${node.data.concurrency || 4}`;
    }
  }

  function createBezierPath(x1: number, y1: number, x2: number, y2: number): string {
    const dx = Math.max(40, Math.abs(x2 - x1) * 0.5);
    return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="canvas-viewport"
  onmousedown={handleMouseDownCanvas}
  onmousemove={handleMouseMove}
  onmouseup={handleMouseUp}
>
  <!-- Toolbar: Step Palette -->
  <div class="canvas-palette">
    <span class="palette-title">ADD STEP:</span>
    <button class="palette-btn action" onclick={() => onaddnode('action')} title="Add Action Step">
      + Action
    </button>
    <button class="palette-btn condition" onclick={() => onaddnode('condition')} title="Add Filter Condition">
      + Condition
    </button>
    <button class="palette-btn transform" onclick={() => onaddnode('transformation')} title="Add Payload Transformation">
      + Transform
    </button>
    <button class="palette-btn branch" onclick={() => onaddnode('branch')} title="Add Conditional Fork">
      + Branch
    </button>
    <button class="palette-btn loop" onclick={() => onaddnode('loop')} title="Add For Each Loop">
      + Loop
    </button>
    <button class="palette-btn delay" onclick={() => onaddnode('delay')} title="Add Execution Delay">
      + Delay
    </button>
    <button class="palette-btn parallel" onclick={() => onaddnode('parallel')} title="Add Parallel Fan-Out">
      + Parallel
    </button>
  </div>

  <div
    class="canvas-space"
    style="width: {canvasWidth}px; height: {canvasHeight}px;"
  >
    <!-- SVG Connection Lines -->
    <svg class="connections-svg" width={canvasWidth} height={canvasHeight}>
      <defs>
        <marker
          id="arrow"
          viewBox="0 0 10 10"
          refX="6"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M 0 1 L 8 5 L 0 9 z" fill="#4a5568" />
        </marker>
        <marker
          id="arrow-active"
          viewBox="0 0 10 10"
          refX="6"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M 0 1 L 8 5 L 0 9 z" fill="#60a5fa" />
        </marker>
      </defs>

      <!-- Existing Edges -->
      {#each edges as edge (edge.id)}
        {@const fromNode = nodes.find((n) => n.id === edge.fromNode)}
        {@const toNode = nodes.find((n) => n.id === edge.toNode)}
        {#if fromNode && toNode}
          {@const p1 = getPortPosition(fromNode, edge.fromPort, true)}
          {@const p2 = getPortPosition(toNode, edge.toPort, false)}
          {@const pathD = createBezierPath(p1.x, p1.y, p2.x, p2.y)}
          <g class="edge-group">
            <path
              d={pathD}
              class="edge-path-bg"
              onclick={() => ondeleteedge(edge.id)}
            />
            <path
              d={pathD}
              class="edge-path"
              marker-end="url(#arrow)"
            />
          </g>
        {/if}
      {/each}

      <!-- In-Flight Connecting Wire -->
      {#if connectingFrom}
        {@const livePath = createBezierPath(connectingFrom.x, connectingFrom.y, mousePos.x, mousePos.y)}
        <path
          d={livePath}
          class="edge-path-connecting"
          marker-end="url(#arrow-active)"
        />
      {/if}
    </svg>

    <!-- Node Cards (Left-to-Right Flow) -->
    {#each nodes as node (node.id)}
      {@const badge = getNodeBadge(node.type)}
      {@const isSelected = selectedNodeId === node.id}
      <div
        class="node-card {node.type} {isSelected ? 'selected' : ''}"
        style="left: {node.x}px; top: {node.y}px;"
        onmousedown={(e) => handleStartDragNode(node, e)}
      >
        <!-- Header -->
        <div class="node-header">
          <Badge variant={badge.variant}>{badge.label}</Badge>
          <div class="header-buttons">
            {#if node.type !== 'trigger'}
              <button
                class="node-action-btn"
                title="Delete Step"
                onclick={(e) => {
                  e.stopPropagation();
                  ondeletenode(node.id);
                }}
              >
                ×
              </button>
            {/if}
          </div>
        </div>

        <!-- Body Summary -->
        <div class="node-body">
          <div class="node-title">{node.name}</div>
          <div class="node-summary" title={getNodeSummary(node)}>
            {getNodeSummary(node)}
          </div>
        </div>

        <!-- Input Ports (Left) -->
        {#if node.inputs && node.inputs.length > 0}
          <div class="ports-column inputs">
            {#each node.inputs as inp (inp.id)}
              <div
                class="port-dot input-dot"
                title={`Input: ${inp.name} (${inp.dataType})`}
                onmouseup={(e) => handleCompleteConnect(node, inp.id, e)}
              >
                <span class="port-label">{inp.name}</span>
              </div>
            {/each}
          </div>
        {/if}

        <!-- Output Ports (Right) -->
        {#if node.outputs && node.outputs.length > 0}
          <div class="ports-column outputs">
            {#each node.outputs as out (out.id)}
              <div
                class="port-dot output-dot"
                title={`Output: ${out.name} (drag to connect)`}
                onmousedown={(e) => handleStartConnect(node, out.id, e)}
              >
                <span class="port-label">{out.name}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  .canvas-viewport {
    position: relative;
    width: 100%;
    height: 480px;
    background: #0b0d10;
    background-image: radial-gradient(#1f242c 1px, transparent 1px);
    background-size: 20px 20px;
    border: 1px solid var(--border-default, #272a30);
    border-radius: var(--radius-md, 8px);
    overflow: auto;
    user-select: none;
    box-shadow: inset 0 0 20px rgba(0, 0, 0, 0.4);
  }

  .canvas-palette {
    position: sticky;
    top: 12px;
    left: 12px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    background: rgba(22, 25, 31, 0.9);
    backdrop-filter: blur(8px);
    border: 1px solid var(--border-default, #272a30);
    border-radius: 6px;
    z-index: 20;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }

  .palette-title {
    font-family: var(--font-mono, monospace);
    font-size: 10px;
    font-weight: 700;
    color: var(--text-muted, #71767b);
    letter-spacing: 0.05em;
  }

  .palette-btn {
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    font-weight: 600;
    padding: 3px 8px;
    background: var(--bg-surface, #121417);
    border: 1px solid var(--border-subtle, #30363d);
    border-radius: 4px;
    color: var(--text-primary, #e6edf3);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .palette-btn:hover {
    background: var(--bg-surface-hover, #20242b);
    border-color: var(--border-default, #4b5360);
  }
  .palette-btn.action:hover {
    border-color: #3b82f6;
    color: #60a5fa;
  }
  .palette-btn.condition:hover {
    border-color: #f59e0b;
    color: #fbbf24;
  }
  .palette-btn.transform:hover {
    border-color: #8b5cf6;
    color: #a78bfa;
  }
  .palette-btn.branch:hover {
    border-color: #ec4899;
    color: #f472b6;
  }
  .palette-btn.delay:hover {
    border-color: #10b981;
    color: #34d399;
  }

  .canvas-space {
    position: relative;
    /* Aligns starting strictly from the left end */
    padding-left: 30px;
    padding-top: 60px;
  }

  .connections-svg {
    position: absolute;
    top: 0;
    left: 0;
    pointer-events: none;
    z-index: 5;
  }

  .edge-path {
    fill: none;
    stroke: #384252;
    stroke-width: 2.2px;
    stroke-linecap: round;
  }

  .edge-path-bg {
    fill: none;
    stroke: transparent;
    stroke-width: 14px;
    cursor: pointer;
    pointer-events: stroke;
  }

  .edge-group:hover .edge-path {
    stroke: #f87171;
    stroke-width: 3px;
  }

  .edge-path-connecting {
    fill: none;
    stroke: #60a5fa;
    stroke-width: 2.5px;
    stroke-dasharray: 4 3;
    animation: dash 1s linear infinite;
  }

  @keyframes dash {
    to {
      stroke-dashoffset: -14;
    }
  }

  /* Node Card Styles */
  .node-card {
    position: absolute;
    width: 240px;
    background: var(--bg-surface-elevated, #16181e);
    border: 1px solid var(--border-default, #272a30);
    border-radius: var(--radius-md, 8px);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
    cursor: grab;
    z-index: 10;
    transition: box-shadow 0.15s ease, border-color 0.15s ease;
  }

  .node-card:active {
    cursor: grabbing;
  }

  .node-card.selected {
    border-color: var(--accent-blue, #3b82f6);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.4), 0 8px 24px rgba(0, 0, 0, 0.6);
  }

  .node-card.trigger {
    border-top: 3px solid #10b981;
  }
  .node-card.condition {
    border-top: 3px solid #f59e0b;
  }
  .node-card.transformation {
    border-top: 3px solid #8b5cf6;
  }
  .node-card.action {
    border-top: 3px solid #3b82f6;
  }
  .node-card.branch {
    border-top: 3px solid #ec4899;
  }
  .node-card.loop {
    border-top: 3px solid #6366f1;
  }
  .node-card.delay {
    border-top: 3px solid #14b8a6;
  }
  .node-card.parallel {
    border-top: 3px solid #06b6d4;
  }

  .node-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    background: var(--bg-surface, #121417);
    border-bottom: 1px solid var(--border-subtle, #21252d);
    border-top-left-radius: 6px;
    border-top-right-radius: 6px;
  }

  .node-action-btn {
    background: transparent;
    border: none;
    color: var(--text-muted, #71767b);
    font-size: 16px;
    font-weight: 700;
    cursor: pointer;
    line-height: 1;
    padding: 2px 4px;
    border-radius: 3px;
  }
  .node-action-btn:hover {
    color: #ef4444;
    background: rgba(239, 68, 68, 0.15);
  }

  .node-body {
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .node-title {
    font-size: 12.5px;
    font-weight: 600;
    color: var(--text-primary, #e6edf3);
  }

  .node-summary {
    font-family: var(--font-mono, monospace);
    font-size: 10.5px;
    color: var(--text-secondary, #9aa0a6);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Ports Column */
  .ports-column {
    position: absolute;
    top: 44px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    z-index: 15;
  }

  .ports-column.inputs {
    left: -7px;
  }

  .ports-column.outputs {
    right: -7px;
  }

  .port-dot {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: #1e232b;
    border: 2px solid #4a5568;
    cursor: crosshair;
    position: relative;
    transition: transform 0.15s ease, border-color 0.15s ease;
  }

  .port-dot:hover {
    transform: scale(1.3);
    border-color: #60a5fa;
    background: #3b82f6;
  }

  .port-label {
    position: absolute;
    top: -2px;
    font-family: var(--font-mono, monospace);
    font-size: 9.5px;
    color: var(--text-muted, #71767b);
    white-space: nowrap;
    pointer-events: none;
  }

  .inputs .port-label {
    left: 18px;
  }

  .outputs .port-label {
    right: 18px;
  }
</style>
