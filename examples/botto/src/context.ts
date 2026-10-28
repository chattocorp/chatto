/** Select Pi's message context from the client's shared, realtime thread timeline. */
import type { Server, ThreadLocation } from '@chatto/client';
import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { computed, subscribe } from '@chatto/client/reactivity';
import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import type { Client } from '@connectrpc/connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { untilAborted } from './abort.ts';
import type { ThreadContext } from './context-messages.ts';

/** The thread timelines and connection identity that context selection needs. */
export type ContextSource = Pick<Server, 'accountId' | 'status'> & {
  rooms: Pick<Server['rooms'], 'thread'>;
};

/** The client loads a thread once and keeps it current through realtime delivery.
 * Botto waits while Chatto reconnects. It makes one more attempt: it reads again
 * when a load fails, or gets the timeline again when the client disposed it
 * during the wait. The returned context contains the root and up to limit recent
 * replies with text or attachments; limit must fit in one thread page (50 replies). */
export async function readContext(
  server: ContextSource,
  location: ThreadLocation,
  limit: number,
  signal: AbortSignal
): Promise<ThreadContext> {
  signal.throwIfAborted();
  // A reconnect reloads the timelines before the status becomes connected.
  await settled(() => (server.status === 'connected' ? true : undefined), signal);
  const { roomId, threadRootId } = location;
  let store = server.rooms.thread(roomId, threadRootId);
  let state = await loadSettled(store, signal);
  if (state === 'disposed') store = server.rooms.thread(roomId, threadRootId);
  else if (state === 'failed') await untilAborted(store.restoreLatestWindow(), signal);
  if (state !== 'current') state = await loadSettled(store, signal);
  if (state !== 'current') throw new Error('Thread history is not ready');
  return selectContext(store, server.accountId, location, limit);
}

/** Include batch members outside the recent window. Unavailable messages produce safe notes. */
export async function includeCurrentMessages(
  server: Pick<Server, 'getMessage'>,
  roomId: string,
  context: ThreadContext,
  currentIds: readonly string[],
  signal: AbortSignal
): Promise<ThreadContext> {
  const messages = [...context.messages];
  const additional: ThreadContext['messages'] = [];
  const imageNotes = [...(context.imageNotes ?? [])];
  const represented = new Set(messages.map((message) => message.id));
  for (const id of currentIds) {
    if (represented.has(id)) continue;
    represented.add(id);
    signal.throwIfAborted();
    try {
      const message = await server.getMessage({ roomId, messageId: id }, { signal });
      signal.throwIfAborted();
      if (!message) throw new Error('Message unavailable');
      additional.push({
        id,
        author: message.authorId,
        text: message.body ?? '',
        attachments: message.attachments
      });
    } catch {
      signal.throwIfAborted();
      imageNotes.push(
        `Images for message ${id} could not be checked. Do not infer their contents.`
      );
    }
  }
  return { ...context, messages: [...additional, ...messages], imageNotes };
}

type LoadState = 'current' | 'failed' | 'disposed';

/** Resolve when the timeline is current, its read failed, or the client disposed it. */
function loadSettled(store: MessagesStore, signal: AbortSignal): Promise<LoadState> {
  return settled((): LoadState | undefined => {
    if (store.isCurrent) return 'current';
    if (store.isDisposed) return 'disposed';
    if (store.loadError) return 'failed';
    return undefined;
  }, signal);
}

/** Resolve with the first value of `read` that is not undefined. `read` reads reactive state. */
function settled<T>(read: () => T | undefined, signal: AbortSignal): Promise<T> {
  const state = computed(read);
  const current = state.get();
  if (current !== undefined) return Promise.resolve(current);
  return untilAborted(
    new Promise<T>((resolve) => {
      const stop = subscribe(state, () => {
        const value = state.get();
        if (value === undefined) return;
        stop();
        resolve(value);
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
    if (
      payload.kind !== TimelineEventKind.MessagePosted ||
      payload.deletedAt ||
      (!payload.body && !payload.attachments.length)
    )
      return [];
    return [
      {
        id: event.id,
        author: event.actorId ?? undefined,
        text: payload.body ?? '',
        ...(payload.attachments.length
          ? {
              attachments: payload.attachments.map(({ id, filename, contentType }) => ({
                id,
                filename,
                contentType
              }))
            }
          : {}),
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

/** The user directory service needed to read public participant names. */
export interface ParticipantSource {
  service(service: typeof UserService): Pick<Client<typeof UserService>, 'batchGetUsers'>;
}

/** Read current names for known authors. Unknown and deleted accounts have no name.
 * Only public names enter model context; errors remain retryable before effects. */
export async function readParticipants(
  server: ParticipantSource,
  authorIds: readonly (string | undefined)[],
  signal: AbortSignal
): Promise<NonNullable<ThreadContext['participants']>> {
  const ids = [...new Set(authorIds.filter((id): id is string => Boolean(id)))];
  const participants: NonNullable<ThreadContext['participants']> = [];
  for (let offset = 0; offset < ids.length; offset += 100) {
    signal.throwIfAborted();
    const requested = ids.slice(offset, offset + 100);
    const { users } = await server
      .service(UserService)
      .batchGetUsers({ userIds: requested }, { signal, timeoutMs: 10_000 });
    signal.throwIfAborted();
    for (const { user } of users) {
      if (!user || user.deleted || !requested.includes(user.id)) continue;
      participants.push({ id: user.id, username: user.login, displayName: user.displayName });
    }
  }
  return participants;
}
