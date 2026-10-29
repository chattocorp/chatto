/** Exercise quiet periods, thread isolation, failure recovery, and shutdown. */
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { RealtimeEvent, type AddressedMessage } from '@chatto/client';
import { Inbox } from './inbox.ts';
import { DEFAULT_INBOX } from './settings.ts';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

function message(id: string, threadRootId = 'thread'): AddressedMessage {
  return { id, roomId: 'room', threadRootId, authorId: 'human', body: id, reasons: ['mention'] };
}

function typing(roomId = 'room', threadRootEventId = 'thread', actorId = 'human') {
  return new RealtimeEvent({
    actorId,
    event: { case: 'userTyping', value: { roomId, threadRootEventId } }
  });
}

test.each([true, false])('typing resets the quiet period for addressed=%s', async (addressed) => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  const delay = addressed ? 2000 : 5000;
  const item = { ...message('one'), reasons: addressed ? ['mention' as const] : [] };
  inbox.typing(typing(), 'bot'); // Typing alone never queues a turn.
  await vi.advanceTimersByTimeAsync(5000);
  expect(turn).not.toHaveBeenCalled();
  inbox.accept(item);
  await vi.advanceTimersByTimeAsync(delay - 1);
  inbox.typing(typing(), 'bot');
  await vi.advanceTimersByTimeAsync(delay - 1);
  expect(turn).not.toHaveBeenCalled();
  inbox.typing(typing(), 'bot');
  await vi.advanceTimersByTimeAsync(delay - 1);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledExactlyOnceWith([item]);
  inbox.typing(typing(), 'bot');
  await vi.advanceTimersByTimeAsync(5000);
  expect(turn).toHaveBeenCalledOnce();
  await inbox.close();
});

test('unrelated, missing-actor, and self typing do not delay a queued thread', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(1999);
  inbox.typing(typing('other-room'), 'bot');
  inbox.typing(typing('room', 'other-thread'), 'bot');
  inbox.typing(typing('room', ''), 'bot');
  inbox.typing(typing('room', 'thread', 'bot'), 'bot');
  inbox.typing(typing('room', 'thread', ''), 'bot');
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledOnce();
  await inbox.close();
  inbox.typing(typing(), 'bot');
  expect(vi.getTimerCount()).toBe(0);
});

test('room typing delays queued roots but keeps their replies separate', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  const one = { ...message('one'), threadRootId: undefined, reasons: ['direct_message' as const] };
  const two = { ...one, id: 'two' };
  inbox.accept(one);
  inbox.accept(two);
  await vi.advanceTimersByTimeAsync(1000);
  inbox.typing(typing('room', ''), 'bot');
  await vi.advanceTimersByTimeAsync(1999);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledTimes(2);
  expect(turn).toHaveBeenNthCalledWith(1, [one]);
  expect(turn).toHaveBeenNthCalledWith(2, [two]);
  await inbox.close();
});

test('typing postpones the next batch after its timer elapsed during a running turn', async () => {
  const running = Promise.withResolvers<void>();
  const turn = vi.fn(async (items: readonly AddressedMessage[]) => {
    if (items[0]?.id === 'one') await running.promise;
  });
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(2000);
  inbox.typing(typing(), 'bot'); // Does not interrupt or queue work during the current turn.
  inbox.accept(message('two'));
  await vi.advanceTimersByTimeAsync(2000);
  inbox.typing(typing(), 'bot');
  running.resolve();
  await vi.advanceTimersByTimeAsync(1999);
  expect(turn).toHaveBeenCalledOnce();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledTimes(2);
  await inbox.close();
});

test('typing in a new thread delays the queued root message too', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept({ ...message('root'), threadRootId: undefined });
  await vi.advanceTimersByTimeAsync(1000);
  inbox.typing(typing('room', 'root'), 'bot');
  await vi.advanceTimersByTimeAsync(1999);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledOnce();
  await inbox.close();
});

test('waits two seconds after the last message and preserves authors and order', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  const one = message('one');
  const two = { ...message('two'), authorId: 'another-human' };
  inbox.accept(one);
  await vi.advanceTimersByTimeAsync(1000);
  inbox.accept(two);
  await vi.advanceTimersByTimeAsync(1999);
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

test('DM roots and thread replies both wait for the short quiet period', async () => {
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
  await vi.advanceTimersByTimeAsync(1999);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledExactlyOnceWith([root]);
  inbox.accept(reply);
  await vi.advanceTimersByTimeAsync(1999);
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
  await vi.advanceTimersByTimeAsync(1000);
  expect(inbox.accept(message('one'))).toBe('duplicate');
  await vi.advanceTimersByTimeAsync(1000);
  expect(turn).toHaveBeenCalledOnce();
  await inbox.close();
});

test('failed work can be re-enqueued while successful work remains deduplicated', async () => {
  const turn = vi.fn(async () => {}).mockRejectedValueOnce(new Error('Offline'));
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(5000);
  expect(inbox.accept(message('one'))).toBe('accepted');
  await vi.advanceTimersByTimeAsync(5000);
  expect(turn).toHaveBeenCalledTimes(2);
  expect(inbox.accept(message('one'))).toBe('duplicate');
  await inbox.close();
});

test('applies configured debounce and queue limits', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn(), {
    ...DEFAULT_INBOX,
    debounceMs: 2500,
    maxThreads: 1,
    maxPendingMessages: 2
  });
  expect(inbox.accept(message('one'))).toBe('accepted');
  expect(inbox.accept(message('other', 'other-thread'))).toBe('full');
  expect(inbox.accept(message('two'))).toBe('accepted');
  expect(inbox.accept(message('three'))).toBe('full');
  await vi.advanceTimersByTimeAsync(2499);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn).toHaveBeenCalledExactlyOnceWith([message('one'), message('two')]);
  expect(inbox.accept(message('other', 'other-thread'))).toBe('accepted');
  await inbox.close();
});

