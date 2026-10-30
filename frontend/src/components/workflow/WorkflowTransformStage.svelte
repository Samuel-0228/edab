<script lang="ts">
  import Badge from '../common/Badge.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import type { SelectOption } from '../common/Select.svelte';

  interface Props {
    transformType: 'passthrough' | 'template' | 'mapping';
    transformTemplate: string;
    options: SelectOption[];
  }

  let {
    transformType = $bindable('passthrough'),
    transformTemplate = $bindable(''),
    options = []
  }: Props = $props();
</script>

<div class="stage-card">
  <div class="stage-header">
    <Badge variant="stage-transform">TRANSFORM</Badge>
    <span class="stage-title">Deterministic Data Formatting (Optional)</span>
  </div>

  <div class="transform-fields-grid">
    <Select label="Transform Mode" {options} bind:value={transformType} />

    <Input
      label="Template Expression / Field Map"
      placeholder="[event.source_id] payload.title — payload.author"
      bind:value={transformTemplate}
      mono
      hint="Interpolate fields into the output payload before dispatching."
    />
  </div>
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

  .transform-fields-grid {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: 12px;
  }
</style>
