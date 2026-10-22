<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../icons/Icon.svelte';

  interface Props {
    open: boolean;
    title: string;
    subtitle?: string;
    maxWidth?: string;
    onclose: () => void;
    headerActions?: Snippet;
    children?: Snippet;
    footer?: Snippet;
  }

  let {
    open = $bindable(),
    title,
    subtitle,
    maxWidth = '640px',
    onclose,
    headerActions,
    children,
    footer
  }: Props = $props();

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && open) {
      onclose();
    }
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      onclose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="dialog-backdrop"
    onclick={handleBackdropClick}
    role="dialog"
    aria-modal="true"
    aria-labelledby="dialog-title"
    tabindex="-1"
  >
    <div class="dialog-panel" style="max-width: {maxWidth}">
      <div class="dialog-header">
        <div class="dialog-header-text">
          <h3 id="dialog-title" class="dialog-title">{title}</h3>
          {#if subtitle}
            <p class="dialog-subtitle">{subtitle}</p>
          {/if}
        </div>

        <div class="dialog-header-controls">
          {#if headerActions}
            {@render headerActions()}
          {/if}
          <button class="dialog-close-btn" onclick={onclose} aria-label="Close dialog">
            <Icon name="close" size={14} />
          </button>
        </div>
      </div>

      <div class="dialog-body">
        {#if children}
          {@render children()}
        {/if}
      </div>

      {#if footer}
        <div class="dialog-footer">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .dialog-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(4, 5, 8, 0.75);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
    z-index: 1000;
    overflow-y: auto;
    animation: fadeIn 0.15s ease-out;
  }

  .dialog-panel {
    background: var(--bg-surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-modal);
    width: 100%;
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - 48px);
    overflow: hidden;
    animation: slideUp 0.15s ease-out;
    position: relative;
  }

  .dialog-panel::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 1px;
    background: linear-gradient(90deg, transparent, var(--border-focus), transparent);
  }

  .dialog-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-surface-elevated);
  }

  .dialog-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    letter-spacing: -0.01em;
  }

  .dialog-subtitle {
    font-size: 11.5px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  .dialog-header-controls {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .dialog-close-btn {
    width: 26px;
    height: 26px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-xs);
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.12s;
  }
  .dialog-close-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .dialog-body {
    padding: 20px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 10px;
    padding: 12px 20px;
    border-top: 1px solid var(--border-subtle);
    background: var(--bg-surface-elevated);
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  @keyframes slideUp {
    from {
      opacity: 0;
      transform: translateY(8px) scale(0.99);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }
</style>
