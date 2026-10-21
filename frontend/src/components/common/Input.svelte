<script lang="ts">
  interface Props {
    label?: string;
    value?: string | number;
    placeholder?: string;
    type?: string;
    hint?: string;
    error?: string;
    required?: boolean;
    disabled?: boolean;
    readonly?: boolean;
    mono?: boolean;
    class?: string;
    oninput?: (e: Event) => void;
    onchange?: (e: Event) => void;
  }

  let {
    label,
    value = $bindable(''),
    placeholder = '',
    type = 'text',
    hint,
    error,
    required = false,
    disabled = false,
    readonly = false,
    mono = false,
    class: className = '',
    oninput,
    onchange
  }: Props = $props();

  const inputId = `input-${Math.random().toString(36).substring(2, 9)}`;
</script>

<div class="tech-field {className}">
  {#if label}
    <label class="field-label" for={inputId}>
      {label}
      {#if required}
        <span class="required-star">*</span>
      {/if}
    </label>
  {/if}

  <div class="input-container">
    <input
      id={inputId}
      {type}
      bind:value
      {placeholder}
      {disabled}
      {readonly}
      {required}
      class="tech-input {mono ? 'font-mono' : ''} {error ? 'has-error' : ''}"
      {oninput}
      {onchange}
    />
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

  .input-container {
    position: relative;
    display: flex;
    align-items: center;
  }

  .tech-input {
    width: 100%;
    height: 34px;
    padding: 0 10px;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-size: 12.5px;
    transition: all 0.12s;
  }

  .tech-input.font-mono {
    font-family: var(--font-mono);
    font-size: 11.5px;
  }

  .tech-input:hover:not(:disabled) {
    border-color: var(--border-strong);
  }

  .tech-input:focus {
    border-color: var(--border-focus);
    background: var(--bg-surface-hover);
    outline: none;
    box-shadow: 0 0 0 1px var(--border-focus);
  }

  .tech-input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .tech-input.has-error {
    border-color: var(--accent-crimson);
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
</style>
