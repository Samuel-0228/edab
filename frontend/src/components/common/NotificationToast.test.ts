import { describe, it, expect, afterEach } from 'vitest';
import { mount, unmount } from 'svelte';
import NotificationToast from './NotificationToast.svelte';
import { notifications, addToast } from '../../api/stores';

describe('NotificationToast Component', () => {
  let target: HTMLElement;
  let instance: any = null;

  afterEach(() => {
    if (instance) {
      unmount(instance);
      instance = null;
    }
    if (target && target.parentNode) {
      target.parentNode.removeChild(target);
    }
    notifications.set([]);
  });

  it('renders notifications with correct types and dismisses when close is clicked', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(NotificationToast, { target });

    addToast('success', 'Operation succeeded', 'Done', 0);
    addToast('error', 'Operation failed', 'Error', 0);
    addToast('warning', 'Be careful', 'Warning', 0);
    addToast('info', 'For your info', 'Info', 0);

    await new Promise((r) => setTimeout(r, 10));

    const toastItems = target.querySelectorAll('.toast-item');
    expect(toastItems.length).toBe(4);

    expect(target.querySelector('.toast-item.success')).toBeTruthy();
    expect(target.querySelector('.toast-item.error')).toBeTruthy();
    expect(target.querySelector('.toast-item.warning')).toBeTruthy();
    expect(target.querySelector('.toast-item.info')).toBeTruthy();

    const firstMsg = target.querySelector('.toast-message');
    expect(firstMsg?.textContent).toContain('For your info');

    const closeBtn = target.querySelector<HTMLButtonElement>('.toast-close');
    expect(closeBtn).toBeTruthy();
    closeBtn?.click();

    await new Promise((r) => setTimeout(r, 10));
    expect(target.querySelectorAll('.toast-item').length).toBe(3);
  });
});
