/** Select Pi's text context from the client's shared, realtime thread timeline. */
import type { Server, ThreadLocation } from '@chatto/client';
import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { computed, subscribe } from '@chatto/client/reactivity';
import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import { untilAborted } from './abort.ts';
import type { ThreadContext } from './context-messages.ts';

/** The thread timelines and connection identity that context selection needs. */
export type ContextSource = Pick<Server, 'accountId' | 'status'> & {
  rooms: Pick<Server['rooms'], 'thread'>;
};

/** The client loads a thread once and keeps it current through realtime delivery.
 * Botto reads again once when a load fails, or gets the timeline again when the
 * client disposed it during the wait. The returned context contains the root and
 * up to limit recent text replies; limit must fit in one thread page (50 replies). */
export async function readContext(
  server: ContextSource,
  location: ThreadLocation,
  limit: number,
  signal: AbortSignal
): Promise<ThreadContext> {
  signal.throwIfAborted();
  if (server.status !== 'connected') throw new Error('Chatto is not connected');
  const { roomId, threadRootId } = location;
  let store = server.rooms.thread(roomId, threadRootId);
  let state = await loadSettled(store, signal);
  if (state === 'disposed') store = server.rooms.thread(roomId, threadRootId);
  else if (state === 'failed') await untilAborted(store.restoreLatestWindow(), signal);
  if (state !== 'current') state = await loadSettled(store, signal);
  if (state !== 'current') throw new Error('Thread history is not ready');
  return selectContext(store, server.accountId, location, limit);
}

type LoadState = 'current' | 'failed' | 'disposed';

/** Resolve when the timeline is current, its read failed, or the client disposed it. */
function loadSettled(store: MessagesStore, signal: AbortSignal): Promise<LoadState> {
  const state = computed((): LoadState | undefined => {
    if (store.isCurrent) return 'current';
    if (store.isDisposed) return 'disposed';
    if (store.loadError) return 'failed';
    return undefined;
  });
  const current = state.get();
  if (current !== undefined) return Promise.resolve(current);
  return untilAborted(
    new Promise<LoadState>((resolve) => {
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
        fromViewer: viewerId !== null && event.actorId === viewerId
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
