<script lang="ts">
  interface Props {
    checked?: boolean;
    label?: string;
    description?: string;
    disabled?: boolean;
    class?: string;
    onchange?: (checked: boolean) => void;
  }

  let {
    checked = $bindable(false),
    label,
    description,
    disabled = false,
    class: className = '',
    onchange
  }: Props = $props();

  function toggle() {
    if (disabled) return;
    checked = !checked;
    onchange?.(checked);
  }
</script>

<div
  class="tech-switch-row {disabled ? 'disabled' : ''} {className}"
  role="button"
  tabindex="0"
  onclick={toggle}
  onkeydown={(e) => { if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); toggle(); } }}
>
  <button
    type="button"
    role="switch"
    aria-checked={checked}
    aria-label={label || 'Toggle switch'}
    {disabled}
    class="switch-track {checked ? 'checked' : ''}"
  >
    <span class="switch-thumb"></span>
  </button>

  {#if label || description}
    <div class="switch-meta">
      {#if label}
        <span class="switch-label">{label}</span>
      {/if}
      {#if description}
        <span class="switch-description">{description}</span>
      {/if}
    </div>
  {/if}
</div>

<style>
  .tech-switch-row {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    cursor: pointer;
    user-select: none;
  }

  .tech-switch-row.disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .switch-track {
    width: 32px;
    height: 18px;
    background: var(--bg-surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: 9px;
    position: relative;
    padding: 1px;
    cursor: pointer;
    transition: all 0.15s ease-in-out;
  }

  .switch-track.checked {
    background: var(--accent-emerald-dim);
    border-color: var(--accent-emerald-border);
  }

  .switch-thumb {
    display: block;
    width: 14px;
    height: 14px;
    background: var(--text-muted);
    border-radius: 50%;
    transition: transform 0.15s ease-in-out, background 0.15s ease-in-out;
  }

  .switch-track.checked .switch-thumb {
    transform: translateX(14px);
    background: var(--accent-emerald);
    box-shadow: 0 0 6px rgba(0, 229, 153, 0.4);
  }

  .switch-meta {
    display: flex;
    flex-direction: column;
  }

  .switch-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-primary);
  }

  .switch-description {
    font-size: 11px;
    color: var(--text-muted);
  }
</style>
