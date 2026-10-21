<script lang="ts">
  import { onMount } from 'svelte';
  import { refreshTick, notifyError } from '../api/stores';
  import { api } from '../api/client';
  import type { HealthResponse, ReadyResponse, ParsedMetric } from '../api/types';
  import Card from '../components/common/Card.svelte';
  import Button from '../components/common/Button.svelte';
  import Badge from '../components/common/Badge.svelte';
  import StatCard from '../components/common/StatCard.svelte';
  import CodeBox from '../components/common/CodeBox.svelte';

  let health = $state<HealthResponse | null>(null);
  let ready = $state<ReadyResponse | null>(null);
  let rawMetrics = $state<string>('');
  let parsedMetrics = $state<ParsedMetric[]>([]);

  function parsePrometheus(text: string): ParsedMetric[] {
    const lines = text.split('\n');
    const result: ParsedMetric[] = [];

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('#')) continue;

      const spaceIdx = trimmed.lastIndexOf(' ');
      if (spaceIdx === -1) continue;

      const metricPart = trimmed.substring(0, spaceIdx);
      const valStr = trimmed.substring(spaceIdx + 1);
      const val = parseFloat(valStr);

      if (isNaN(val)) continue;

      let name = metricPart;
      const labels: Record<string, string> = {};

      const braceStart = metricPart.indexOf('{');
      if (braceStart !== -1 && metricPart.endsWith('}')) {
        name = metricPart.substring(0, braceStart);
        const labelStr = metricPart.substring(braceStart + 1, metricPart.length - 1);
        const parts = labelStr.split(',');
        for (const p of parts) {
          const [k, v] = p.split('=');
          if (k && v) {
            labels[k.trim()] = v.replace(/^"|"$/g, '').trim();
          }
        }
      }

      result.push({
        name,
        value: val,
        labels: Object.keys(labels).length > 0 ? labels : undefined
      });
    }

    return result;
  }

  async function loadHealthData() {
    try {
      const [h, r, m] = await Promise.all([
        api.getHealth(),
        api.getReady(),
        api.getMetricsText()
      ]);
      health = h;
      ready = r;
      rawMetrics = m;
      parsedMetrics = parsePrometheus(m);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to query system health';
      notifyError(msg);
    }
  }

  $effect(() => {
    void $refreshTick;
    loadHealthData();
  });

  onMount(() => {
    loadHealthData();
  });

  let workflowExecs = $derived(
    parsedMetrics.find((m) => m.name === 'kerob_workflow_executions_total')?.value ?? 0
  );
  let workflowSuccesses = $derived(
    parsedMetrics.find((m) => m.name === 'kerob_workflow_successes_total')?.value ?? 0
  );
  let workflowFailures = $derived(
    parsedMetrics.find((m) => m.name === 'kerob_workflow_failures_total')?.value ?? 0
  );
  let eventsProcessed = $derived(
    parsedMetrics.find((m) => m.name === 'kerob_events_processed_total')?.value ?? 0
  );
</script>

<div class="health-view">
  <!-- Top Probes Status Row -->
  <div class="grid-3">
    <Card title="Liveness Probe" subtitle="Continuous HTTP probe (/health)">
      <div class="probe-content">
        <div class="probe-status-row">
          <Badge variant={health?.status === 'healthy' ? 'success' : 'failed'} dot size="md">
            {health?.status ? health.status.toUpperCase() : 'UNKNOWN'}
          </Badge>
          <span class="probe-meta mono">200 OK</span>
        </div>
        <p class="probe-desc">
          Responds immediately if the HTTP listener and core event loops are responsive.
        </p>
      </div>
    </Card>

    <Card title="Readiness Probe" subtitle="Dependency validation (/ready)">
      <div class="probe-content">
        <div class="probe-status-row">
          <Badge variant={ready?.status === 'ready' ? 'success' : 'failed'} dot size="md">
            {ready?.status ? ready.status.toUpperCase() : 'NOT READY'}
          </Badge>
          <span class="probe-meta mono">Workers Initialized</span>
        </div>
        <p class="probe-desc">
          Ensures database pools, worker concurrency slots, and adapter registries are online.
        </p>
      </div>
    </Card>

    <Card title="SSRF & Network Shield" subtitle="Outbound egress protection">
      <div class="probe-content">
        <div class="probe-status-row">
          <Badge variant="success" size="md">ENFORCED</Badge>
          <span class="probe-meta mono">RFC 1918 Blocked</span>
        </div>
        <p class="probe-desc">
          Restricted HTTP client blocks loopback, link-local, and metadata services (169.254.169.254).
        </p>
      </div>
    </Card>
  </div>

  <!-- Prometheus Metrics Gauges -->
  <div class="grid-4">
    <StatCard
      label="Prometheus Executions"
      value={workflowExecs}
      subtext="kerob_workflow_executions_total"
      indicator="neutral"
      icon="workflow"
    />

    <StatCard
      label="Execution Successes"
      value={workflowSuccesses}
      subtext="kerob_workflow_successes_total"
      indicator="positive"
      icon="check"
    />

    <StatCard
      label="Execution Failures"
      value={workflowFailures}
      subtext="kerob_workflow_failures_total"
      indicator={workflowFailures > 0 ? 'negative' : 'positive'}
      icon="failure"
    />

    <StatCard
      label="Events Processed"
      value={eventsProcessed}
      subtext="kerob_events_processed_total"
      indicator="neutral"
      icon="event"
    />
  </div>

  <!-- Registered Adapters In Readiness -->
  {#if ready}
    <div class="grid-3">
      <Card title="Registered Triggers" subtitle="Sources listening for inbound events">
        <div class="tags-list">
          {#each ready.registered_sources as s}
            <Badge variant="stage-when">{s}</Badge>
          {/each}
        </div>
      </Card>

      <Card title="Registered Destinations" subtitle="Outbound delivery targets">
        <div class="tags-list">
          {#each ready.registered_destinations as d}
            <Badge variant="stage-then">{d}</Badge>
          {/each}
        </div>
      </Card>

      <Card title="Registered Actions" subtitle="Pipeline action executors">
        <div class="tags-list">
          {#each ready.registered_actions as a}
            <Badge variant="neutral">{a}</Badge>
          {/each}
        </div>
      </Card>
    </div>
  {/if}

  <!-- Raw Prometheus Scrape Output -->
  <Card
    title="Prometheus Exposition Stream (/metrics)"
    subtitle="Standard Prometheus text exposition format consumed by monitoring scrapers."
    padding="none"
  >
    {#snippet headerActions()}
      <Button variant="ghost" size="sm" onclick={loadHealthData}>
        Re-scrape
      </Button>
    {/snippet}

    <div style="padding: 14px 16px;">
      <CodeBox content={rawMetrics || '# No metrics scraped yet'} maxHeight="260px" language="plaintext" />
    </div>
  </Card>
</div>

<style>
  .health-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .probe-content {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .probe-status-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .probe-meta {
    font-size: 11px;
    color: var(--text-muted);
  }

  .probe-desc {
    font-size: 11.5px;
    color: var(--text-secondary);
    line-height: 1.45;
  }

  .tags-list {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
</style>
