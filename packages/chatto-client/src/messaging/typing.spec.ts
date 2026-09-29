// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { startTyping, withTyping } from './typing.js';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('withTyping', () => {
  it('runs the work alone without an update', async () => {
    await expect(
      withTyping(new AbortController().signal, undefined, async () => 'done')
    ).resolves.toBe('done');
  });

  it('refreshes at once and three seconds after each refresh, never overlapping', async () => {
    const started: number[] = [];
    let finishUpdate!: () => void;
    const update = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          started.push(Date.now());
          finishUpdate = resolve;
        })
    );
    let finishWork!: (value: string) => void;
    const work = new Promise<string>((resolve) => (finishWork = resolve));
    const running = withTyping(new AbortController().signal, update, () => work);

    expect(update).toHaveBeenCalledOnce();
    // A slow refresh delays the next one: no second request while one runs.
    await vi.advanceTimersByTimeAsync(5000);
    expect(update).toHaveBeenCalledOnce();
    finishUpdate();
    await vi.advanceTimersByTimeAsync(2999);
    expect(update).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    expect(update).toHaveBeenCalledTimes(2);
    expect(started[1]! - started[0]!).toBe(8000);

    finishWork('answer');
    await expect(running).resolves.toBe('answer');
  });

  it('aborts the refresh in flight and schedules none after the work ends', async () => {
    const signals: AbortSignal[] = [];
    const update = vi.fn(async (signal: AbortSignal) => {
      signals.push(signal);
    });
    await withTyping(new AbortController().signal, update, async () => 'done');
    expect(signals[0]!.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(update).toHaveBeenCalledOnce();
  });

  it('keeps the work result and error when refreshes fail', async () => {
    const update = vi.fn(async () => {
      throw new Error('typing unavailable');
    });
    await expect(
      withTyping(new AbortController().signal, update, async () => 'done')
    ).resolves.toBe('done');
    await expect(
      withTyping(new AbortController().signal, update, async () => {
        throw new Error('work failed');
      })
    ).rejects.toThrow('work failed');
  });

  it('keeps refreshing after a failed refresh while the work runs', async () => {
    const update = vi.fn(async () => {
      throw new Error('typing unavailable');
    });
    let finishWork!: () => void;
    const running = withTyping(
      new AbortController().signal,
      update,
      () => new Promise<void>((resolve) => (finishWork = resolve))
    );
    await vi.advanceTimersByTimeAsync(3000);
    expect(update).toHaveBeenCalledTimes(2);
    finishWork();
    await running;
  });

  it('stops refreshing when the caller aborts, while the work continues', async () => {
    const caller = new AbortController();
    const update = vi.fn(async () => {});
    let finishWork!: () => void;
    const running = withTyping(
      caller.signal,
      update,
      () => new Promise<void>((resolve) => (finishWork = resolve))
    );
    await vi.advanceTimersByTimeAsync(0);
    caller.abort();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(update).toHaveBeenCalledOnce();
    finishWork();
    await running;
  });
});

describe('startTyping', () => {
  it('waits for the first update and refreshes every three seconds until stopped', async () => {
    const update = vi.fn(async () => {});
    const stop = await startTyping(update);
    expect(update).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(6000);
    expect(update).toHaveBeenCalledTimes(3);
    stop();
    stop();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(update).toHaveBeenCalledTimes(3);
  });

  it('ignores failed updates', async () => {
    const update = vi.fn(async () => {
      throw new Error('typing unavailable');
    });
    const stop = await startTyping(update);
    await vi.advanceTimersByTimeAsync(3000);
    expect(update).toHaveBeenCalledTimes(2);
    stop();
  });

  it('schedules no refresh after a stop during an update in flight', async () => {
    let finish!: () => void;
    const update = vi
      .fn<() => Promise<void>>()
      .mockResolvedValueOnce()
      .mockImplementationOnce(() => new Promise<void>((resolve) => (finish = resolve)));
    const stop = await startTyping(update);
    await vi.advanceTimersByTimeAsync(3000);
    expect(update).toHaveBeenCalledTimes(2);
    stop();
    finish();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(update).toHaveBeenCalledTimes(2);
  });
});
