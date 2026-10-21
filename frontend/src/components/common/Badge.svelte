<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    variant?:
      | 'neutral'
      | 'success'
      | 'failed'
      | 'danger'
      | 'warning'
      | 'running'
      | 'pending'
      | 'skipped'
      | 'stage-when'
      | 'stage-if'
      | 'stage-transform'
      | 'stage-then';
    dot?: boolean;
    size?: 'sm' | 'md';
    class?: string;
    children?: Snippet;
  }

  let {
    variant = 'neutral',
    dot = false,
    size = 'sm',
    class: className = '',
    children
  }: Props = $props();
</script>

<span class="tech-badge {variant} size-{size} {className}">
  {#if dot}
    <span class="badge-dot"></span>
  {/if}
  {#if children}
    {@render children()}
  {/if}
</span>

<style>
  .tech-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-family: var(--font-mono);
    text-transform: uppercase;
    font-weight: 600;
    letter-spacing: 0.04em;
    border-radius: var(--radius-xs);
    white-space: nowrap;
    border: 1px solid transparent;
  }

  .size-sm {
    font-size: 10px;
    padding: 1px 6px;
    line-height: 1.4;
  }

  .size-md {
    font-size: 11px;
    padding: 3px 8px;
    line-height: 1.4;
  }

  /* Variants */
  .neutral {
    background: var(--bg-surface-elevated);
    color: var(--text-secondary);
    border-color: var(--border-subtle);
  }

  .success {
    background: var(--accent-emerald-dim);
    color: var(--accent-emerald);
    border-color: var(--accent-emerald-border);
  }

  .failed, .danger {
    background: var(--accent-crimson-dim);
    color: var(--accent-crimson);
    border-color: var(--accent-crimson-border);
  }

  .warning, .skipped, .retrying {
    background: var(--accent-amber-dim);
    color: var(--accent-amber);
    border-color: var(--accent-amber-border);
  }

  .running {
    background: var(--accent-cyan-dim);
    color: var(--accent-cyan);
    border-color: var(--accent-cyan-border);
  }

  .pending {
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-muted);
    border-color: var(--border-subtle);
  }

  .stage-when {
    background: rgba(0, 210, 255, 0.15);
    color: var(--accent-cyan);
    border-color: var(--accent-cyan-border);
    font-size: 9px;
  }

  .stage-if {
    background: rgba(245, 158, 11, 0.15);
    color: var(--accent-amber);
    border-color: var(--accent-amber-border);
    font-size: 9px;
  }

  .stage-transform {
    background: rgba(168, 85, 247, 0.15);
    color: var(--accent-purple);
    border-color: var(--accent-purple-border);
    font-size: 9px;
  }

  .stage-then {
    background: rgba(0, 229, 153, 0.15);
    color: var(--accent-emerald);
    border-color: var(--accent-emerald-border);
    font-size: 9px;
  }

  .badge-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: currentColor;
    display: inline-block;
  }
</style>
