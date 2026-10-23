import { expect, expectTypeOf, test, vi } from 'vitest';
import { getEventListeners } from 'node:events';
import { awaitWithSignal } from './await-with-signal.ts';

test.each(['before', 'before callback'] as const)(
  'does not start when aborted %s',
  async (when) => {
    const controller = new AbortController();
    const reason = new Error('Stopped');
    const operation = vi.fn(async () => 42);
    if (when === 'before') controller.abort(reason);
    const result = awaitWithSignal(operation, controller.signal);
    controller.abort(reason);
    await expect(result).rejects.toBe(reason);
    expect(operation).not.toHaveBeenCalled();
    expect(getEventListeners(controller.signal, 'abort')).toHaveLength(0);
  }
);

test.each(['success', 'failure', 'throw'] as const)(
  'preserves %s and removes its listener',
  async (outcome) => {
    const controller = new AbortController();
    const reason = new Error('Operation failed');
    const result = awaitWithSignal(() => {
      if (outcome === 'throw') throw reason;
      return outcome === 'success' ? Promise.resolve(42) : Promise.reject(reason);
    }, controller.signal);
    expectTypeOf(result).toEqualTypeOf<Promise<number>>();
    if (outcome === 'success') await expect(result).resolves.toBe(42);
    else await expect(result).rejects.toBe(reason);
    expect(getEventListeners(controller.signal, 'abort')).toHaveLength(0);
  }
);

test.each(['resolve', 'reject'] as const)(
  'abort ends the wait while remote work can later %s',
  async (outcome) => {
    const controller = new AbortController();
    const remote = Promise.withResolvers<number>();
    const started = Promise.withResolvers<void>();
    const result = awaitWithSignal(() => {
      started.resolve();
      return remote.promise;
    }, controller.signal);
    await started.promise;
    const reason = new Error('No longer waiting');
    controller.abort(reason);
    await expect(result).rejects.toBe(reason);
    expect(getEventListeners(controller.signal, 'abort')).toHaveLength(0);
    if (outcome === 'resolve') remote.resolve(42);
    else remote.reject(new Error('Late failure'));
    // A late rejection must remain observed after the caller has stopped waiting.
    await new Promise<void>((resolve) => setImmediate(resolve));
    await expect(result).rejects.toBe(reason);
  }
);

test('observes both failures when the operation aborts and then throws synchronously', async () => {
  const controller = new AbortController();
  const reason = new Error('Stopped');
  await expect(
    awaitWithSignal(() => {
      controller.abort(reason);
      throw new Error('Operation also failed');
    }, controller.signal)
  ).rejects.toBe(reason);
  expect(getEventListeners(controller.signal, 'abort')).toHaveLength(0);
});
