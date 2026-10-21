<script lang="ts">
  import Icon from '../icons/Icon.svelte';

  interface Props {
    label: string;
    value: string | number;
    subtext?: string;
    indicator?: 'positive' | 'warning' | 'negative' | 'neutral';
    icon?: any;
    class?: string;
  }

  let {
    label,
    value,
    subtext,
    indicator = 'neutral',
    icon,
    class: className = ''
  }: Props = $props();
</script>

<div class="stat-card {className}">
  <div class="stat-top">
    <div class="stat-label-row">
      <span class="status-dot {indicator}"></span>
      <span class="stat-label">{label}</span>
    </div>
    {#if icon}
      <div class="stat-icon-wrap">
        <Icon name={icon} size={15} />
      </div>
    {/if}
  </div>

  <div class="stat-value">{value}</div>

  {#if subtext}
    <div class="stat-subtext">{subtext}</div>
  {/if}
</div>

<style>
  .stat-card {
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    position: relative;
    overflow: hidden;
  }

  .stat-card::after {
    content: "";
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 1px;
    background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.08), transparent);
  }

  .stat-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }

  .stat-label-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .stat-label {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
  }
  .status-dot.positive {
    background: var(--accent-emerald);
    box-shadow: 0 0 6px rgba(0, 229, 153, 0.5);
  }
  .status-dot.warning {
    background: var(--accent-amber);
    box-shadow: 0 0 6px rgba(245, 158, 11, 0.5);
  }
  .status-dot.negative {
    background: var(--accent-crimson);
    box-shadow: 0 0 6px rgba(255, 51, 85, 0.5);
  }
  .status-dot.neutral {
    background: var(--text-faint);
  }

  .stat-icon-wrap {
    color: var(--text-muted);
  }

  .stat-value {
    font-family: var(--font-mono);
    font-size: 22px;
    font-weight: 700;
    color: var(--text-primary);
    line-height: 1.15;
    letter-spacing: -0.02em;
  }

  .stat-subtext {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 6px;
    display: flex;
    align-items: center;
    gap: 4px;
  }
</style>
