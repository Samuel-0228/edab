import { describe, it, expect, afterEach } from 'vitest';
import { mount, unmount } from 'svelte';
import Badge from './Badge.svelte';

describe('Badge Component', () => {
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

  it('renders with default neutral variant and size', () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(Badge, {
      target,
      props: {
        variant: 'neutral',
        size: 'sm'
      }
    });

    const badge = target.querySelector('.tech-badge');
    expect(badge).toBeTruthy();
    expect(badge?.classList.contains('neutral')).toBe(true);
    expect(badge?.classList.contains('size-sm')).toBe(true);
  });

  it('renders with success variant and dot indicator', () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(Badge, {
      target,
      props: {
        variant: 'success',
        dot: true,
        size: 'md'
      }
    });

    const badge = target.querySelector('.tech-badge');
    expect(badge).toBeTruthy();
    expect(badge?.classList.contains('success')).toBe(true);
    expect(badge?.classList.contains('size-md')).toBe(true);
    expect(badge?.querySelector('.badge-dot')).toBeTruthy();
  });
});
