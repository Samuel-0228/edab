<script lang="ts">
  import Button from './Button.svelte';

  interface Props {
    label?: string;
    value?: string;
    placeholder?: string;
    minHeight?: string;
    hint?: string;
    required?: boolean;
    class?: string;
    onchange?: (val: string) => void;
  }

  let {
    label,
    value = $bindable('{}'),
    placeholder = '{\n  "key": "value"\n}',
    minHeight = '120px',
    hint,
    required = false,
    class: className = '',
    onchange
  }: Props = $props();

  let syntaxError = $state<string | null>(null);
  const textareaId = `json-textarea-${Math.random().toString(36).substring(2, 9)}`;

  function validate(val: string) {
    if (!val.trim()) {
      syntaxError = null;
      return;
    }
    try {
      JSON.parse(val);
      syntaxError = null;
    } catch (err: unknown) {
      syntaxError = err instanceof Error ? err.message : 'Invalid JSON';
    }
  }

  function handleInput(e: Event) {
    const target = e.target as HTMLTextAreaElement;
    validate(target.value);
    onchange?.(target.value);
  }

  function formatJson() {
    try {
      const parsed = JSON.parse(value);
      value = JSON.stringify(parsed, null, 2);
      syntaxError = null;
      onchange?.(value);
    } catch {
      // Keep as-is if invalid
    }
  }

  $effect(() => {
    validate(value);
  });
</script>

<div class="json-editor-field {className}">
  <div class="editor-header">
    {#if label}
      <label class="field-label" for={textareaId}>
        {label}
        {#if required}
          <span class="required-star">*</span>
        {/if}
      </label>
    {/if}

    <div class="editor-actions">
      {#if syntaxError}
        <span class="syntax-badge error">Syntax Error</span>
      {:else}
        <span class="syntax-badge valid">Valid JSON</span>
      {/if}
      <Button variant="ghost" size="sm" onclick={formatJson}>Format</Button>
    </div>
  </div>

  <textarea
    id={textareaId}
    bind:value
    {placeholder}
    style="min-height: {minHeight}"
    class="json-textarea {syntaxError ? 'has-error' : ''}"
    spellcheck="false"
    oninput={handleInput}
  ></textarea>

  {#if syntaxError}
    <div class="syntax-error-text">{syntaxError}</div>
  {:else if hint}
    <div class="field-hint">{hint}</div>
  {/if}
</div>

<style>
  .json-editor-field {
    display: flex;
    flex-direction: column;
    gap: 5px;
    width: 100%;
  }

  .editor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
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

  .editor-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .syntax-badge {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 600;
    padding: 1px 5px;
    border-radius: var(--radius-xs);
    border: 1px solid transparent;
  }
  .syntax-badge.valid {
    color: var(--accent-emerald);
    background: var(--accent-emerald-dim);
    border-color: var(--accent-emerald-border);
  }
  .syntax-badge.error {
    color: var(--accent-crimson);
    background: var(--accent-crimson-dim);
    border-color: var(--accent-crimson-border);
  }

  .json-textarea {
    width: 100%;
    padding: 10px 12px;
    background: #090a0f;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 11.5px;
    line-height: 1.5;
    resize: vertical;
    transition: all 0.12s;
  }

  .json-textarea:hover {
    border-color: var(--border-strong);
  }

  .json-textarea:focus {
    border-color: var(--border-focus);
    background: #0d0f17;
    outline: none;
    box-shadow: 0 0 0 1px var(--border-focus);
  }

  .json-textarea.has-error {
    border-color: var(--accent-crimson);
  }

  .syntax-error-text {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--accent-crimson);
  }

  .field-hint {
    font-size: 11px;
    color: var(--text-muted);
  }
</style>
