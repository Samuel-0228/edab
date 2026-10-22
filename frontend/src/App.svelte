<script lang="ts">
  import { currentRoute, engineConnection, triggerRefresh } from './api/stores';
  import { api } from './api/client';
  import './styles/theme.css';

  import Sidebar from './components/layout/Sidebar.svelte';
  import Header from './components/layout/Header.svelte';
  import NotificationToast from './components/common/NotificationToast.svelte';

  // Views
  import OverviewView from './views/OverviewView.svelte';
  import WorkflowsView from './views/WorkflowsView.svelte';
  import SchedulesView from './views/SchedulesView.svelte';
  import IntegrationsView from './views/IntegrationsView.svelte';
  import CredentialsView from './views/CredentialsView.svelte';
  import ExecutionsView from './views/ExecutionsView.svelte';
  import FailuresView from './views/FailuresView.svelte';
  import EventsView from './views/EventsView.svelte';
  import WebhooksView from './views/WebhooksView.svelte';
  import LogsView from './views/LogsView.svelte';
  import HealthView from './views/HealthView.svelte';
  import SettingsView from './views/SettingsView.svelte';

  // Global Modals
  import WorkflowBuilderModal from './components/modals/WorkflowBuilderModal.svelte';
  import DispatchEventModal from './components/modals/DispatchEventModal.svelte';
  import CredentialModal from './components/modals/CredentialModal.svelte';
  import WebhookModal from './components/modals/WebhookModal.svelte';
  import ExecutionDetailModal from './components/modals/ExecutionDetailModal.svelte';
  import TourModal from './components/modals/TourModal.svelte';
  import ConfirmModal from './components/modals/ConfirmModal.svelte';
</script>

<div class="noise-overlay"></div>

<div class="app-shell">
  <!-- Sidebar Navigation -->
  <Sidebar />

  <!-- Main View Area -->
  <div class="main-viewport">
    <Header />

    {#if $engineConnection.online === false}
      <div class="engine-offline-bar">
        <div class="engine-offline-content">
          <span class="offline-pill">ENGINE OFFLINE</span>
          <span class="offline-text">
            Unable to connect to Kerob Engine at <code
              >{api.getBaseUrl() || 'http://localhost:8080'}</code
            >. Ensure the service is running (<code>docker compose up -d</code> or
            <code>go run ./cmd/server</code>).
          </span>
        </div>
        <button class="offline-retry-btn" onclick={() => triggerRefresh()}>
          ↻ Retry Connection
        </button>
      </div>
    {/if}

    <main class="page-content-scroll">
      {#if $currentRoute === 'overview'}
        <OverviewView />
      {:else if $currentRoute === 'workflows' || $currentRoute === 'builder'}
        <WorkflowsView />
      {:else if $currentRoute === 'schedules'}
        <SchedulesView />
      {:else if $currentRoute === 'integrations'}
        <IntegrationsView />
      {:else if $currentRoute === 'credentials'}
        <CredentialsView />
      {:else if $currentRoute === 'executions'}
        <ExecutionsView />
      {:else if $currentRoute === 'failures'}
        <FailuresView />
      {:else if $currentRoute === 'events'}
        <EventsView />
      {:else if $currentRoute === 'webhooks'}
        <WebhooksView />
      {:else if $currentRoute === 'logs'}
        <LogsView />
      {:else if $currentRoute === 'health'}
        <HealthView />
      {:else if $currentRoute === 'settings'}
        <SettingsView />
      {:else}
        <OverviewView />
      {/if}
    </main>
  </div>
</div>

<!-- Global Modals & Notifications -->
<WorkflowBuilderModal />
<DispatchEventModal />
<CredentialModal />
<WebhookModal />
<ExecutionDetailModal />
<TourModal />
<ConfirmModal />
<NotificationToast />

<style>
  .engine-offline-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 10px 24px;
    background: rgba(255, 51, 85, 0.08);
    border-bottom: 1px solid var(--accent-crimson-border);
    animation: fadeIn 0.2s ease-out;
  }

  .engine-offline-content {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }

  .offline-pill {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.08em;
    padding: 3px 8px;
    background: var(--accent-crimson);
    color: #fff;
    border-radius: var(--radius-xs);
    box-shadow: 0 0 10px rgba(255, 51, 85, 0.4);
  }

  .offline-text {
    font-size: 12px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .offline-text code {
    font-family: var(--font-mono);
    font-size: 11px;
    background: var(--bg-surface-elevated);
    padding: 2px 6px;
    border-radius: var(--radius-xs);
    color: var(--accent-amber);
    border: 1px solid var(--border-subtle);
  }

  .offline-retry-btn {
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 600;
    padding: 5px 12px;
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;
  }

  .offline-retry-btn:hover {
    background: var(--bg-surface-hover);
    border-color: var(--accent-crimson);
    color: var(--accent-crimson);
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
