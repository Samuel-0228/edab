<script lang="ts">
  import Icon from '../icons/Icon.svelte';
  import { notifySuccess } from '../../api/stores';

  interface Props {
    content: string | object | unknown;
    maxHeight?: string;
    language?: string;
    class?: string;
  }

  let { content, maxHeight = '240px', language = 'json', class: className = '' }: Props = $props();

  let formatted = $derived.by(() => {
    if (typeof content === 'string') {
      try {
        const parsed = JSON.parse(content);
        return JSON.stringify(parsed, null, 2);
      } catch {
        return content;
      }
    }
    try {
      return JSON.stringify(content, null, 2);
    } catch {
      return String(content);
    }
  });

  let copied = $state(false);

  async function copyToClipboard() {
    try {
      await navigator.clipboard.writeText(formatted);
      copied = true;
      notifySuccess('Copied to clipboard');
      setTimeout(() => {
        copied = false;
      }, 2000);
    } catch (e) {
      console.error(e);
    }
  }
</script>

<div class="codebox-wrapper {className}">
  <div class="codebox-toolbar">
    <span class="codebox-lang">{language}</span>
    <button class="copy-btn" onclick={copyToClipboard} title="Copy code">
      {#if copied}
        <Icon name="check" size={12} class="copied-icon" />
        <span>Copied</span>
      {:else}
        <Icon name="copy" size={12} />
        <span>Copy</span>
      {/if}
    </button>
  </div>
  <pre class="codebox-pre" style="max-height: {maxHeight}"><code>{formatted}</code></pre>
</div>

<style>
  .codebox-wrapper {
    background: #090a0f;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    position: relative;
    overflow: hidden;
  }

  .codebox-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 4px 10px;
    background: var(--bg-surface-elevated);
    border-bottom: 1px solid var(--border-subtle);
  }

  .codebox-lang {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .copy-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-family: var(--font-mono);
    font-size: 10.5px;
    color: var(--text-muted);
    cursor: pointer;
    padding: 2px 6px;
    border-radius: var(--radius-xs);
    transition: all 0.12s;
  }
  .copy-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  :global(.copied-icon) {
    color: var(--accent-emerald);
  }

  .codebox-pre {
    padding: 10px 12px;
    overflow-x: auto;
    overflow-y: auto;
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1.5;
    color: var(--text-secondary);
    margin: 0;
  }

  .codebox-pre code {
    background: transparent;
    padding: 0;
    border: none;
    color: inherit;
    font-family: inherit;
    font-size: inherit;
    white-space: pre;
  }
</style>
