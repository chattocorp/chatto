import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import type { MessageResource } from '$lib/api-client/messageResources';
import { MessageReconciler } from './messageReconciler';

function resource(id: string): MessageResource {
  return { message: new Message({ id }), timeline: null };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('MessageReconciler', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('deduplicates bursts, bounds batches and uses cursor arrival order', async () => {
    const read = vi.fn(async (_room: string, ids: string[]) => ids.map(resource));
    const apply = vi.fn();
    const queue = new MessageReconciler(read, () => apply);
    for (let id = 0; id < 205; id++) void queue.enqueue('room', `${id}`, true, 'z-first');
    const completion = queue.enqueue('room', '0', false, 'a-last');
    await vi.advanceTimersByTimeAsync(10);
    await completion;
    expect(read.mock.calls.map(([, ids]) => ids.length)).toEqual([100, 100, 5]);
    expect(read).toHaveBeenNthCalledWith(1, 'room', expect.any(Array), 'a-last');
    expect(apply).toHaveBeenCalledTimes(205);
    expect(apply).toHaveBeenCalledWith('0', expect.anything(), true);
  });

  it('keeps a change queued between drain completion and promise settlement', async () => {
    const read = vi.fn(async (_room: string, ids: string[]) => ids.map(resource));
    const queue = new MessageReconciler(read, () => (id) => {
      if (id === 'first')
        queueMicrotask(() => {
          void queue.enqueue('room', 'second', true);
        });
    });
    const first = queue.enqueue('room', 'first', true);
    await vi.runAllTimersAsync();
    await first;
    expect(read.mock.calls.map(([, ids]) => ids)).toEqual([['first'], ['second']]);
  });

  it('shares completion and suppresses a response superseded during its read', async () => {
    const pending = deferred<MessageResource[]>();
    const read = vi
      .fn()
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValue([resource('post')]);
    const apply = vi.fn();
    const queue = new MessageReconciler(read, () => apply);
    const first = queue.enqueue('room', 'post', true, 'first');
    await vi.advanceTimersByTimeAsync(10);
    const second = queue.enqueue('room', 'post', false, 'second');
    expect(first).toBe(second);
    pending.resolve([resource('post')]);
    await second;
    expect(read).toHaveBeenCalledTimes(2);
    expect(read).toHaveBeenLastCalledWith('room', ['post'], 'second');
    expect(apply).toHaveBeenCalledExactlyOnceWith('post', expect.anything(), true);
  });

  it.each(['room', 'server'])('fences old reads after a %s reset', async (scope) => {
    const pending = deferred<MessageResource[]>();
    const apply = vi.fn();
    const queue = new MessageReconciler(
      () => pending.promise,
      () => apply
    );
    const completion = queue.enqueue('room', 'post', true);
    await vi.advanceTimersByTimeAsync(10);
    if (scope === 'room') queue.invalidateRoom('room');
    else queue.reset();
    pending.resolve([resource('post')]);
    await completion;
    expect(apply).not.toHaveBeenCalled();
  });

  it('resolves newly discovered thread and echo references without cycling', async () => {
    const read = vi.fn(async (_room: string, ids: string[]) =>
      ids.map((id) => ({
        message: new Message({
          id,
          threadRootEventId: id === 'reply' ? 'root' : '',
          channelEchoEventId: id === 'reply' ? 'echo' : '',
          echoOfEventId: id === 'echo' ? 'reply' : ''
        }),
        timeline: null
      }))
    );
    const apply = vi.fn();
    const queue = new MessageReconciler(read, () => apply);
    const completion = queue.enqueue('room', 'reply', false, 'cursor');
    await vi.advanceTimersByTimeAsync(10);
    await completion;
    expect(read.mock.calls.map(([, ids]) => ids)).toEqual([['reply'], ['root', 'echo']]);
    expect(apply).toHaveBeenCalledTimes(3);
  });

  it('fails the shared completion and permits replay to retry', async () => {
    const failure = new Error('read failed');
    const read = vi.fn().mockRejectedValueOnce(failure).mockResolvedValue([]);
    const apply = vi.fn();
    const queue = new MessageReconciler(read, () => apply);
    const failed = expect(queue.enqueue('room', 'post', true, 'cursor')).rejects.toBe(failure);
    await vi.advanceTimersByTimeAsync(10);
    await failed;
    const replay = queue.enqueue('room', 'post', true, 'cursor');
    await vi.advanceTimersByTimeAsync(10);
    await replay;
    expect(apply).toHaveBeenCalledExactlyOnceWith('post', null, true);
  });
});
