<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../icons/Icon.svelte';

  interface Props {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'success';
    size?: 'sm' | 'md' | 'lg';
    disabled?: boolean;
    loading?: boolean;
    type?: 'button' | 'submit' | 'reset';
    icon?: any;
    class?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  }

  let {
    variant = 'secondary',
    size = 'md',
    disabled = false,
    loading = false,
    type = 'button',
    icon,
    class: className = '',
    onclick,
    children
  }: Props = $props();
</script>

<button
  {type}
  disabled={disabled || loading}
  class="tech-btn {variant} size-{size} {className}"
  {onclick}
>
  {#if loading}
    <span class="btn-spinner"></span>
  {:else if icon}
    <Icon name={icon} size={size === 'sm' ? 13 : 15} />
  {/if}
  {#if children}
    <span class="btn-text">
      {@render children()}
    </span>
  {/if}
</button>

<style>
  .tech-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    font-family: var(--font-sans);
    font-weight: 500;
    letter-spacing: -0.01em;
    cursor: pointer;
    border-radius: var(--radius-sm);
    border: 1px solid transparent;
    transition: all 0.12s ease-in-out;
    white-space: nowrap;
    user-select: none;
  }

  .tech-btn:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  /* Sizes */
  .size-sm {
    height: 28px;
    padding: 0 10px;
    font-size: 11.5px;
  }

  .size-md {
    height: 34px;
    padding: 0 14px;
    font-size: 12.5px;
  }

  .size-lg {
    height: 40px;
    padding: 0 18px;
    font-size: 13.5px;
  }

  /* Variants */
  .primary {
    background: var(--accent-emerald);
    color: #050608;
    font-weight: 600;
    border-color: var(--accent-emerald);
  }
  .primary:hover:not(:disabled) {
    background: #00ffaa;
    border-color: #00ffaa;
    box-shadow: 0 0 12px rgba(0, 229, 153, 0.3);
  }

  .secondary {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
    border-color: var(--border-default);
  }
  .secondary:hover:not(:disabled) {
    background: var(--bg-surface-hover);
    border-color: var(--border-strong);
    color: #fff;
  }

  .ghost {
    background: transparent;
    color: var(--text-secondary);
    border-color: transparent;
  }
  .ghost:hover:not(:disabled) {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
    border-color: var(--border-subtle);
  }

  .danger {
    background: var(--accent-crimson-dim);
    color: var(--accent-crimson);
    border-color: var(--accent-crimson-border);
  }
  .danger:hover:not(:disabled) {
    background: rgba(255, 51, 85, 0.22);
    border-color: var(--accent-crimson);
    color: #fff;
  }

  .success {
    background: var(--accent-emerald-dim);
    color: var(--accent-emerald);
    border-color: var(--accent-emerald-border);
  }
  .success:hover:not(:disabled) {
    background: rgba(0, 229, 153, 0.22);
    border-color: var(--accent-emerald);
  }

  .btn-spinner {
    width: 12px;
    height: 12px;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
