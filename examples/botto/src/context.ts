/** Select Pi's text context from the client's shared, realtime thread timeline. */
import type { Server, ThreadLocation } from '@chatto/client';
import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import { untilAborted } from '@chatto/client/util/untilAborted';
import type { ThreadContext } from './context-messages.ts';

/** Only the shared timeline accessor and connection identity are needed. */
export type ContextSource = Pick<Server, 'accountId' | 'status'> & {
  rooms: Pick<Server['rooms'], 'thread'>;
};

/** Botto retries a failed initial load once and backfills its context window.
 * The client owns shared history, realtime recovery, and access cleanup.
 * The returned context contains the root and up to limit recent text replies. */
export async function readContext(
  server: ContextSource,
  location: ThreadLocation,
  limit: number,
  signal: AbortSignal
): Promise<ThreadContext> {
  signal.throwIfAborted();
  if (server.status !== 'connected') throw new Error('Chatto is not connected');
  const store = server.rooms.thread(location.roomId, location.threadRootId);
  if (!(await store.waitForLoad({ signal }))) {
    await untilAborted(store.restoreLatestWindow(), signal);
    if (!(await store.waitForLoad({ signal }))) throw new Error('Thread history is not ready');
  }
  while (!store.hasReachedStart && store.events.length < limit + 1) {
    signal.throwIfAborted();
    const count = store.events.length;
    await untilAborted(store.loadMore(), signal);
    if (
      !(await store.waitForLoad({ signal })) ||
      (!store.hasReachedStart && store.events.length <= count)
    )
      throw new Error('Thread history is not ready');
  }
  if (server.status !== 'connected') throw new Error('Thread history is not ready');
  const messages = store.threadEvents.flatMap((event) => {
    const payload = event.event;
    if (payload.kind !== TimelineEventKind.MessagePosted || payload.deletedAt || !payload.body)
      return [];
    return [
      {
        id: event.id,
        author: event.actorId ?? undefined,
        text: payload.body,
        fromViewer: event.actorId === server.accountId
      }
    ];
  });
  const root = messages.find((message) => message.id === location.threadRootId);
  const replies = messages.filter((message) => message !== root);
  return {
    messages: [...(root ? [root] : []), ...replies.slice(-limit)],
    olderContextOmitted: !store.hasReachedStart || replies.length > limit
  };
}
