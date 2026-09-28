import { expect, test, vi } from 'vitest';
import { createChattoClient } from '@chatto/client';
import { createThreadReader } from './thread.ts';
import type { Delivery } from './chatto/routing.ts';

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
// These tests cover thread reading; the name lookup finds no users.
const withoutNames =
  (request: typeof fetch): typeof fetch =>
  (url, init) =>
    String(url).endsWith('UserService/BatchGetUsers')
      ? Promise.resolve(Response.json({ users: [] }))
      : request(url, init);
const reader = (request: typeof fetch) =>
  createThreadReader(
    createChattoClient({
      serverUrl: 'https://chat.example',
      apiKey: 'key',
      fetch: withoutNames(request)
    }),
    'bot'
  );
const event = (id: string, body: string, actorId = 'alice') => ({
  id,
  messagePosted: { message: { actorId, body } }
});

test('loads all pages in order with one root and no overlapping messages', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(
      Response.json({
        page: {
          events: [event('root', 'Hello'), event('three', 'drei'), event('ping', "What's next?")],
          hasOlder: true,
          startCursor: 'older'
        }
      })
    )
    .mockResolvedValueOnce(
      Response.json({
        page: {
          events: [event('one', 'eins'), event('two', 'zwei', 'bot'), event('three', 'drei')]
        }
      })
    );
  const messages = await reader(request)(delivery, new AbortController().signal);
  expect(messages.map((message) => message.id)).toEqual(['root', 'one', 'two', 'three', 'ping']);
  expect(messages[2]?.role).toBe('bot');
  expect(JSON.parse(request.mock.calls[1]![1]!.body as string)).toEqual({
    roomId: 'room',
    threadRootEventId: 'root',
    limit: 100,
    before: 'older'
  });
  expect(request.mock.calls[0]![1]?.redirect).toBe('error');
});

test('fails when pagination repeats instead of returning incomplete history', async () => {
  const request = vi.fn<typeof fetch>().mockImplementation(async () =>
    Response.json({
      page: { events: [], hasOlder: true, startCursor: 'same' }
    })
  );
  await expect(reader(request)(delivery, new AbortController().signal)).rejects.toThrow(
    'pagination did not advance'
  );
});

test('reports permission failures before composing a reply', async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 403 }));
  await expect(reader(request)(delivery, new AbortController().signal)).rejects.toThrow('403');
});

test.each([null, 'existing-root'])('reads DM thread context for root %s', async (threadRoot) => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(
    Response.json({
      page: { events: [event('one', 'Hello'), event('two', 'Hi', 'bot')] }
    })
  );
  const messages = await reader(request)(
    { ...delivery, triggers: ['direct_message'], thread_root_id: threadRoot },
    new AbortController().signal
  );
  expect(String(request.mock.calls[0]![0])).toContain('ThreadService/GetThreadEvents');
  expect(JSON.parse(request.mock.calls[0]![1]!.body as string)).toEqual({
    roomId: 'room',
    threadRootEventId: threadRoot ?? 'ping',
    limit: 100
  });
  expect(messages.map((message) => message.id)).toEqual(['one', 'two']);
});

test('messages carry author names, cached for ten minutes, and survive a failed lookup', async () => {
  let clock = 0;
  const lookups: string[][] = [];
  let failNames = false;
  const request = vi.fn<typeof fetch>().mockImplementation(async (url, init) => {
    if (String(url).endsWith('UserService/BatchGetUsers')) {
      if (failNames) return new Response('unavailable', { status: 503 });
      const { userIds } = JSON.parse(init!.body as string) as { userIds: string[] };
      lookups.push(userIds);
      return Response.json({
        users: userIds.map((id) => ({
          user: { id, login: id, ...(id === 'alice' ? { displayName: 'Alice Doe' } : {}) }
        }))
      });
    }
    return Response.json({
      page: {
        events: [event('root', 'Hello'), event('ping', 'Hi', 'bot'), event('x', 'Yo', 'bob')]
      }
    });
  });
  const read = createThreadReader(
    createChattoClient({ serverUrl: 'https://chat.example', apiKey: 'key', fetch: request }),
    'bot',
    () => clock
  );
  const signal = new AbortController().signal;
  const messages = await read(delivery, signal);
  expect(messages.map(({ authorName, authorLogin }) => [authorName, authorLogin])).toEqual([
    ['Alice Doe', 'alice'],
    ['bot', 'bot'],
    ['bob', 'bob']
  ]);
  await read(delivery, signal);
  expect(lookups).toEqual([['alice', 'bot', 'bob']]);
  clock = 11 * 60_000;
  failNames = true;
  const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
  // Expired names are looked up again; a failed lookup keeps the thread and the old names.
  expect((await read(delivery, signal)).map((message) => message.body)).toEqual([
    'Hello',
    'Hi',
    'Yo'
  ]);
  expect(warning).toHaveBeenCalledWith('ChattoBot could not read author names for a thread.');
  warning.mockRestore();
});
