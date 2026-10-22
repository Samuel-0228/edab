import { describe, it, expect, afterEach } from 'vitest';
import { mount, unmount } from 'svelte';
import StatCard from './StatCard.svelte';

describe('StatCard Component', () => {
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

  it('renders label, value, and subtext correctly', () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(StatCard, {
      target,
      props: {
        label: 'Active Workflows',
        value: 12,
        subtext: '+2 this week',
        indicator: 'positive'
      }
    });

    const labelEl = target.querySelector('.stat-label');
    const valueEl = target.querySelector('.stat-value');
    const subtextEl = target.querySelector('.stat-subtext');
    const dotEl = target.querySelector('.status-dot');

    expect(labelEl?.textContent).toBe('Active Workflows');
    expect(valueEl?.textContent).toBe('12');
    expect(subtextEl?.textContent).toBe('+2 this week');
    expect(dotEl?.classList.contains('positive')).toBe(true);
  });

  it('applies warning indicator and omits subtext when absent', () => {
    target = document.createElement('div');
    document.body.appendChild(target);

    instance = mount(StatCard, {
      target,
      props: {
        label: 'Failures',
        value: 3,
        indicator: 'warning'
      }
    });

    const dotEl = target.querySelector('.status-dot');
    const subtextEl = target.querySelector('.stat-subtext');

    expect(dotEl?.classList.contains('warning')).toBe(true);
    expect(subtextEl).toBeNull();
  });
});
