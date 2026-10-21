import { afterEach, expect, test, vi } from 'vitest';
import { createNotifier } from './notifier.ts';

afterEach(() => vi.useRealTimers());

test('notifications before a wait coalesce into one retained wake', async () => {
  const notifier = createNotifier();
  notifier.notify();
  notifier.notify();
  expect(await notifier.wait()).toBe(true);
  expect(await notifier.wait({ timeoutMs: 0 })).toBe(false);
});

test('notification wakes a waiter and removes its timer and abort listener', async () => {
  vi.useFakeTimers();
  const notifier = createNotifier();
  const controller = new AbortController();
  const removed = vi.spyOn(controller.signal, 'removeEventListener');
  const waiting = notifier.wait({ signal: controller.signal, timeoutMs: 1_000 });
  expect(vi.getTimerCount()).toBe(1);
  notifier.notify();
  expect(await waiting).toBe(true);
  expect(vi.getTimerCount()).toBe(0);
  expect(removed).toHaveBeenCalledOnce();
  // A late abort of a finished wait must not clear a subsequent waiter.
  const next = notifier.wait();
  controller.abort();
  notifier.notify();
  expect(await next).toBe(true);
});

test('a timeout removes its listener and permits the next wait', async () => {
  vi.useFakeTimers();
  const notifier = createNotifier();
  const controller = new AbortController();
  const removed = vi.spyOn(controller.signal, 'removeEventListener');
  const waiting = notifier.wait({ signal: controller.signal, timeoutMs: 100 });
  await vi.advanceTimersByTimeAsync(100);
  expect(await waiting).toBe(false);
  expect(removed).toHaveBeenCalledOnce();
  notifier.notify();
  expect(await notifier.wait()).toBe(true);
});

test('cancellation rejects the wait and releases its resources', async () => {
  vi.useFakeTimers();
  const notifier = createNotifier();
  const controller = new AbortController();
  const removed = vi.spyOn(controller.signal, 'removeEventListener');
  const waiting = notifier.wait({ signal: controller.signal, timeoutMs: 100 });
  const reason = new Error('cancel');
  const rejected = expect(waiting).rejects.toBe(reason);
  controller.abort(reason);
  await rejected;
  expect(vi.getTimerCount()).toBe(0);
  expect(removed).toHaveBeenCalledOnce();
  notifier.notify();
  expect(await notifier.wait()).toBe(true);
});

test('an already cancelled wait does not consume a retained notification', async () => {
  const notifier = createNotifier();
  notifier.notify();
  const reason = new Error('cancel');
  await expect(notifier.wait({ signal: AbortSignal.abort(reason) })).rejects.toBe(reason);
  expect(await notifier.wait()).toBe(true);
});

test('concurrent waits reject without replacing the first waiter', async () => {
  const notifier = createNotifier();
  const first = notifier.wait();
  await expect(notifier.wait()).rejects.toThrow('already has a pending wait');
  notifier.notify();
  expect(await first).toBe(true);
});

test.each([-1, NaN, Infinity, 2_147_483_648])(
  'invalid timeout %s fails before consuming a wake',
  async (timeoutMs) => {
    const notifier = createNotifier();
    notifier.notify();
    await expect(notifier.wait({ timeoutMs })).rejects.toBeInstanceOf(RangeError);
    expect(await notifier.wait()).toBe(true);
  }
);
