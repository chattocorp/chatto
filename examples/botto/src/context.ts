/** Select Pi's text context from the client's shared, realtime thread timeline. */
import type { Server, ThreadLocation } from '@chatto/client';
import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { computed, subscribe } from '@chatto/client/reactivity';
import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import { untilAborted } from './abort.ts';
import type { ThreadContext } from './context-messages.ts';

/** The thread timelines and connection identity that context selection needs. */
export type ContextSource = Pick<Server, 'accountId' | 'status'> & {
  rooms: Pick<Server['rooms'], 'thread' | 'retainThread' | 'releaseThread'>;
};

/** The client loads a thread once and keeps it current through realtime delivery.
 * Botto retries a failed load once. The returned context contains the root and up
 * to limit recent text replies; limit must fit in one thread page (50 replies). */
export async function readContext(
  server: ContextSource,
  location: ThreadLocation,
  limit: number,
  signal: AbortSignal
): Promise<ThreadContext> {
  signal.throwIfAborted();
  if (server.status !== 'connected') throw new Error('Chatto is not connected');
  const { roomId, threadRootId } = location;
  const store = server.rooms.thread(roomId, threadRootId);
  // Retain the timeline while reading, so that the client keeps it.
  server.rooms.retainThread(roomId, threadRootId, store);
  try {
    if (!(await loadSettled(store, signal))) {
      await untilAborted(store.restoreLatestWindow(), signal);
      if (!(await loadSettled(store, signal))) throw new Error('Thread history is not ready');
    }
    return selectContext(store, server.accountId, location, limit);
  } finally {
    server.rooms.releaseThread(roomId, threadRootId, store);
  }
}

/** Resolve true when the timeline is current, or false when its read fails. */
function loadSettled(store: MessagesStore, signal: AbortSignal): Promise<boolean> {
  const state = computed(() => (store.isCurrent ? true : store.loadError ? false : undefined));
  const current = state.get();
  if (current !== undefined) return Promise.resolve(current);
  return untilAborted(
    new Promise<boolean>((resolve) => {
      const stop = subscribe(state, () => {
        const settled = state.get();
        if (settled === undefined) return;
        stop();
        resolve(settled);
      });
      signal.addEventListener('abort', stop, { once: true });
    }),
    signal
  );
}

function selectContext(
  store: MessagesStore,
  viewerId: string | null,
  location: ThreadLocation,
  limit: number
): ThreadContext {
  const messages = store.threadEvents.flatMap((event) => {
    const payload = event.event;
    if (payload.kind !== TimelineEventKind.MessagePosted || payload.deletedAt || !payload.body)
      return [];
    return [
      {
        id: event.id,
        author: event.actorId ?? undefined,
        text: payload.body,
        fromViewer: event.actorId === viewerId
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
