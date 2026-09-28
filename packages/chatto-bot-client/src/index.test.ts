import { expect, test, vi } from 'vitest';
import { RealtimeEvent, RoomKind } from '@chatto/client';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type { CreateMessageRequest } from '@chatto/api-types/api/v1/messages_pb';
import type { GetThreadEventsRequest } from '@chatto/api-types/api/v1/room_timeline_pb';
import {
  conversationKey,
  createBotClient,
  createDeliveryTracker,
  replyDestination
} from './index.js';
import { fakeConnection } from './testing/fakeConnection.js';

function dmEvent(id = 'incoming', body = 'hello') {
  return new RealtimeEvent({
    id,
    actorId: 'human',
    event: {
      case: 'messagePosted',
      value: {
        roomId: 'room',
        roomKind: RoomKind.DM,
        bodyPlaintext: body,
        threadRootEventId: 'root'
      }
    }
  });
}

test('resolves identity once and replies to the prompting message', async () => {
  const created: CreateMessageRequest[] = [];
  const fake = fakeConnection((router) =>
    router.service(MessageService, {
      createMessage(request) {
        created.push(request);
        return { message: { id: 'reply' } };
      }
    })
  );
  const bot = await createBotClient(fake.chatto);
  expect(bot.chatto).toBe(fake.chatto);
  expect(bot.viewerId).toBe('bot');
  const message = (await bot.addressedMessage(dmEvent()))!;
  expect(message.reasons).toEqual(['direct_message']);
  expect(bot.conversationKey(message)).toBe(JSON.stringify(['bot', 'room', 'root', 'human']));
  await bot.reply(message, 'hi');
  expect(created).toHaveLength(1);
  expect(created[0]).toMatchObject({
    roomId: 'room',
    threadRootEventId: 'root',
    inReplyTo: 'incoming',
    body: 'hi'
  });
  await expect(bot.reply(message, 'no', AbortSignal.abort(new Error('stop')))).rejects.toThrow();
  expect(created).toHaveLength(1);
});

test('identity failures and cancellation do not create a bot client', async () => {
  const rejected = fakeConnection(undefined, Promise.reject(new Error('rejected API key')));
  await expect(createBotClient(rejected.chatto)).rejects.toThrow('rejected API key');
  const signal = AbortSignal.abort(new Error('cancelled'));
  await expect(createBotClient(fakeConnection().chatto, { signal })).rejects.toThrow('cancelled');
});

test('splits long messages at 8000 code points and sends them in order', async () => {
  const bodies: string[] = [];
  const fake = fakeConnection((router) =>
    router.service(MessageService, {
      createMessage(request) {
        bodies.push(request.body);
        return {};
      }
    })
  );
  const bot = await createBotClient(fake.chatto);
  await bot.postMessage({ roomId: 'room', threadRootId: 'root' }, '😀'.repeat(8001));
  expect(bodies.map((body) => Array.from(body).length)).toEqual([8000, 1]);
  await bot.postMessage({ roomId: 'room', threadRootId: 'root' }, '');
  expect(bodies).toHaveLength(3);
});

test('reads all thread pages root first, with bot roles kept separate', async () => {
  const requests: GetThreadEventsRequest[] = [];
  const posted = (id: string, actorId: string, body: string) => ({
    id,
    actorId,
    event: { case: 'messagePosted' as const, value: { message: { id, actorId, body } } }
  });
  const fake = fakeConnection((router) =>
    router.service(ThreadService, {
      getThreadEvents(request) {
        requests.push(request);
        return request.cursor.case === 'before'
          ? { page: { events: [posted('root', 'human', 'question')], hasOlder: false } }
          : {
              page: {
                events: [posted('one', 'bot', 'answer'), posted('two', 'human', 'thanks')],
                hasOlder: true,
                startCursor: 'older'
              }
            };
      }
    })
  );
  const bot = await createBotClient(fake.chatto);
  const messages = await bot.readThread({ roomId: 'room', threadRootId: 'root' });
  expect(messages.map((message) => message.id)).toEqual(['root', 'one', 'two']);
  expect(requests.map((request) => request.cursor.value)).toEqual([undefined, 'older']);
  const roles = await bot.readBotThread({ roomId: 'room', threadRootId: 'root' });
  expect(roles.map((message) => message.role)).toEqual(['human', 'bot', 'human']);
  expect(messages[1]).not.toHaveProperty('role');
});

test('rejects thread pagination that does not advance', async () => {
  const fake = fakeConnection((router) =>
    router.service(ThreadService, {
      getThreadEvents: () => ({ page: { events: [], hasOlder: true, startCursor: 'same' } })
    })
  );
  const bot = await createBotClient(fake.chatto);
  await expect(bot.readThread({ roomId: 'room', threadRootId: 'root' })).rejects.toThrow(
    'did not advance'
  );
});

