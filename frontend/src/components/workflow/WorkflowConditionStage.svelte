<script lang="ts">
  import Badge from '../common/Badge.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import type { SelectOption } from '../common/Select.svelte';

  interface Props {
    conditionField: string;
    conditionOp: string;
    conditionVal: string;
    operators: SelectOption[];
  }

  let {
    conditionField = $bindable(''),
    conditionOp = $bindable(''),
    conditionVal = $bindable(''),
    operators = []
  }: Props = $props();
</script>

<div class="stage-card">
  <div class="stage-header">
    <Badge variant="stage-if">IF</Badge>
    <span class="stage-title">Evaluation Conditions (Optional)</span>
  </div>

  <div class="condition-fields-grid">
    <Input
      label="Payload Field Path"
      placeholder="e.g. payload.text or payload.title"
      bind:value={conditionField}
      mono
    />

    <Select label="Operator" options={operators} bind:value={conditionOp} />

    <Input
      label="Target Value"
      placeholder="e.g. urgent or true"
      bind:value={conditionVal}
      mono
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

  .condition-fields-grid {
    display: grid;
    grid-template-columns: 1.2fr 1fr 1fr;
    gap: 10px;
  }
</style>
