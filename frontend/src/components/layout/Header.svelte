<script lang="ts">
  import {
    currentRoute,
    triggerRefresh,
    openCreateWorkflow,
    openDispatchModal
  } from '../../api/stores';
  import type { AppRoute } from '../../api/types';
  import Button from '../common/Button.svelte';

  const routeTitles: Record<AppRoute, { title: string; subtitle: string }> = {
    overview: {
      title: 'Platform Overview',
      subtitle: 'Real-time operational engine state & execution throughput'
    },
    workflows: {
      title: 'Automation Pipelines',
      subtitle: 'Configured event-driven workflow rules, triggers & transformations'
    },
    builder: {
      title: 'Workflow Studio',
      subtitle: 'Interactive 4-stage automation builder & lifecycle validator'
    },
    schedules: {
      title: 'Scheduled Automations',
      subtitle: 'Periodic cron expressions & interval tick pipelines'
    },
    executions: {
      title: 'Execution Audit Trail',
      subtitle: 'Detailed runs and granular step-by-step lifecycle outputs'
    },
    failures: {
      title: 'Dead-Letter Queue',
      subtitle: 'Unrecovered failures requiring diagnosis, retry, or discard'
    },
    events: {
      title: 'Normalized Events Stream',
      subtitle: 'Immutable event log ingested across messaging & webhook adapters'
    },
    webhooks: {
      title: 'Inbound Webhook Endpoints',
      subtitle: 'HTTP endpoints receiving external webhooks and ingesting events'
    },
    integrations: {
      title: 'Integrations Catalog',
      subtitle: 'Supported service adapters, triggers, actions, and security requirements'
    },
    credentials: {
      title: 'Credentials Vault',
      subtitle: 'AES-256-GCM encrypted tokens for external API authorizations'
    },
    logs: {
      title: 'Live Engine Logs',
      subtitle: 'Real-time structured event, execution, and delivery logs stream'
    },
    health: {
      title: 'System Health & Diagnostics',
      subtitle: 'Liveness probes, Prometheus metrics gauges, and SSRF network policies'
    },
    settings: {
      title: 'Platform Settings',
      subtitle: 'API authentication tokens, backend endpoints, and security keys'
    }
  };

  let pageMeta = $derived(
    routeTitles[$currentRoute] || {
      title: 'Kerob Engine',
      subtitle: 'Automation Control Platform'
    }
  );
</script>

<header class="tech-header">
  <div class="header-left">
    <h2 class="header-title">{pageMeta.title}</h2>
    <span class="header-separator">/</span>
    <span class="header-subtitle">{pageMeta.subtitle}</span>
  </div>

  <div class="header-actions">
    <Button variant="ghost" size="sm" icon="refresh" onclick={triggerRefresh}>
      Sync
    </Button>

    <Button variant="secondary" size="sm" icon="play" onclick={() => openDispatchModal()}>
      Test Ingest
    </Button>

    <Button variant="primary" size="sm" icon="plus" onclick={() => openCreateWorkflow()}>
      New Automation
    </Button>
  </div>
</header>

<style>
  .tech-header {
    height: var(--header-height);
    background: var(--bg-surface);
    border-bottom: 1px solid var(--border-default);
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 24px;
    z-index: 5;
    flex-shrink: 0;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .header-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    white-space: nowrap;
    letter-spacing: -0.01em;
  }

  .header-separator {
    color: var(--text-faint);
    font-family: var(--font-mono);
  }

  .header-subtitle {
    font-size: 11.5px;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }

  @media (max-width: 860px) {
    .header-subtitle, .header-separator {
      display: none;
    }
  }
</style>
