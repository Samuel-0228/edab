import { describe, it, expect, afterEach } from 'vitest';
import { mount, unmount } from 'svelte';
import ConfirmModal from './ConfirmModal.svelte';
import { confirmModal, showConfirm } from '../../api/stores';

describe('ConfirmModal Component', () => {
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
    confirmModal.set({ open: false, title: '', message: '' });
  });

  it('renders and displays confirm message when open', async () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(ConfirmModal, { target });

    let confirmed = false;
    showConfirm({
      title: 'Delete Workflow',
      message: 'Are you sure you want to delete this workflow?',
      confirmLabel: 'Delete Forever',
      danger: true,
      onConfirm: async () => {
        confirmed = true;
      }
    });

    await new Promise((r) => setTimeout(r, 10));

    const msg = target.querySelector('.confirm-message');
    expect(msg?.textContent).toBe('Are you sure you want to delete this workflow?');

    const buttons = Array.from(target.querySelectorAll('button'));
    const confirmBtn = buttons.find((b) => b.textContent?.includes('Delete Forever'));
    expect(confirmBtn).toBeTruthy();

    confirmBtn?.click();
    await new Promise((r) => setTimeout(r, 10));
    expect(confirmed).toBe(true);
  });
});
