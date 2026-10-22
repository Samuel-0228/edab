<script lang="ts">
  import { tourModal, closeTourModal } from '../../api/stores';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';

  const steps = [
    {
      title: '1. Platform Architecture & Zero Mock Data',
      badge: 'FOUNDATION',
      content:
        'Kerob is an event-driven automation platform built on Go and PostgreSQL. Every metric, execution count, dead-letter record, and credential shown in this interface reflects live, persistent engine state. Zero mock data or synthetic placeholders are used.'
    },
    {
      title: '2. The Adapter Ecosystem',
      badge: 'INTEGRATIONS',
      content:
        'All external service protocols (Telegram Bot API, Telegram Public Poller, Discord Webhooks, GitHub REST & Webhooks, Inbound HTTP Webhooks, and Generic REST) plug in through uniform adapter interfaces. The core routing and condition engine remains completely service-agnostic.'
    },
    {
      title: '3. Declarative Pipeline: WHEN → IF → TRANSFORM → THEN',
      badge: 'LIFECYCLE',
      content:
        'Automations follow a 4-stage pipeline. A Trigger (WHEN) normalizes external events. A Condition (IF) evaluates deterministic rules (equals, contains, regex, numerical comparisons). A Transformation formats event payloads using {{payload.var}} templates. An Action (THEN) delivers to target destinations with idempotency.'
    },
    {
      title: '4. Execution Auditing & Dead-Letter Queue',
      badge: 'RELIABILITY',
      content:
        'Every pipeline run records granular step durations, inputs, and outputs. Failed actions retry with exponential backoff. If max retries are exhausted, the execution lands safely in the Dead-Letter Queue for operator diagnosis and one-click replay.'
    },
    {
      title: '5. Security, SSRF Hardening & Encrypted Vault',
      badge: 'SECURITY',
      content:
        'Sensitive integration tokens are encrypted at rest with AES-256-GCM and never exposed via standard API endpoints. Outbound HTTP requests are protected by SSRF defenses blocking private RFC 1918 subnets, loopbacks, and cloud metadata endpoints (169.254.169.254).'
    }
  ];

  let currentStep = $state(0);

  $effect(() => {
    if ($tourModal) {
      currentStep = 0;
    }
  });

  function nextStep() {
    if (currentStep < steps.length - 1) {
      currentStep++;
    } else {
      closeTourModal();
    }
  }

  function prevStep() {
    if (currentStep > 0) {
      currentStep--;
    }
  }
</script>

<Dialog
  open={$tourModal}
  title="Kerob Architectural Overview"
  subtitle={`Step ${currentStep + 1} of ${steps.length} — Platform Operational Guide`}
  maxWidth="580px"
  onclose={closeTourModal}
>
  {@const step = steps[currentStep]}
  <div class="tour-card">
    <div class="tour-top">
      <span class="tour-badge">{step.badge}</span>
      <span class="tour-counter">STEP {currentStep + 1} / {steps.length}</span>
    </div>

    <h3 class="tour-title">{step.title}</h3>
    <p class="tour-content">{step.content}</p>
  </div>

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeTourModal}>Exit Tour</Button>
    <div style="flex:1;"></div>
    {#if currentStep > 0}
      <Button variant="secondary" size="sm" onclick={prevStep}>Back</Button>
    {/if}
    <Button variant="primary" size="sm" onclick={nextStep}>
      {currentStep === steps.length - 1 ? 'Finish Tour' : 'Next Step →'}
    </Button>
  {/snippet}
</Dialog>

<style>
  .tour-card {
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: 18px 20px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .tour-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .tour-badge {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 700;
    color: var(--accent-emerald);
    background: var(--accent-emerald-dim);
    border: 1px solid var(--accent-emerald-border);
    padding: 2px 6px;
    border-radius: var(--radius-xs);
    letter-spacing: 0.05em;
  }

  .tour-counter {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-muted);
  }

  .tour-title {
    font-size: 14.5px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .tour-content {
    font-size: 12.5px;
    line-height: 1.6;
    color: var(--text-secondary);
  }
</style>
