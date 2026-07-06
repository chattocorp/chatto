import { describe, it, expect, beforeEach, vi } from 'vitest';
import { toast, getToasts } from './toastState.svelte';

describe('toast', () => {
  beforeEach(() => {
    toast.clear();
    vi.useFakeTimers();
  });

  it('adds an error toast', () => {
    toast.error('Something went wrong');
    const toasts = getToasts();
    expect(toasts).toHaveLength(1);
    expect(toasts[0].tone).toBe('error');
    expect(toasts[0].message).toBe('Something went wrong');
  });

  it('adds a success toast', () => {
    toast.success('Message sent');
    const toasts = getToasts();
    expect(toasts).toHaveLength(1);
    expect(toasts[0].tone).toBe('success');
  });

  it('adds an info toast', () => {
    toast.info('Connecting...');
    const toasts = getToasts();
    expect(toasts).toHaveLength(1);
    expect(toasts[0].tone).toBe('info');
  });

  it('adds a warning toast', () => {
    toast.warning('Check your input');
    const toasts = getToasts();
    expect(toasts).toHaveLength(1);
    expect(toasts[0].tone).toBe('warning');
  });

  it('stacks multiple toasts', () => {
    toast.error('Error 1');
    toast.success('Success 1');
    toast.info('Info 1');
    expect(getToasts()).toHaveLength(3);
  });

  it('removes a toast by id', () => {
    const id = toast.error('To be removed');
    expect(getToasts()).toHaveLength(1);
    toast.remove(id);
    expect(getToasts()).toHaveLength(0);
  });

  it('clears all toasts', () => {
    toast.error('Error 1');
    toast.success('Success 1');
    expect(getToasts()).toHaveLength(2);
    toast.clear();
    expect(getToasts()).toHaveLength(0);
  });

  it('auto-dismisses after default duration', () => {
    toast.error('Will disappear');
    expect(getToasts()).toHaveLength(1);

    vi.advanceTimersByTime(5000);
    expect(getToasts()).toHaveLength(0);
  });

  it('respects custom duration', () => {
    toast.error('Quick toast', 1000);
    expect(getToasts()).toHaveLength(1);

    vi.advanceTimersByTime(999);
    expect(getToasts()).toHaveLength(1);

    vi.advanceTimersByTime(1);
    expect(getToasts()).toHaveLength(0);
  });

  it('does not auto-dismiss when duration is 0', () => {
    toast.error('Persistent toast', 0);
    expect(getToasts()).toHaveLength(1);

    vi.advanceTimersByTime(10000);
    expect(getToasts()).toHaveLength(1);
  });

  it('supports action with label and onClick', () => {
    const onClick = vi.fn();
    toast.info('New version available', 0, { label: 'Reload', onClick });
    const toasts = getToasts();
    expect(toasts).toHaveLength(1);
    expect(toasts[0].action).toBeDefined();
    expect(toasts[0].action?.label).toBe('Reload');
    toasts[0].action?.onClick();
    expect(onClick).toHaveBeenCalled();
  });
});