test('consumes events in order, reports status and gaps, and stops on abort', async () => {
  const fake = fakeConnection();
  const bot = await createBotClient(fake.chatto);
  const controller = new AbortController();
  const handled: string[] = [];
  const statuses: unknown[] = [];
  let release!: () => void;
  const consuming = bot.consumeEvents({
    signal: controller.signal,
    onStatus: (status) => statuses.push(status),
    async onEvent(event) {
      handled.push(event.id);
      if (event.id === 'first') await new Promise<void>((resolve) => (release = resolve));
    }
  });
  fake.reset();
  fake.setStatus('connected');
  fake.emit(dmEvent('first'));
  fake.emit(dmEvent('second'));
  await vi.waitFor(() => expect(handled).toEqual(['first']));
  release();
  await vi.waitFor(() => expect(handled).toEqual(['first', 'second']));
  // A resync publishes a reset and then a snapshot: one gap.
  fake.reset();
  fake.setStatus('connecting');
  fake.reset();
  fake.setStatus('connected');
  expect(statuses).toEqual([
    { state: 'connecting' },
    { state: 'ready', gap: false },
    { state: 'ready', gap: true },
    { state: 'reconnecting' },
    { state: 'ready', gap: false }
  ]);
  controller.abort();
  await consuming;
  expect(fake.listenerCount).toBe(0);
});

test('stops when the server ends the session or the connection closes', async () => {
  for (const end of ['session', 'close'] as const) {
    const fake = fakeConnection();
    const bot = await createBotClient(fake.chatto);
    const consuming = bot.consumeEvents({
      signal: new AbortController().signal,
      onEvent: () => {}
    });
    if (end === 'session') fake.endSession();
    else fake.chatto.close();
    await expect(consuming).rejects.toThrow(end === 'session' ? 'ended the session' : 'closed');
    expect(fake.listenerCount).toBe(0);
  }
});

test('stops instead of buffering without bound behind a slow handler', async () => {
  const fake = fakeConnection();
  const bot = await createBotClient(fake.chatto);
  const consuming = bot.consumeEvents({
    signal: new AbortController().signal,
    onEvent: () => new Promise(() => {})
  });
  for (let index = 0; index < 1002; index++) fake.emit(dmEvent(`event-${index}`));
  await expect(consuming).rejects.toThrow('faster than the bot handled them');
});

test('a failing status callback stops consumption without breaking the connection', async () => {
  const fake = fakeConnection();
  const bot = await createBotClient(fake.chatto);
  const consuming = bot.consumeEvents({
    signal: new AbortController().signal,
    onStatus: () => {
      throw new Error('status handler failed');
    },
    onEvent: () => {}
  });
  expect(() => fake.setStatus('connected')).not.toThrow();
  await expect(consuming).rejects.toThrow('status handler failed');
});

test('a failed event handler stops consumption with its error', async () => {
  const fake = fakeConnection();
  const bot = await createBotClient(fake.chatto);
  const consuming = bot.consumeEvents({
    signal: new AbortController().signal,
    onEvent: () => Promise.reject(new Error('dispatch failed'))
  });
  fake.emit(dmEvent());
  await expect(consuming).rejects.toThrow('dispatch failed');
  expect(fake.listenerCount).toBe(0);
});

test('default keys isolate bot, room, thread, and sender; root messages reply in their own thread', () => {
  const message = { id: 'root', roomId: 'room', authorId: 'human' };
  const key = conversationKey('bot', message);
  expect(replyDestination(message)).toEqual({
    roomId: 'room',
    threadRootId: 'root',
    inReplyTo: 'root'
  });
  expect(conversationKey('bot', { ...message, threadRootId: 'root' })).toBe(key);
  expect(conversationKey('other', message)).not.toBe(key);
  for (const override of [{ roomId: 'other' }, { threadRootId: 'other' }, { authorId: 'other' }]) {
    expect(conversationKey('bot', { ...message, ...override })).not.toBe(key);
  }
});

test('deliveries remain retryable until explicitly accepted, then expire', async () => {
  let now = 0;
  const tracker = createDeliveryTracker({ retentionMs: 10, now: () => now });
  const accept = async (register: () => Promise<void>) => {
    if (tracker.has('message')) return;
    await register();
    tracker.accept('message');
  };
  await expect(
    accept(async () => {
      throw new Error('disk');
    })
  ).rejects.toThrow('disk');
  expect(tracker.has('message')).toBe(false);
  const register = vi.fn().mockResolvedValue(undefined);
  await accept(register);
  await accept(register);
  expect(register).toHaveBeenCalledOnce();
  expect(createDeliveryTracker().has('message')).toBe(false);
  now = 10;
  await accept(register);
  expect(register).toHaveBeenCalledTimes(2);
});

test.each([0, -1, Infinity, NaN])('rejects invalid delivery retention: %s', (retentionMs) => {
  expect(() => createDeliveryTracker({ retentionMs })).toThrow('positive and finite');
});
