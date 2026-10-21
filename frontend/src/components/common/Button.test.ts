import { describe, it, expect, afterEach, vi } from 'vitest';
import { mount, unmount } from 'svelte';
import Button from './Button.svelte';

describe('Button Component', () => {
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

  it('renders button with primary variant and handles click event', () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    const handleClick = vi.fn();

    instance = mount(Button, {
      target,
      props: {
        variant: 'primary',
        size: 'md',
        onclick: handleClick
      }
    });

    const button = target.querySelector('button');
    expect(button).toBeTruthy();
    expect(button?.classList.contains('primary')).toBe(true);
    expect(button?.classList.contains('size-md')).toBe(true);

    button?.click();
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('disables button when loading or disabled prop is true', () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(Button, {
      target,
      props: {
        loading: true
      }
    });

    const button = target.querySelector('button');
    expect(button?.disabled).toBe(true);
    expect(target.querySelector('.btn-spinner')).toBeTruthy();
  });
});
