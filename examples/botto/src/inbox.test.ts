/** Exercise quiet periods, thread isolation, failure recovery, and shutdown. */
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { AddressedMessage } from '@chatto/client';
import { Inbox } from './inbox.ts';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

function message(id: string, threadRootId = 'thread'): AddressedMessage {
  return { id, roomId: 'room', threadRootId, authorId: 'human', body: id, reasons: ['mention'] };
}

test('waits five seconds after the last message and preserves authors and order', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  const one = message('one');
  const two = { ...message('two'), authorId: 'another-human' };
  inbox.accept(one);
  await vi.advanceTimersByTimeAsync(4000);
  inbox.accept(two);
  await vi.advanceTimersByTimeAsync(4999);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledExactlyOnceWith([one, two]);
  await inbox.close();
});

test('keeps DM threads, top-level posts, and rooms separate', async () => {
  const batches: string[][] = [];
  const inbox = new Inbox(async (items) => {
    batches.push(items.map((item) => item.id));
  }, vi.fn());
  inbox.accept({ ...message('one', 'dm-thread'), reasons: ['direct_message'] });
  inbox.accept({ ...message('two', 'dm-thread'), reasons: ['direct_message'] });
  inbox.accept({
    ...message('other-room', 'dm-thread'),
    roomId: 'other',
    reasons: ['direct_message']
  });
  inbox.accept({ ...message('root-one'), threadRootId: undefined });
  inbox.accept({ ...message('root-two'), threadRootId: undefined });
  await vi.advanceTimersByTimeAsync(5000);
  expect(batches).toEqual([['one', 'two'], ['other-room'], ['root-one'], ['root-two']]);
  await inbox.close();
});

test('DM roots start immediately while DM thread replies still debounce', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  const root: AddressedMessage = {
    ...message('root'),
    threadRootId: undefined,
    reasons: ['direct_message']
  };
  const reply: AddressedMessage = {
    ...message('reply', 'root'),
    reasons: ['direct_message']
  };
  inbox.accept(root);
  await vi.advanceTimersByTimeAsync(0);
  expect(turn).toHaveBeenCalledExactlyOnceWith([root]);
  inbox.accept(reply);
  await vi.advanceTimersByTimeAsync(4999);
  expect(turn).toHaveBeenCalledOnce();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenLastCalledWith([reply]);
  expect(turn).toHaveBeenCalledTimes(2);
  await inbox.close();
});

test('duplicates do not reset the quiet period', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(4000);
  expect(inbox.accept(message('one'))).toBe('duplicate');
  await vi.advanceTimersByTimeAsync(1000);
  expect(turn).toHaveBeenCalledOnce();
  await inbox.close();
});

test('applies configured debounce and queue limits', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn(), { debounceMs: 250, maxThreads: 1, maxPendingMessages: 2 });
  expect(inbox.accept(message('one'))).toBe('accepted');
  expect(inbox.accept(message('other', 'other-thread'))).toBe('full');
  expect(inbox.accept(message('two'))).toBe('accepted');
  expect(inbox.accept(message('three'))).toBe('full');
  await vi.advanceTimersByTimeAsync(249);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledExactlyOnceWith([message('one'), message('two')]);
  expect(inbox.accept(message('other', 'other-thread'))).toBe('accepted');
  await inbox.close();
});

test('zero debounce starts a thread batch without a quiet-period delay', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn(), { debounceMs: 0, maxThreads: 1, maxPendingMessages: 2 });
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(0);
  expect(turn).toHaveBeenCalledExactlyOnceWith([message('one')]);
  await inbox.close();
});

test('debounces while busy, serializes turns, and allows other threads to proceed', async () => {
  const first = Promise.withResolvers<void>();
  const batches: string[][] = [];
  const inbox = new Inbox(async (items) => {
    batches.push(items.map((item) => item.id));
    if (items[0]!.id === 'one') await first.promise;
  }, vi.fn());
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(5000);
  inbox.accept(message('two'));
  inbox.accept(message('three'));
  inbox.accept(message('other', 'other-thread'));
  await vi.advanceTimersByTimeAsync(5000);
  expect(batches).toEqual([['one'], ['other']]);
  first.resolve();
  await vi.advanceTimersByTimeAsync(0);
  expect(batches).toEqual([['one'], ['other'], ['two', 'three']]);
  await inbox.close();
});

test('a failed turn does not bypass the next batch quiet period', async () => {
  const first = Promise.withResolvers<void>();
  const failed = vi.fn();
  const completed = vi.fn();
  const inbox = new Inbox(async (items) => {
    if (items[0]!.id === 'bad') {
      await first.promise;
      throw new Error('private payload');
    }
    completed();
  }, failed);
  inbox.accept(message('bad'));
  await vi.advanceTimersByTimeAsync(5000);
  inbox.accept(message('good'));
  first.resolve();
  await vi.advanceTimersByTimeAsync(4999);
  expect(failed).toHaveBeenCalledWith();
  expect(completed).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(completed).toHaveBeenCalledOnce();
  await inbox.close();
});

test('shutdown waits for active work and discards timers and queued batches', async () => {
  const active = Promise.withResolvers<void>();
  const turn = vi.fn(() => active.promise);
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(5000);
  inbox.accept(message('two'));
  inbox.accept(message('other', 'other-thread'));
  const closed = inbox.close();
  expect(inbox.accept(message('three'))).toBe('closed');
  active.resolve();
  await closed;
  await vi.advanceTimersByTimeAsync(5000);
  expect(turn).toHaveBeenCalledOnce();
  expect(vi.getTimerCount()).toBe(0);
});

test('shutdown discards a batch scheduled for a turn that has not started', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept({ ...message('root'), threadRootId: undefined, reasons: ['direct_message'] });
  // Immediate DM delivery schedules a promise callback; close before that callback runs.
  await inbox.close();
  expect(turn).not.toHaveBeenCalled();
});

test('bounds threads and messages even when they are still debouncing', async () => {
  const inbox = new Inbox(async () => {}, vi.fn());
  for (let index = 0; index < 8; index++)
    expect(inbox.accept(message(`first-${index}`, `thread-${index}`))).toBe('accepted');
  expect(inbox.accept(message('overflow', 'thread-9'))).toBe('full');
  for (let index = 0; index < 92; index++)
    expect(inbox.accept(message(`queued-${index}`, 'thread-0'))).toBe('accepted');
  expect(inbox.accept(message('overflow-queue', 'thread-0'))).toBe('full');
  await inbox.close();
  expect(vi.getTimerCount()).toBe(0);
});
