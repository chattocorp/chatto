import { expect, test, vi } from 'vitest';
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
  const messages = await createThreadReader(
    'https://chat.example',
    'key',
    request
  )(delivery, new AbortController().signal);
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
  await expect(
    createThreadReader(
      'https://chat.example',
      'key',
      request
    )(delivery, new AbortController().signal)
  ).rejects.toThrow('pagination did not advance');
});

test('reports permission failures before composing a reply', async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 403 }));
  await expect(
    createThreadReader(
      'https://chat.example',
      'key',
      request
    )(delivery, new AbortController().signal)
  ).rejects.toThrow('403');
});

test.each([null, 'existing-root'])('reads DM thread context for root %s', async (threadRoot) => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(
    Response.json({
      page: { events: [event('one', 'Hello'), event('two', 'Hi', 'bot')] }
    })
  );
  const messages = await createThreadReader(
    'https://chat.example',
    'key',
    request
  )(
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
