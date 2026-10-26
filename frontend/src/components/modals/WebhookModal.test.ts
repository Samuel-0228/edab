import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import WebhookModal from './WebhookModal.svelte';
import { openCreateWebhookModal, closeCreateWebhookModal, notifications } from '../../api/stores';
import { api } from '../../api/client';

describe('WebhookModal Component', () => {
  let target: HTMLElement;
  let instance: any = null;

  beforeEach(() => {
    notifications.set([]);
  });

  afterEach(() => {
    if (instance) {
      unmount(instance);
      instance = null;
    }
    if (target && target.parentNode) {
      target.parentNode.removeChild(target);
    }
    closeCreateWebhookModal();
  });

  it('handles validation and creates webhook endpoint', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(WebhookModal, { target });

    openCreateWebhookModal();
    await new Promise((r) => setTimeout(r, 20));

    expect(target.querySelector('.dialog-panel')).toBeTruthy();

    const submitBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Create Endpoint')
    );
    expect(submitBtn).toBeTruthy();

    // Submit empty name -> error
    submitBtn?.click();
    await new Promise((r) => setTimeout(r, 20));

    let currentNotifications: any[] = [];
    notifications.subscribe((n) => (currentNotifications = n))();
    expect(currentNotifications.some((n) => n.message.includes('Webhook name is required'))).toBe(
      true
    );

    // Enter name and submit
    const inputs = target.querySelectorAll('input');
    const nameInput = inputs[0];
    nameInput.value = 'GitHub Inbound';
    nameInput.dispatchEvent(new Event('input'));

    const mockCreate = vi.spyOn(api, 'createWebhook').mockResolvedValue({
      id: 'wh-123',
      name: 'GitHub Inbound',
      event_type: 'webhook.received',
      enabled: true,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString()
    });

    submitBtn?.click();
    await new Promise((r) => setTimeout(r, 30));

    expect(mockCreate).toHaveBeenCalledTimes(1);
  });
});
