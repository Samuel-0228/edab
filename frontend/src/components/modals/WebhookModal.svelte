<script lang="ts">
  import {
    webhookModal,
    closeCreateWebhookModal,
    triggerRefresh,
    notifySuccess,
    notifyError
  } from '../../api/stores';
  import { api } from '../../api/client';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';
  import Input from '../common/Input.svelte';

  let name = $state('');
  let eventType = $state('webhook.received');
  let secretToken = $state('');
  let loading = $state(false);

  $effect(() => {
    if ($webhookModal) {
      name = '';
      eventType = 'webhook.received';
      secretToken = '';
    }
  });

  async function handleSubmit() {
    if (!name.trim()) {
      notifyError('Webhook name is required');
      return;
    }

    loading = true;
    try {
      const wh = await api.createWebhook({
        name: name.trim(),
        event_type: eventType.trim() || 'webhook.received',
        secret_token: secretToken.trim() || undefined,
        enabled: true
      });
      notifySuccess(`Webhook endpoint "${wh.name}" created`);
      triggerRefresh();
      closeCreateWebhookModal();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to create webhook endpoint';
      notifyError(msg);
    } finally {
      loading = false;
    }
  }
</script>

<Dialog
  open={$webhookModal}
  title="Create Inbound Webhook Endpoint"
  subtitle="Generate an HTTP ingestion URL that receives JSON payloads and normalizes them into Kerob events."
  maxWidth="500px"
  onclose={closeCreateWebhookModal}
>
  <div class="form-grid">
    <Input
      label="Endpoint Name"
      placeholder="e.g. Stripe Payment Webhook"
      bind:value={name}
      required
    />

    <Input
      label="Event Type Tag"
      placeholder="e.g. payment.success or github.push"
      bind:value={eventType}
      mono
      hint="Workflows can bind trigger rules to this event type."
      required
    />

    <Input
      type="password"
      label="Secret Token (Optional Authentication)"
      placeholder="Optional bearer token or secret header"
      bind:value={secretToken}
      mono
      hint="When set, incoming webhook calls must provide this secret for authentication."
    />
  </div>

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeCreateWebhookModal} disabled={loading}>
      Cancel
    </Button>
    <Button variant="primary" size="sm" {loading} onclick={handleSubmit}>Create Endpoint</Button>
  {/snippet}
</Dialog>

<style>
  .form-grid {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
</style>
