<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../icons/Icon.svelte';

  export interface SelectOption {
    value: string;
    label: string;
    disabled?: boolean;
  }

  interface Props {
    label?: string;
    value?: string;
    options?: SelectOption[];
    hint?: string;
    error?: string;
    required?: boolean;
    disabled?: boolean;
    mono?: boolean;
    class?: string;
    onchange?: (e: Event) => void;
    children?: Snippet;
  }

  let {
    label,
    value = $bindable(''),
    options = [],
    hint,
    error,
    required = false,
    disabled = false,
    mono = false,
    class: className = '',
    onchange,
    children
  }: Props = $props();

  const selectId = `select-${Math.random().toString(36).substring(2, 9)}`;
</script>

<div class="tech-field {className}">
  {#if label}
    <label class="field-label" for={selectId}>
      {label}
      {#if required}
        <span class="required-star">*</span>
      {/if}
    </label>
  {/if}

  <div class="select-container">
    <select
      id={selectId}
      bind:value
      {disabled}
      {required}
      class="tech-select {mono ? 'font-mono' : ''} {error ? 'has-error' : ''}"
      {onchange}
    >
      {#if children}
        {@render children()}
      {:else}
        {#each options as opt}
          <option value={opt.value} disabled={opt.disabled}>{opt.label}</option>
        {/each}
      {/if}
    </select>
    <div class="select-arrow">
      <Icon name="chevron-down" size={13} />
    </div>
  </div>

  {#if error}
    <div class="field-error">{error}</div>
  {:else if hint}
    <div class="field-hint">{hint}</div>
  {/if}
</div>

<style>
  .tech-field {
    display: flex;
    flex-direction: column;
    gap: 5px;
    width: 100%;
  }

  .field-label {
    font-size: 11.5px;
    font-weight: 500;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 3px;
  }

  .required-star {
    color: var(--accent-crimson);
  }

  .select-container {
    position: relative;
    display: flex;
    align-items: center;
  }

  .tech-select {
    width: 100%;
    height: 34px;
    padding: 0 28px 0 10px;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-size: 12.5px;
    appearance: none;
    -webkit-appearance: none;
    cursor: pointer;
    transition: all 0.12s;
  }

  .tech-select.font-mono {
    font-family: var(--font-mono);
    font-size: 11.5px;
  }

  .tech-select:hover:not(:disabled) {
    border-color: var(--border-strong);
  }

  .tech-select:focus {
    border-color: var(--border-focus);
    background: var(--bg-surface-hover);
    outline: none;
    box-shadow: 0 0 0 1px var(--border-focus);
  }

  .tech-select:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .tech-select.has-error {
    border-color: var(--accent-crimson);
  }

  .select-arrow {
    position: absolute;
    right: 8px;
    pointer-events: none;
    color: var(--text-muted);
  }

  .field-error {
    font-size: 11px;
    color: var(--accent-crimson);
    font-family: var(--font-mono);
  }

  .field-hint {
    font-size: 11px;
    color: var(--text-muted);
    line-height: 1.4;
  }

  option {
    background: var(--bg-surface);
    color: var(--text-primary);
  }
</style>