test('the minimum configured delay still waits for two seconds', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn(), {
    ...DEFAULT_INBOX,
    debounceMs: 2000,
    maxThreads: 1,
    maxPendingMessages: 2
  });
  inbox.accept(message('one'));
  await vi.advanceTimersByTimeAsync(1999);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
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
  await vi.advanceTimersByTimeAsync(1999);
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

test('shutdown discards a queued DM root', async () => {
  const turn = vi.fn(async () => {});
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept({ ...message('root'), threadRootId: undefined, reasons: ['direct_message'] });
  // A DM root now waits too; shutdown must cancel its timer.
  await inbox.close();
  await vi.advanceTimersByTimeAsync(2000);
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

test('passive thread messages wait for five seconds of quiet', async () => {
  const turn = vi.fn(async (_messages: readonly AddressedMessage[]) => {});
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept({ ...message('one'), reasons: [] });
  await vi.advanceTimersByTimeAsync(4000);
  inbox.accept({ ...message('two'), reasons: [] });
  await vi.advanceTimersByTimeAsync(4999);
  expect(turn).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(turn.mock.calls[0]?.[0].map((item) => item.id)).toEqual(['one', 'two']);
  await inbox.close();
});

test.each(['mention', 'reply', 'direct_message'] as const)(
  '%s promotes the pending batch and later passive text keeps the short debounce',
  async (reason) => {
    const turn = vi.fn(async (_messages: readonly AddressedMessage[]) => {});
    const inbox = new Inbox(turn, vi.fn());
    inbox.accept({ ...message('ordinary'), reasons: [] });
    await vi.advanceTimersByTimeAsync(1000);
    inbox.accept({ ...message('addressed'), reasons: [reason] });
    await vi.advanceTimersByTimeAsync(1000);
    inbox.accept({ ...message('clarification'), reasons: [] });
    await vi.advanceTimersByTimeAsync(1999);
    expect(turn).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(turn.mock.calls[0]?.[0].map((item) => item.id)).toEqual([
      'ordinary',
      'addressed',
      'clarification'
    ]);
    inbox.accept({ ...message('next'), reasons: [] });
    await vi.advanceTimersByTimeAsync(2000);
    expect(turn).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(3000);
    expect(turn).toHaveBeenCalledTimes(2);
    await inbox.close();
  }
);

test('active work is scoped by room and thread and uses the configured passive delay', async () => {
  const running = Promise.withResolvers<void>();
  const turn = vi.fn(() => running.promise);
  const inbox = new Inbox(turn, vi.fn(), { ...DEFAULT_INBOX, passiveDebounceMs: 2500 });
  inbox.accept({ ...message('one'), reasons: [] });
  expect(inbox.isActive({ roomId: 'room', threadRootId: 'thread' })).toBe(true);
  expect(inbox.isActive({ roomId: 'other', threadRootId: 'thread' })).toBe(false);
  expect(inbox.isActive({ roomId: 'room', threadRootId: 'other' })).toBe(false);
  await vi.advanceTimersByTimeAsync(2500);
  expect(turn).toHaveBeenCalledOnce();
  expect(inbox.isActive({ roomId: 'room', threadRootId: 'thread' })).toBe(true);
  running.resolve();
  await vi.advanceTimersByTimeAsync(0);
  expect(inbox.isActive({ roomId: 'room', threadRootId: 'thread' })).toBe(false);
  await inbox.close();
});

test('promotes a passive batch during a running turn without overlapping turns', async () => {
  const running = Promise.withResolvers<void>();
  const turn = vi.fn(async (items: readonly AddressedMessage[]) => {
    if (items[0]?.id === 'first') await running.promise;
  });
  const inbox = new Inbox(turn, vi.fn());
  inbox.accept(message('first'));
  await vi.advanceTimersByTimeAsync(5000);
  inbox.accept({ ...message('ordinary'), reasons: [] });
  inbox.accept(message('ping'));
  await vi.advanceTimersByTimeAsync(5000);
  expect(turn).toHaveBeenCalledOnce();
  running.resolve();
  await vi.advanceTimersByTimeAsync(0);
  expect(turn.mock.calls[1]?.[0].map((item) => item.id)).toEqual(['ordinary', 'ping']);
  await inbox.close();
});
