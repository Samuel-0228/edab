<script lang="ts">
  import Badge from '../common/Badge.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';
  import SchemaConfigForm from './SchemaConfigForm.svelte';
  import type { SelectOption } from '../common/Select.svelte';
  import type { Credential, IntegrationJSONSchema } from '../../api/types';

  interface Props {
    actionIntegration: string;
    actionType: string;
    actionCredentialId: string;
    actionConfigRaw: string;
    actionSchema: IntegrationJSONSchema | null;
    maxRetries: number;
    integrations: SelectOption[];
    actionTypes: SelectOption[];
    credentials: Credential[];
    onIntegrationChange: () => void;
    onTypeChange: () => void;
  }

  let {
    actionIntegration = $bindable('discord'),
    actionType = $bindable('send_message'),
    actionCredentialId = $bindable(''),
    actionConfigRaw = $bindable('{}'),
    actionSchema = null,
    maxRetries = $bindable(3),
    integrations = [],
    actionTypes = [],
    credentials = [],
    onIntegrationChange,
    onTypeChange
  }: Props = $props();

  let credentialOptions = $derived.by(() => [
    { value: '', label: '(None / Inline Credentials)' },
    ...credentials.map((c) => ({
      value: c.id,
      label: `${c.name} (${c.integration})`
    }))
  ]);
</script>

<div class="stage-card">
  <div class="stage-header">
    <Badge variant="stage-then">THEN</Badge>
    <span class="stage-title">Action Execution</span>
  </div>

  <div class="stage-fields-grid">
    <Select
      label="Integration Adapter"
      options={integrations}
      bind:value={actionIntegration}
      onchange={onIntegrationChange}
    />

    <Select
      label="Action Operation"
      options={actionTypes}
      bind:value={actionType}
      onchange={onTypeChange}
    />
  </div>

  <SchemaConfigForm
    schema={actionSchema}
    bind:value={actionConfigRaw}
    label="Action Configuration"
    required
  />

  <div class="stage-fields-grid" style="margin-top: 8px;">
    <Select
      label="Encrypted Credential (Optional)"
      bind:value={actionCredentialId}
      options={credentialOptions}
    />

    <Input
      type="number"
      label="Max Retries"
      bind:value={maxRetries}
      mono
      hint="Exponential backoff retries before execution lands in Dead-Letter Queue."
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

  .stage-fields-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
</style>
