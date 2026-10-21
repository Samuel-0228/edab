<script lang="ts">
  import {
    currentRoute,
    navigate,
    summaryCounts,
    openTourModal
  } from '../../api/stores';
  import type { AppRoute } from '../../api/types';
  import Icon from '../icons/Icon.svelte';

  interface NavItem {
    id: AppRoute;
    label: string;
    icon: any;
    countKey?: keyof typeof $summaryCounts;
    danger?: boolean;
  }

  const engineNav: NavItem[] = [
    { id: 'overview', label: 'Overview', icon: 'overview' },
    { id: 'workflows', label: 'Automations', icon: 'workflow', countKey: 'activeWorkflows' },
    { id: 'schedules', label: 'Schedules', icon: 'schedule' },
    { id: 'executions', label: 'Executions', icon: 'execution' },
    { id: 'failures', label: 'Dead-Letter Queue', icon: 'failure', countKey: 'deadLetters', danger: true }
  ];

  const connectivityNav: NavItem[] = [
    { id: 'events', label: 'Events Stream', icon: 'event' },
    { id: 'webhooks', label: 'Inbound Webhooks', icon: 'webhook' },
    { id: 'integrations', label: 'Integrations', icon: 'integration' },
    { id: 'credentials', label: 'Credentials Store', icon: 'credential' }
  ];

  const systemNav: NavItem[] = [
    { id: 'logs', label: 'Real-time Logs', icon: 'logs' },
    { id: 'health', label: 'System Health', icon: 'health' },
    { id: 'settings', label: 'Settings', icon: 'settings' }
  ];
</script>

<aside class="tech-sidebar">
  <!-- Brand Section -->
  <div class="sidebar-brand">
    <div class="brand-logo-container">
      <img src="/kerob_logo.png" alt="Kerob" class="brand-logo-img" />
    </div>
    <div class="brand-meta">
      <div class="brand-title-row">
        <span class="brand-title">KEROB</span>
        <span class="brand-badge">CONTROL</span>
      </div>
      <span class="brand-subtitle">Automation Platform</span>
    </div>
  </div>

  <!-- Navigation Lists -->
  <div class="sidebar-nav-scroll">
    <div class="nav-section">
      <div class="nav-section-title">ENGINE CONTROL</div>
      <div class="nav-items-group">
        {#each engineNav as item}
          <button
            class="nav-link {$currentRoute === item.id ? 'active' : ''}"
            onclick={() => navigate(item.id)}
          >
            <div class="nav-link-left">
              <Icon name={item.icon} size={15} />
              <span>{item.label}</span>
            </div>
            {#if item.countKey !== undefined && $summaryCounts[item.countKey] !== undefined}
              <span class="nav-counter {item.danger && $summaryCounts[item.countKey] > 0 ? 'danger' : ''}">
                {$summaryCounts[item.countKey]}
              </span>
            {/if}
          </button>
        {/each}
      </div>
    </div>

    <div class="nav-section">
      <div class="nav-section-title">CONNECTIVITY</div>
      <div class="nav-items-group">
        {#each connectivityNav as item}
          <button
            class="nav-link {$currentRoute === item.id ? 'active' : ''}"
            onclick={() => navigate(item.id)}
          >
            <div class="nav-link-left">
              <Icon name={item.icon} size={15} />
              <span>{item.label}</span>
            </div>
          </button>
        {/each}
      </div>
    </div>

    <div class="nav-section">
      <div class="nav-section-title">OBSERVABILITY</div>
      <div class="nav-items-group">
        {#each systemNav as item}
          <button
            class="nav-link {$currentRoute === item.id ? 'active' : ''}"
            onclick={() => navigate(item.id)}
          >
            <div class="nav-link-left">
              <Icon name={item.icon} size={15} />
              <span>{item.label}</span>
            </div>
          </button>
        {/each}
      </div>
    </div>
  </div>

  <!-- Sidebar Footer -->
  <div class="sidebar-footer">
    <div class="status-indicator-box">
      <span class="pulse-dot"></span>
      <span class="status-text">CORE ACTIVE</span>
    </div>

    <button class="tour-btn" onclick={openTourModal} title="Guided Tour">
      Tour
    </button>
  </div>
</aside>

<style>
  .tech-sidebar {
    width: var(--sidebar-width);
    min-width: var(--sidebar-width);
    height: 100vh;
    background: var(--bg-surface);
    border-right: 1px solid var(--border-default);
    display: flex;
    flex-direction: column;
    z-index: 10;
  }

  .sidebar-brand {
    height: var(--header-height);
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 16px;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-deep);
  }

  .brand-logo-container {
    width: 30px;
    height: 30px;
    background: #141722;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    flex-shrink: 0;
    box-shadow: 0 0 10px rgba(0, 229, 153, 0.15);
  }

  .brand-logo-img {
    width: 100%;
    height: 100%;
    object-fit: contain;
    display: block;
    padding: 2px;
  }

  .brand-meta {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .brand-title-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .brand-title {
    font-size: 13.5px;
    font-weight: 700;
    letter-spacing: 0.08em;
    color: var(--text-primary);
  }

  .brand-badge {
    font-family: var(--font-mono);
    font-size: 9px;
    font-weight: 700;
    color: var(--accent-emerald);
    background: var(--accent-emerald-dim);
    padding: 1px 4px;
    border-radius: var(--radius-xs);
    border: 1px solid var(--accent-emerald-border);
  }

  .brand-subtitle {
    font-size: 10px;
    color: var(--text-muted);
    letter-spacing: 0.02em;
    text-transform: uppercase;
  }

  .sidebar-nav-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 16px 10px;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .nav-section {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .nav-section-title {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-faint);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    padding: 0 8px;
    margin-bottom: 2px;
  }

  .nav-items-group {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .nav-link {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 7px 10px;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-size: 12.5px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.12s;
    text-align: left;
    border: 1px solid transparent;
  }

  .nav-link:hover {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
  }

  .nav-link.active {
    background: var(--bg-surface-elevated);
    color: var(--text-primary);
    border-color: var(--border-subtle);
    font-weight: 600;
    box-shadow: inset 2px 0 0 var(--accent-emerald);
  }

  .nav-link-left {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .nav-counter {
    font-family: var(--font-mono);
    font-size: 10.5px;
    padding: 1px 5px;
    border-radius: var(--radius-xs);
    background: var(--bg-surface-active);
    color: var(--text-muted);
  }

  .nav-counter.danger {
    color: var(--accent-crimson);
    background: var(--accent-crimson-dim);
  }

  .sidebar-footer {
    padding: 12px 14px;
    border-top: 1px solid var(--border-subtle);
    background: var(--bg-deep);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .status-indicator-box {
    display: flex;
    align-items: center;
    gap: 7px;
  }

  .pulse-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--accent-emerald);
    box-shadow: 0 0 8px rgba(0, 229, 153, 0.7);
    animation: pulse 2s infinite ease-in-out;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; transform: scale(1); }
    50% { opacity: 0.4; transform: scale(0.85); }
  }

  .status-text {
    font-family: var(--font-mono);
    font-size: 10.5px;
    font-weight: 600;
    color: var(--text-secondary);
    letter-spacing: 0.04em;
  }

  .tour-btn {
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-muted);
    padding: 2px 8px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-xs);
    cursor: pointer;
    transition: all 0.12s;
  }
  .tour-btn:hover {
    color: var(--text-primary);
    border-color: var(--border-default);
    background: var(--bg-surface-elevated);
  }
</style>
