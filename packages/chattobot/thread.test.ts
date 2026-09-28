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
const reader = (request: typeof fetch) =>
  createThreadReader(
    createChattoClient({ serverUrl: 'https://chat.example', apiKey: 'key', fetch: request }),
    'bot'
  );
const event = (id: string, body: string, actorId = 'alice') => ({
  id,
  messagePosted: { message: { actorId, body } }
});

test('reads the newest replies with roles and names, then only messages after the cursor', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(
      Response.json({
        page: {
          events: [
            event('root', 'Hello'),
            event('two', 'zwei', 'bot'),
            event('ping', "What's next?")
          ],
          hasOlder: true,
          endCursor: 'c1',
          includes: { users: { alice: { login: 'alice', displayName: 'Alice Doe' } } }
        }
      })
    )
    .mockResolvedValueOnce(
      Response.json({
        page: { events: [event('three', 'drei')], hasNewer: false, endCursor: 'c2' }
      })
    );
  const read = reader(request);
  const first = await read(delivery, new AbortController().signal);
  expect(first).toMatchObject({ cursor: 'c1', olderOmitted: true });
  expect(first.messages.map(({ id, role, authorName }) => [id, role, authorName])).toEqual([
    ['root', 'human', 'Alice Doe'],
    ['two', 'bot', undefined],
    ['ping', 'human', 'Alice Doe']
  ]);
  const next = await read(delivery, new AbortController().signal, first.cursor);
  expect(next.messages.map((message) => message.id)).toEqual(['three']);
  expect(JSON.parse(request.mock.calls[1]![1]!.body as string)).toEqual({
    roomId: 'room',
    threadRootEventId: 'root',
    limit: 100,
    after: 'c1'
  });
  expect(request.mock.calls[0]![1]?.redirect).toBe('error');
});

test('fails when pagination repeats instead of returning incomplete history', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockImplementation(async () =>
      Response.json({ page: { events: [], hasNewer: true, endCursor: 'same' } })
    );
  await expect(reader(request)(delivery, new AbortController().signal, 'same')).rejects.toThrow(
    'pagination did not advance'
  );
});

test('reports permission failures before composing a reply', async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 403 }));
  await expect(reader(request)(delivery, new AbortController().signal)).rejects.toThrow('403');
});

test.each([null, 'existing-root'])('reads DM thread context for root %s', async (threadRoot) => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValue(
      Response.json({ page: { events: [event('one', 'Hello'), event('two', 'Hi', 'bot')] } })
    );
  const { messages } = await reader(request)(
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
