import { describe, it, expect } from 'vitest';
import { currentRoute, navigate, notifications, addToast, dismissToast } from '../api/stores';
import { get } from 'svelte/store';

describe('Application State & Routing', () => {
  it('updates currentRoute when navigate is called', () => {
    navigate('workflows');
    expect(get(currentRoute)).toBe('workflows');

    navigate('executions');
    expect(get(currentRoute)).toBe('executions');

    navigate('schedules');
    expect(get(currentRoute)).toBe('schedules');

    navigate('settings');
    expect(get(currentRoute)).toBe('settings');
  });

  it('adds and dismisses toast notifications', () => {
    expect(get(notifications).length).toBe(0);

    addToast('success', 'Workflow saved successfully', 'Success', 0);
    expect(get(notifications).length).toBe(1);

    const toast = get(notifications)[0];
    expect(toast.message).toBe('Workflow saved successfully');
    expect(toast.type).toBe('success');

    dismissToast(toast.id);
    expect(get(notifications).length).toBe(0);
  });
});
