<script lang="ts">
  import { notifications, dismissToast } from '../../api/stores';
  import Icon from '../icons/Icon.svelte';
</script>

<div class="toast-container" aria-live="polite">
  {#each $notifications as toast (toast.id)}
    <div class="toast-item {toast.type}">
      <div class="toast-icon">
        {#if toast.type === 'success'}
          <Icon name="check" size={15} />
        {:else if toast.type === 'error'}
          <Icon name="alert" size={15} />
        {:else if toast.type === 'warning'}
          <Icon name="alert" size={15} />
        {:else}
          <Icon name="info" size={15} />
        {/if}
      </div>

      <div class="toast-content">
        {#if toast.title}
          <div class="toast-title">{toast.title}</div>
        {/if}
        <div class="toast-message">{toast.message}</div>
      </div>

      <button
        class="toast-close"
        onclick={() => dismissToast(toast.id)}
        aria-label="Dismiss notification"
      >
        <Icon name="close" size={12} />
      </button>
    </div>
  {/each}
</div>

<style>
  .toast-container {
    position: fixed;
    top: 16px;
    right: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    z-index: 2000;
    pointer-events: none;
    max-width: 380px;
    width: calc(100vw - 32px);
  }

  .toast-item {
    pointer-events: auto;
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 14px;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
    animation: slideIn 0.2s cubic-bezier(0.16, 1, 0.3, 1);
  }

  .toast-item.success {
    border-left: 3px solid var(--accent-emerald);
  }
  .toast-item.success .toast-icon {
    color: var(--accent-emerald);
  }

  .toast-item.error {
    border-left: 3px solid var(--accent-crimson);
  }
  .toast-item.error .toast-icon {
    color: var(--accent-crimson);
  }

  .toast-item.warning {
    border-left: 3px solid var(--accent-amber);
  }
  .toast-item.warning .toast-icon {
    color: var(--accent-amber);
  }

  .toast-item.info {
    border-left: 3px solid var(--accent-cyan);
  }
  .toast-item.info .toast-icon {
    color: var(--accent-cyan);
  }

  .toast-icon {
    flex-shrink: 0;
    margin-top: 1px;
  }

  .toast-content {
    flex: 1;
    min-width: 0;
  }

  .toast-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary);
    margin-bottom: 2px;
  }

  .toast-message {
    font-size: 11.5px;
    color: var(--text-secondary);
    line-height: 1.4;
    word-break: break-word;
  }

  .toast-close {
    flex-shrink: 0;
    color: var(--text-muted);
    cursor: pointer;
    padding: 2px;
    border-radius: var(--radius-xs);
    transition: color 0.12s;
  }
  .toast-close:hover {
    color: var(--text-primary);
  }

  @keyframes slideIn {
    from {
      opacity: 0;
      transform: translateX(16px);
    }
    to {
      opacity: 1;
      transform: translateX(0);
    }
  }
</style>
