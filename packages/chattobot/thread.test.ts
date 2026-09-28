import { expect, test } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { createBotClient } from '@chatto/bot-client';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type { GetThreadEventsRequest } from '@chatto/api-types/api/v1/room_timeline_pb';
import { createThreadReader } from './thread.ts';
import type { Delivery } from './chatto/routing.ts';
import { fakeChatto } from './chatto/fake-chatto.ts';

const delivery: Delivery = {
  version: 1,
  id: 'ping',
  type: 'message.created',
  triggers: ['mention'],
  occurred_at: 'now',
  bot_id: 'bot',
  room_id: 'room',
  thread_root_id: 'root',
  message: { id: 'ping', author_id: 'alice', body: "What's next?" }
};
type Page = { events: ReturnType<typeof event>[]; hasOlder?: boolean; startCursor?: string };
async function reader(pages: (request: GetThreadEventsRequest) => Page) {
  const requests: GetThreadEventsRequest[] = [];
  const { connectChatto } = fakeChatto({
    viewerId: 'bot',
    routes: (router) =>
      router.service(ThreadService, {
        getThreadEvents(request) {
          requests.push(request);
          return { page: pages(request) };
        }
      })
  });
  const bot = await createBotClient(
    connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' })
  );
  return { read: createThreadReader(bot, 'bot'), requests };
}
const event = (id: string, body: string, actorId = 'alice') => ({
  id,
  actorId,
  event: { case: 'messagePosted' as const, value: { message: { id, actorId, body } } }
});

test('loads all pages in order with one root and no overlapping messages', async () => {
  const { read, requests } = await reader((request) =>
    request.cursor.case === 'before'
      ? { events: [event('one', 'eins'), event('two', 'zwei', 'bot'), event('three', 'drei')] }
      : {
          events: [event('root', 'Hello'), event('three', 'drei'), event('ping', "What's next?")],
          hasOlder: true,
          startCursor: 'older'
        }
  );
  const messages = await read(delivery, new AbortController().signal);
  expect(messages.map((message) => message.id)).toEqual(['root', 'one', 'two', 'three', 'ping']);
  expect(messages[2]?.role).toBe('bot');
  expect(requests[1]).toMatchObject({
    roomId: 'room',
    threadRootEventId: 'root',
    limit: 100,
    cursor: { case: 'before', value: 'older' }
  });
});

test('fails when pagination repeats instead of returning incomplete history', async () => {
  const { read } = await reader(() => ({ events: [], hasOlder: true, startCursor: 'same' }));
  await expect(read(delivery, new AbortController().signal)).rejects.toThrow(
    'pagination did not advance'
  );
});

test('reports permission failures before composing a reply', async () => {
  const { read } = await reader(() => {
    throw new ConnectError('denied', Code.PermissionDenied);
  });
  await expect(read(delivery, new AbortController().signal)).rejects.toMatchObject({
    code: Code.PermissionDenied
  });
});

test.each([null, 'existing-root'])('reads DM thread context for root %s', async (threadRoot) => {
  const { read, requests } = await reader(() => ({
    events: [event('one', 'Hello'), event('two', 'Hi', 'bot')]
  }));
  const messages = await read(
    { ...delivery, triggers: ['direct_message'], thread_root_id: threadRoot },
    new AbortController().signal
  );
  expect(requests[0]).toMatchObject({
    roomId: 'room',
    threadRootEventId: threadRoot ?? 'ping',
    limit: 100,
    cursor: { case: undefined }
  });
  expect(messages.map((message) => message.id)).toEqual(['one', 'two']);
});
