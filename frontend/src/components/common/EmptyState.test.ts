import { describe, it, expect, afterEach } from 'vitest';
import { mount, unmount } from 'svelte';
import EmptyState from './EmptyState.svelte';

describe('EmptyState Component', () => {
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
  });

  it('renders title and description', () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(EmptyState, {
      target,
      props: {
        title: 'No Workflows Found',
        description: 'Create your first workflow to automate events.',
        icon: 'workflow'
      }
    });

    const titleEl = target.querySelector('.empty-title');
    const descEl = target.querySelector('.empty-description');

    expect(titleEl?.textContent).toBe('No Workflows Found');
    expect(descEl?.textContent).toBe('Create your first workflow to automate events.');
  });
});
