import { expect, test } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type {
  GetThreadEventsRequest,
  RoomTimelinePage
} from '@chatto/api-types/api/v1/room_timeline_pb';
import type { PartialMessage } from '@bufbuild/protobuf';
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

/** A thread reader backed by an in-memory server that answers with `pages`. */
async function reader(
  pages: (request: GetThreadEventsRequest) => PartialMessage<RoomTimelinePage>
) {
  const requests: GetThreadEventsRequest[] = [];
  const { createApi } = fakeChatto({
    viewerId: 'bot',
    routes: (router) =>
      router.service(ThreadService, {
        getThreadEvents(request) {
          requests.push(request);
          return { page: pages(request) };
        }
      })
  });
  const api = createApi({ serverUrl: 'https://chat.example', apiKey: 'key' });
  return { read: createThreadReader(api), requests };
}

const event = (id: string, body: string, actorId = 'alice') => ({
  id,
  actorId,
  event: { case: 'messagePosted' as const, value: { message: { id, actorId, body } } }
});

test('reads the newest replies with roles and names, then only messages after the cursor', async () => {
  const { read, requests } = await reader((request) =>
    request.cursor.case === 'after'
      ? { events: [event('three', 'drei')], hasNewer: false, endCursor: 'c2' }
      : {
          events: [
            event('root', 'Hello'),
            event('two', 'zwei', 'bot'),
            event('ping', "What's next?")
          ],
          hasOlder: true,
          endCursor: 'c1',
          includes: { users: { alice: { login: 'alice', displayName: 'Alice Doe' } } }
        }
  );
  const first = await read(delivery, new AbortController().signal);
  expect(first).toMatchObject({ cursor: 'c1', olderOmitted: true });
  expect(first.messages.map(({ id, role, authorName }) => [id, role, authorName])).toEqual([
    ['root', 'human', 'Alice Doe'],
    ['two', 'bot', undefined],
    ['ping', 'human', 'Alice Doe']
  ]);
  const next = await read(delivery, new AbortController().signal, first.cursor);
  expect(next.messages.map((message) => message.id)).toEqual(['three']);
  expect(requests[1]).toMatchObject({
    roomId: 'room',
    threadRootEventId: 'root',
    limit: 100,
    cursor: { case: 'after', value: 'c1' }
  });
});

test('fails when pagination repeats instead of returning incomplete history', async () => {
  const { read } = await reader(() => ({ events: [], hasNewer: true, endCursor: 'same' }));
  await expect(read(delivery, new AbortController().signal, 'same')).rejects.toThrow(
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
  const { messages } = await read(
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
