<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title?: string;
    subtitle?: string;
    badge?: string;
    badgeVariant?: 'neutral' | 'success' | 'warning' | 'danger';
    padding?: 'none' | 'sm' | 'md' | 'lg';
    class?: string;
    headerActions?: Snippet;
    children?: Snippet;
  }

  let {
    title,
    subtitle,
    badge,
    badgeVariant = 'neutral',
    padding = 'md',
    class: className = '',
    headerActions,
    children
  }: Props = $props();
</script>

<div class="tech-card padding-{padding} {className}">
  {#if title || headerActions}
    <div class="card-header">
      <div class="header-titles">
        {#if title}
          <div class="card-title-row">
            <h3 class="card-title">{title}</h3>
            {#if badge}
              <span class="card-badge {badgeVariant}">{badge}</span>
            {/if}
          </div>
        {/if}
        {#if subtitle}
          <p class="card-subtitle">{subtitle}</p>
        {/if}
      </div>

      {#if headerActions}
        <div class="header-actions">
          {@render headerActions()}
        </div>
      {/if}
    </div>
  {/if}

  <div class="card-body">
    {#if children}
      {@render children()}
    {/if}
  </div>
</div>

<style>
  .tech-card {
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    position: relative;
    overflow: hidden;
  }

  .card-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 14px 16px;
    border-bottom: 1px solid var(--border-subtle);
    background: rgba(255, 255, 255, 0.01);
  }

  .header-titles {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .card-title-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .card-title {
    font-size: 13.5px;
    font-weight: 600;
    color: var(--text-primary);
    letter-spacing: -0.01em;
  }

  .card-badge {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: var(--radius-xs);
    border: 1px solid transparent;
  }

  .card-badge.neutral {
    background: var(--bg-surface-elevated);
    color: var(--text-secondary);
    border-color: var(--border-subtle);
  }
  .card-badge.success {
    background: var(--accent-emerald-dim);
    color: var(--accent-emerald);
    border-color: var(--accent-emerald-border);
  }
  .card-badge.warning {
    background: var(--accent-amber-dim);
    color: var(--accent-amber);
    border-color: var(--accent-amber-border);
  }
  .card-badge.danger {
    background: var(--accent-crimson-dim);
    color: var(--accent-crimson);
    border-color: var(--accent-crimson-border);
  }

  .card-subtitle {
    font-size: 11.5px;
    color: var(--text-muted);
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  /* Padding Options */
  .padding-none .card-body {
    padding: 0;
  }
  .padding-sm .card-body {
    padding: 10px 12px;
  }
  .padding-md .card-body {
    padding: 14px 16px;
  }
  .padding-lg .card-body {
    padding: 20px 24px;
  }
</style>
