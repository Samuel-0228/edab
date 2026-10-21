<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../icons/Icon.svelte';

  interface Props {
    title: string;
    description?: string;
    icon?: any;
    class?: string;
    action?: Snippet;
  }

  let {
    title,
    description,
    icon = 'info',
    class: className = '',
    action
  }: Props = $props();
</script>

<div class="empty-state-panel {className}">
  <div class="empty-icon-wrap">
    <Icon name={icon} size={24} />
  </div>
  <h4 class="empty-title">{title}</h4>
  {#if description}
    <p class="empty-description">{description}</p>
  {/if}
  {#if action}
    <div class="empty-action">
      {@render action()}
    </div>
  {/if}
</div>

<style>
  .empty-state-panel {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: 44px 20px;
    background: rgba(0, 0, 0, 0.2);
    border-radius: var(--radius-md);
  }

  .empty-icon-wrap {
    color: var(--text-faint);
    margin-bottom: 12px;
    padding: 10px;
    border-radius: 50%;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-subtle);
  }

  .empty-title {
    font-size: 13.5px;
    font-weight: 600;
    color: var(--text-secondary);
    margin-bottom: 4px;
  }

  .empty-description {
    font-size: 11.5px;
    color: var(--text-muted);
    max-width: 380px;
    line-height: 1.5;
  }

  .empty-action {
    margin-top: 14px;
  }
</style>
