/** Real client thread timelines with an in-memory history API for Botto tests. */
import { vi } from 'vitest';
import { RoomStores } from '@chatto/client/server/roomStores';
import { signal } from '@chatto/client/reactivity';
import type { ServerStatus } from '@chatto/client/server/server';
import { messagePostedPayload, type EventConnectionPage } from '@chatto/client/api/roomTimeline';
import type { ServerConnection } from '@chatto/client/server/serverConnection';
import type { ServerPresence } from '@chatto/client/server/presence';
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

export function page(events: TimelineEventView[], hasOlder = false): EventConnectionPage {
  return { events, hasOlder, hasNewer: false, startCursor: 'start', endCursor: 'end' };
}

/** `history` answers every thread read with `initial` until a test changes it. */
export function contextFixture(initial?: Promise<EventConnectionPage>) {
  const history = vi
    .fn<() => Promise<EventConnectionPage>>()
    .mockImplementation(
      () =>
        initial ??
        Promise.resolve(
          page([
            row('root', 'Root'),
            row('ordinary', 'Useful unaddressed context'),
            row('one', 'First'),
            row('two', 'Clarification', 'another')
          ])
        )
    );
  // Thread timelines only use getAPI from the connection; reads stay in memory.
  const connection = {
    getAPI: () => ({ getThreadEvents: history })
  } as unknown as ServerConnection;
  const rooms = new RoomStores({
    serverId: 'server',
    connection,
    presence: {} as ServerPresence,
    realtimeViewerId: () => 'bot',
    viewerId: () => 'bot',
    projectedMemberIds: () => null
  });
  // Reactive like the client's status, so that a read can wait for a reconnect.
  const status = signal<ServerStatus>('connected');
  const server = {
    rooms,
    accountId: 'bot',
    get status() {
      return status.get();
    },
    set status(value: ServerStatus) {
      status.set(value);
    }
  };
  const store = () => rooms.thread('room', 'root');
  return { history, rooms, store, server };
}
