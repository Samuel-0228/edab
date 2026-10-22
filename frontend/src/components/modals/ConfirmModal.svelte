<script lang="ts">
  import { confirmModal, closeConfirm } from '../../api/stores';
  import Dialog from '../common/Dialog.svelte';
  import Button from '../common/Button.svelte';

  let loading = $state(false);

  async function handleConfirm() {
    if (!$confirmModal.onConfirm) {
      closeConfirm();
      return;
    }
    loading = true;
    try {
      await $confirmModal.onConfirm();
      closeConfirm();
    } catch (e) {
      console.error(e);
    } finally {
      loading = false;
    }
  }
</script>

<Dialog
  open={$confirmModal.open}
  title={$confirmModal.title || 'Confirm Action'}
  maxWidth="440px"
  onclose={closeConfirm}
>
  <p class="confirm-message">{$confirmModal.message}</p>

  {#snippet footer()}
    <Button variant="ghost" size="sm" onclick={closeConfirm} disabled={loading}>Cancel</Button>
    <Button
      variant={$confirmModal.danger ? 'danger' : 'primary'}
      size="sm"
      {loading}
      onclick={handleConfirm}
    >
      {$confirmModal.confirmLabel || 'Confirm'}
    </Button>
  {/snippet}
</Dialog>

<style>
  .confirm-message {
    font-size: 12.5px;
    color: var(--text-secondary);
    line-height: 1.5;
  }
</style>
