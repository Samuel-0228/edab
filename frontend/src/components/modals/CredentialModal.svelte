<script lang="ts">
  import {
    credentialModal,
    closeAddCredentialModal,
    triggerRefresh,
    notifySuccess,
    notifyError
  } from '../../api/stores';
  import { api } from '../../api/client';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';
  import Input from '../common/Input.svelte';
  import Select from '../common/Select.svelte';

  let name = $state('');
  let integration = $state('telegram');
  let key = $state('bot_token');
  let secret = $state('');
  let loading = $state(false);

  const integrationOptions = [
    { value: 'telegram', label: 'Telegram (bot_token)' },
    { value: 'discord', label: 'Discord (webhook_url, bot_token)' },
    { value: 'github', label: 'GitHub (personal_access_token, webhook_secret)' },
    { value: 'http', label: 'Generic HTTP (api_key, bearer_token)' }
  ];

  $effect(() => {
    if ($credentialModal.open) {
      name = '';
      secret = '';
      if ($credentialModal.preselectedIntegration) {
        integration = $credentialModal.preselectedIntegration;
      }
      updateKeyDefault();
    }
  });

  function updateKeyDefault() {
    if (integration === 'telegram') key = 'bot_token';
    else if (integration === 'discord') key = 'webhook_url';
    else if (integration === 'github') key = 'personal_access_token';
    else key = 'api_key';
  }

  async function handleSubmit() {
    if (!name.trim()) {
      notifyError('Credential name is required');
      return;
    }
    if (!secret.trim()) {
      notifyError('Secret token is required');
      return;
    }

    loading = true;
    try {
      await api.createCredential({
        name: name.trim(),
        integration,
        values: {
          [key.trim() || 'token']: secret.trim()
        }
      });
      notifySuccess(`Credential "${name}" encrypted and stored in vault`);
      triggerRefresh();
      closeAddCredentialModal();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to store credential';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }
</script>

<Dialog
  open={$credentialModal.open}
  title="Store Encrypted Credential"
  subtitle="Secrets are encrypted at rest with AES-256-GCM and never exposed via standard API endpoints."
  maxWidth="500px"
  onclose={closeAddCredentialModal}
>
  <div class="form-grid">
    <Input
      label="Credential Name"
      placeholder="e.g. Production Telegram Bot Token"
      bind:value={name}
      required
    />

    <Select
      label="Target Integration"
      options={integrationOptions}
      bind:value={integration}
      onchange={updateKeyDefault}
      required
    />

    <Input
      label="Key Identifier"
      placeholder="e.g. bot_token or api_key"
      bind:value={key}
      mono
      hint="Internal key attribute name passed to the integration adapter."
      required
    />

    <Input
      type="password"
      label="Secret Value"
      placeholder="Sensitive token / secret key"
      bind:value={secret}
      mono
      hint="AES-256-GCM symmetric encryption secures this value at rest in PostgreSQL."
      required
    />
  </div>

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeAddCredentialModal} disabled={loading}>
      Cancel
    </Button>
    <Button variant="primary" size="sm" {loading} onclick={handleSubmit}>Encrypt & Store</Button>
  {/snippet}
</Dialog>

<style>
  .form-grid {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
</style>
