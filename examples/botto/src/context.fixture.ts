/** Real client timeline with an in-memory history API for Botto integration tests. */
import { vi } from 'vitest';
import { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { messagePostedPayload, type EventConnectionPage } from '@chatto/client/api/roomTimeline';
import type { ServerConnection } from '@chatto/client/server/serverConnection';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';

export function row(id: string, body: string, actorId = 'human'): TimelineEventView {
  return {
    id,
    actorId,
    createdAt: '2026-10-23T00:00:00Z',
    event: messagePostedPayload(
      new Message({
        roomId: 'room',
        threadRootEventId: id === 'root' ? '' : 'root',
        body
      }),
      {}
    )
  };
}

export function contextFixture(initial?: Promise<EventConnectionPage>) {
  const history = vi.fn<() => Promise<EventConnectionPage>>().mockImplementation(
    () =>
      initial ??
      Promise.resolve({
        events: [
          row('root', 'Root'),
          row('ordinary', 'Useful unaddressed context'),
          row('one', 'First'),
          row('two', 'Clarification', 'another')
        ],
        hasOlder: false,
        hasNewer: false,
        startCursor: 'start',
        endCursor: 'end'
      })
  );
  // MessagesStore only uses getAPI from its connection; reads stay entirely in memory.
  const connection = {
    getAPI: () => ({ getThreadEvents: history })
  } as unknown as ServerConnection;
  const store = new MessagesStore(connection, () => 'bot', {
    roomId: 'room',
    threadRootEventId: 'root'
  });
  const server = {
    rooms: { thread: vi.fn(() => store) },
    accountId: 'bot',
    status: 'connected' as const
  };
  return { history, store, server };
}
