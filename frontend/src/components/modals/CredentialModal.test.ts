import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import CredentialModal from './CredentialModal.svelte';
import { openAddCredentialModal, closeAddCredentialModal, notifications } from '../../api/stores';
import { api } from '../../api/client';

describe('CredentialModal Component', () => {
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
    closeAddCredentialModal();
  });

  it('renders modal when open and handles validation and submission', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(CredentialModal, { target });

    openAddCredentialModal('discord');
    await new Promise((r) => setTimeout(r, 20));

    expect(target.querySelector('.dialog-panel')).toBeTruthy();

    const submitBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Encrypt & Store')
    );
    expect(submitBtn).toBeTruthy();

    // 1. Submit with empty name -> triggers validation notification
    submitBtn?.click();
    await new Promise((r) => setTimeout(r, 20));

    let currentNotifications: any[] = [];
    notifications.subscribe((n) => (currentNotifications = n))();
    expect(
      currentNotifications.some((n) => n.message.includes('Credential name is required'))
    ).toBe(true);

    // 2. Fill name and secret, submit
    const inputs = target.querySelectorAll('input');
    const nameInput = inputs[0];
    const secretInput = inputs[2]; // 0: Name, 1: Key, 2: Secret Value

    nameInput.value = 'Production Discord';
    nameInput.dispatchEvent(new Event('input'));

    secretInput.value = 'https://discord.com/api/webhooks/123/xyz';
    secretInput.dispatchEvent(new Event('input'));

    const mockCreate = vi.spyOn(api, 'createCredential').mockResolvedValue({
      id: 'cred-1',
      name: 'Production Discord',
      integration: 'discord',
      key_id: 'default',
      masked_fields: { webhook_url: '••••••••/xyz' },
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString()
    });

    submitBtn?.click();
    await new Promise((r) => setTimeout(r, 30));

    expect(mockCreate).toHaveBeenCalledTimes(1);
  });
});
