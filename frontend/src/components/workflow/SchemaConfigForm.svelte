<script lang="ts">
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import JsonEditor from '../common/JsonEditor.svelte';
  import Button from '../common/Button.svelte';
  import Badge from '../common/Badge.svelte';
  import type { IntegrationJSONSchema } from '../../api/types';

  interface Props {
    schema?: IntegrationJSONSchema | null;
    value?: string;
    label?: string;
    hint?: string;
    required?: boolean;
  }

  let {
    schema = null,
    value = $bindable('{}'),
    label = 'Action Configuration',
    hint = 'Configure parameters according to the integration manifest schema.',
    required = false
  }: Props = $props();

  let mode = $state<'form' | 'json'>('form');
  let formData = $state<Record<string, unknown>>({});
  let parseError = $state<string | null>(null);

  // Sync JSON string into formData when value or schema changes
  $effect(() => {
    try {
      const parsed = JSON.parse(value || '{}');
      if (typeof parsed === 'object' && parsed !== null) {
        formData = { ...parsed };
        parseError = null;
      }
    } catch (e) {
      parseError = e instanceof Error ? e.message : 'Invalid JSON';
    }
  });

  function updateField(key: string, val: unknown) {
    formData[key] = val;
    value = JSON.stringify(formData, null, 2);
  }

  let propertyKeys = $derived.by(() => {
    if (!schema || !schema.properties) return [];
    return Object.keys(schema.properties);
  });

  let hasSchemaFields = $derived(propertyKeys.length > 0);

  function isFieldRequired(key: string): boolean {
    return Array.isArray(schema?.required) && schema.required.includes(key);
  }

  function getFieldTitle(key: string): string {
    const prop = schema?.properties?.[key];
    return prop?.title || key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
  }

  function getFieldHint(key: string): string | undefined {
    const prop = schema?.properties?.[key];
    return prop?.description;
  }
</script>

<div class="schema-form-wrapper">
  <div class="form-header">
    <div class="header-left">
      <span class="form-label">{label}</span>
      {#if required}
        <span class="required-star">*</span>
      {/if}
      {#if hasSchemaFields}
        <Badge variant="neutral">Schema Driven</Badge>
      {/if}
    </div>

    {#if hasSchemaFields}
      <div class="mode-toggle">
        <Button
          variant={mode === 'form' ? 'secondary' : 'ghost'}
          size="sm"
          onclick={() => (mode = 'form')}
        >
          Form View
        </Button>
        <Button
          variant={mode === 'json' ? 'secondary' : 'ghost'}
          size="sm"
          onclick={() => (mode = 'json')}
        >
          Raw JSON
        </Button>
      </div>
    {/if}
  </div>

  {#if hint}
    <p class="form-hint">{hint}</p>
  {/if}

  {#if parseError}
    <p class="error-msg">{parseError}</p>
  {/if}

  {#if mode === 'form' && hasSchemaFields}
    <div class="fields-grid">
      {#each propertyKeys as key (key)}
        {@const prop = schema?.properties?.[key]}
        {@const req = isFieldRequired(key)}
        {@const title = getFieldTitle(key)}
        {@const fieldHint = getFieldHint(key)}

        {#if prop?.enum && Array.isArray(prop.enum)}
          <Select
            label={title}
            value={String(formData[key] ?? prop.default ?? '')}
            options={prop.enum.map((opt) => ({
              value: String(opt),
              label: String(opt)
            }))}
            hint={fieldHint}
            required={req}
            onchange={(e) => updateField(key, (e.target as HTMLSelectElement).value)}
          />
        {:else if prop?.type === 'number' || prop?.type === 'integer'}
          <Input
            type="number"
            label={title}
            value={Number(formData[key] ?? prop.default ?? 0)}
            hint={fieldHint}
            required={req}
            mono
            oninput={(e) => updateField(key, Number((e.target as HTMLInputElement).value))}
          />
        {:else if prop?.type === 'object'}
          <JsonEditor
            label={title}
            value={JSON.stringify(formData[key] ?? {}, null, 2)}
            hint={fieldHint}
            minHeight="80px"
            required={req}
            onchange={(val) => {
              try {
                updateField(key, JSON.parse(val));
              } catch {
                // invalid JSON in sub-field ignored until valid
              }
            }}
          />
        {:else}
          <Input
            label={title}
            value={String(formData[key] ?? prop?.default ?? '')}
            hint={fieldHint || 'Supports template placeholders like {{payload.text}}'}
            required={req}
            mono
            oninput={(e) => updateField(key, (e.target as HTMLInputElement).value)}
          />
        {/if}
      {/each}
    </div>
  {:else}
    <JsonEditor
      label=""
      bind:value
      minHeight="120px"
      hint="Valid JSON payload parameters. Supports &#123;&#123;...&#125;&#125; template tags."
      required={required}
    />
  {/if}
</div>

<style>
  .schema-form-wrapper {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 4px;
  }

  .form-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .form-label {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-color, #e0e0e0);
  }

  .required-star {
    color: var(--danger-color, #ef4444);
    font-weight: bold;
  }

  .form-hint {
    font-size: 11px;
    color: var(--text-secondary, #888);
    margin: 0;
  }

  .error-msg {
    font-size: 11px;
    color: var(--danger-color, #ef4444);
    margin: 0;
  }

  .mode-toggle {
    display: flex;
    gap: 4px;
  }

  .fields-grid {
    display: flex;
    flex-direction: column;
    gap: 12px;
    background: var(--card-bg-subtle, rgba(255, 255, 255, 0.02));
    border: 1px solid var(--border-color, #333);
    border-radius: 6px;
    padding: 12px;
  }
</style>
