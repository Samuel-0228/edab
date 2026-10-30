<script lang="ts">
  import Badge from '../common/Badge.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import type { SelectOption } from '../common/Select.svelte';

  interface Props {
    triggerType: string;
    triggerEventType: string;
    scheduleInterval: string;
    scheduleCron: string;
    options: SelectOption[];
    onchange?: () => void;
  }

  let {
    triggerType = $bindable('telegram'),
    triggerEventType = $bindable('*'),
    scheduleInterval = $bindable('5m'),
    scheduleCron = $bindable(''),
    options = [],
    onchange
  }: Props = $props();
</script>

<div class="stage-card">
  <div class="stage-header">
    <Badge variant="stage-when">WHEN</Badge>
    <span class="stage-title">Trigger Event Ingestion</span>
  </div>

  <div class="stage-fields-grid">
    <Select
      label="Service Adapter"
      {options}
      bind:value={triggerType}
      {onchange}
    />

    <Input
      label="Event Type Pattern"
      placeholder="e.g. * or telegram.message"
      bind:value={triggerEventType}
      mono
      hint="Use * to match all events from this trigger adapter."
    />
  </div>

  {#if triggerType === 'schedule'}
    <div class="schedule-config-box">
      <Input
        label="Tick Interval (e.g. 1m, 5m, 1h, 24h)"
        placeholder="5m"
        bind:value={scheduleInterval}
        mono
      />
      <Input
        label="Or Standard Cron Expression (optional)"
        placeholder="*/5 * * * *"
        bind:value={scheduleCron}
        mono
        hint="5-field cron expression evaluated by the engine background scheduler."
      />
    </div>
  {/if}
</div>

<style>
  .stage-card {
    background: var(--card-bg, #18181b);
    border: 1px solid var(--border-color, #27272a);
    border-radius: 8px;
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .stage-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 2px;
  }

  .stage-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary, #f4f4f5);
  }

  .stage-fields-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  .schedule-config-box {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    background: rgba(0, 0, 0, 0.2);
    border: 1px dashed var(--border-color, #3f3f46);
    border-radius: 6px;
    padding: 10px;
    margin-top: 4px;
  }
</style>
