import { expect, it } from 'vitest';
import { untilAborted } from './abort.ts';

it('cancels one waiter while another receives the shared result', async () => {
  let complete!: (value: string) => void;
  const work = new Promise<string>((resolve) => {
    complete = resolve;
  });
  const controller = new AbortController();
  const cancelled = untilAborted(work, controller.signal);
  const retained = untilAborted(work);
  controller.abort(new Error('Stopped'));
  await expect(cancelled).rejects.toThrow('Stopped');
  complete('result');
  await expect(retained).resolves.toBe('result');
});

it('handles pre-aborted signals and preserves ordinary failures', async () => {
  await expect(
    untilAborted(Promise.resolve('unused'), AbortSignal.abort(new Error('Already stopped')))
  ).rejects.toThrow('Already stopped');
  await expect(
    untilAborted(Promise.reject(new Error('Failed')), new AbortController().signal)
  ).rejects.toThrow('Failed');
});
