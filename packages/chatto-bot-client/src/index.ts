/**
 * Bot conventions on top of a `@chatto/client` connection.
 *
 * `createBotClient` wraps a {@link ChattoConnection} with the operations that
 * bots need: message delivery in chunks, complete thread reads, reactions,
 * typing refreshes, addressing recognition, and an ordered realtime event
 * loop. It adds no environment loading, persistence, or workflow lifecycle.
 */

import type { ServiceType } from '@bufbuild/protobuf';
import type { Client } from '@connectrpc/connect';
import type { ChattoConnection, RealtimeEvent } from '@chatto/client';
import { effect, effectRoot, untrack } from '@chatto/client/reactivity';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type { RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import { addressedMessage, type AddressingOptions } from './addressing.js';
import type {
  ChattoMessage,
  Destination,
  RealtimeStatus,
  ThreadLocation,
  ThreadMessage,
  ThreadRead
} from './types.js';

export {
  addressedMessage,
  type AddressedMessage,
  type AddressingReason,
  type AddressingOptions
} from './addressing.js';
export { createDeliveryTracker, type DeliveryTracker } from './deliveries.js';
export { startTyping, withTyping, type TypingUpdate } from './typing.js';
export type {
  ChattoMessage,
  ChattoPost,
  ChattoTyping,
  Destination,
  RealtimeStatus,
  ThreadLocation,
  ThreadMessage,
  ThreadRead
} from './types.js';

/** Timeout of one request. The client never retries requests. */
const REQUEST_TIMEOUT_MS = 10_000;
/** Total timeout of a complete thread read. */
const THREAD_READ_TIMEOUT_MS = 30_000;
/** Largest message body in Unicode code points. */
const MESSAGE_CHUNK_LENGTH = 8000;
/** Thread events requested per page. */
const THREAD_PAGE_SIZE = 100;
/**
 * Largest number of received events that wait for the handler. When a slow
 * handler falls further behind, the waiting events are dropped and a gap is
 * reported, so memory stays bounded and the bot stays responsive.
 */
const MAX_QUEUED_EVENTS = 1000;

/** Thread text classified relative to this bot; other bots also have the human role. */
export interface BotThreadMessage extends ThreadMessage {
  role: 'bot' | 'human';
}

/** A thread read whose messages carry roles relative to this bot. */
export interface BotThreadRead extends Omit<ThreadRead, 'messages'> {
  messages: BotThreadMessage[];
}

/** Options for a thread read; see {@link BotApi.readThread}. */
export interface ThreadReadOptions {
  /** A `cursor` from an earlier read: return only newer messages. */
  after?: string;
  /** Most replies to return without `after`. Default: 100. */
  limit?: number;
}

/**
 * Read thread text with roles relative to a known bot identity. `options`
 * selects the newest replies or, with `after`, only newer messages.
 */
export async function readBotThread(
  client: {
    readThread(
      location: ThreadLocation,
      signal?: AbortSignal,
      options?: ThreadReadOptions
    ): Promise<ThreadRead>;
  },
  viewerId: string,
  location: ThreadLocation,
  signal?: AbortSignal,
  options?: ThreadReadOptions
): Promise<BotThreadRead> {
  const read = await client.readThread(location, signal, options);
  return {
    ...read,
    messages: read.messages.map((message) => ({
      ...message,
      role: message.authorId === viewerId ? 'bot' : 'human'
    }))
  };
}

/** Reply in the original thread and reference the message that prompted the reply. */
export function replyDestination(
  message: Pick<ChattoMessage, 'id' | 'roomId' | 'threadRootId'>
): Destination {
  return {
    roomId: message.roomId,
    threadRootId: message.threadRootId ?? message.id,
    inReplyTo: message.id
  };
}

/** Default conversation scope: bot, room, thread, and sender. Scope storage to one server.
 * Hosts can use their own key when participants should share a conversation. */
export function conversationKey(
  viewerId: string,
  message: Pick<ChattoMessage, 'id' | 'roomId' | 'threadRootId' | 'authorId'>
): string {
  return JSON.stringify([
    viewerId,
    message.roomId,
    message.threadRootId ?? message.id,
    message.authorId
  ]);
}

/** Options for {@link BotClient.consumeEvents}. */
export interface ConsumeEventsOptions {
  /** Stops consumption. The returned promise resolves when it aborts. */
  signal: AbortSignal;
  /**
   * Handle one event. Events are handled in order; the next event waits until
   * the returned promise resolves. A rejection stops consumption. When more
   * than 1000 received events wait, they are dropped and a gap is reported.
   */
  onEvent: (event: RealtimeEvent) => void | Promise<void>;
  /**
   * Receive realtime status changes. Repeated statuses are not reported
   * again, but each gap is. A throw stops consumption; the status change in
   * the connection is not affected.
   */
  onStatus?: (status: RealtimeStatus) => void;
}

/**
 * Call options for one request: the caller's cancellation and the request
 * timeout. A cancelled caller sends nothing.
 */
function callOptions(signal: AbortSignal | undefined, timeoutMs = REQUEST_TIMEOUT_MS) {
  signal?.throwIfAborted();
  return { signal, timeoutMs };
}

/** Anything that creates typed service clients: a `ChattoApi` or a `ChattoConnection`. */
export interface ServiceSource {
  service<T extends ServiceType>(service: T): Client<T>;
}

/**
 * Request helpers for a bot with a known identity. `viewerId` must be the
 * user that the source's token authenticates. These helpers need no realtime
 * connection, so short-lived hosts such as webhook handlers can use them.
 */
export function createBotApi(source: ServiceSource, viewerId: string) {
  const messages = source.service(MessageService);
  const rooms = source.service(RoomService);
  const threads = source.service(ThreadService);

  /** Read one visible message. Missing or mismatched identities return undefined.
   * RPC failures reject; hosts choose whether to retry or ignore them. */
  async function getMessage({
    roomId,
    messageId,
    signal
  }: {
    roomId: string;
    messageId: string;
    signal?: AbortSignal;
  }): Promise<ChattoMessage | undefined> {
    const { message } = await messages.getMessage(
      { roomId, eventId: messageId },
      callOptions(signal)
    );
    if (!message || message.id !== messageId || message.roomId !== roomId || !message.actorId)
      return;
    return {
      id: message.id,
      roomId: message.roomId,
      authorId: message.actorId,
      ...(message.threadRootEventId ? { threadRootId: message.threadRootEventId } : {}),
      ...(message.inReplyTo ? { inReplyTo: message.inReplyTo } : {}),
      ...(message.body !== undefined ? { body: message.body } : {})
    };
  }

  /** Send one message without retrying; callers own any uncertain delivery outcome.
   * An explicit `inReplyTo` takes precedence over the destination's. */
  async function createMessage(
    destination: Destination,
    body: string,
    signal?: AbortSignal,
    inReplyTo?: string
  ): Promise<{ id: string | undefined }> {
    const response = await messages.createMessage(
      {
        roomId: destination.roomId,
        body,
        threadRootEventId: destination.threadRootId,
        inReplyTo: inReplyTo ?? destination.inReplyTo ?? ''
      },
      callOptions(signal)
    );
    return { id: response.message?.id };
  }

  /** Split at 8000 Unicode code points and send in order. A failed chunk stops
   * delivery; earlier chunks can already have been delivered. */
  async function postMessage(
    destination: Destination,
    body: string,
    signal?: AbortSignal
  ): Promise<void> {
    const characters = Array.from(body);
    for (let offset = 0; offset < Math.max(1, characters.length); offset += MESSAGE_CHUNK_LENGTH) {
      await createMessage(
        destination,
        characters.slice(offset, offset + MESSAGE_CHUNK_LENGTH).join(''),
        signal
      );
    }
  }

  /** Refresh the thread's typing indicator once. */
  async function refreshTyping(destination: Destination, signal?: AbortSignal): Promise<void> {
    await rooms.refreshTypingIndicator(
      { roomId: destination.roomId, threadRootEventId: destination.threadRootId },
      callOptions(signal)
    );
  }

  /** Add a reaction to a message event, which can differ from the thread root. */
  async function addReaction(
    roomId: string,
    messageEventId: string,
    emoji: string,
    signal?: AbortSignal
  ): Promise<void> {
    await messages.addReaction({ roomId, messageEventId, emoji }, callOptions(signal));
  }

  /**
   * Read a thread within 30 seconds. Without `after`, returns the root and the
   * newest `limit` replies (100 by default); `olderOmitted` tells whether older
   * replies exist. With `after`, a `cursor` from an earlier read, returns only
   * newer messages and reads as many pages as needed. Messages carry the
   * author's display name and login from the page. Rejects an empty `after`
   * cursor (omit `after` to read from the start), missing pages, and
   * pagination that does not advance.
   */
  async function readThread(
    location: ThreadLocation,
    signal?: AbortSignal,
    { after, limit = THREAD_PAGE_SIZE }: { after?: string; limit?: number } = {}
  ): Promise<ThreadRead> {
    if (after === '') throw new Error('A thread cursor must not be empty');
    const readSignal = AbortSignal.any([
      ...(signal ? [signal] : []),
      AbortSignal.timeout(THREAD_READ_TIMEOUT_MS)
    ]);
    const rootId = location.threadRootId;
    const readPage = async (cursor?: string) => {
      const { page } = await threads.getThreadEvents(
        {
          roomId: location.roomId,
          threadRootEventId: rootId,
          limit,
          cursor: cursor ? { case: 'after', value: cursor } : { case: undefined }
        },
        callOptions(readSignal)
      );
      if (!page) throw new Error('Chatto did not return the thread page');
      return page;
    };
    const pages: RoomTimelinePage[] = [];
    let cursor = after;
    let olderOmitted = false;
    if (after === undefined) {
      const page = await readPage();
      pages.push(page);
      cursor = page.endCursor || undefined;
      olderOmitted = page.hasOlder;
    } else {
      const seen = new Set<string>();
      while (true) {
        if (seen.has(cursor!)) throw new Error('Thread pagination did not advance');
        seen.add(cursor!);
        const page = await readPage(cursor);
        pages.push(page);
        if (!page.hasNewer) {
          cursor = page.endCursor || cursor;
          break;
        }
        if (!page.endCursor) throw new Error('Thread pagination did not advance');
        cursor = page.endCursor;
      }
    }

    const users = Object.assign({}, ...pages.map((page) => page.includes?.users ?? {})) as Record<
      string,
      { login: string; displayName: string }
    >;
    const events = pages.flatMap((page) => page.events);
    const root = events.find((event) => event.id === rootId);
    const seen = new Set<string>();
    const messages = [
      ...(root ? [root] : []),
      ...events.filter((event) => event.id !== rootId)
    ].flatMap((event): ThreadMessage[] => {
      if (!event.id || seen.has(event.id) || event.event.case !== 'messagePosted') return [];
      seen.add(event.id);
      const message = event.event.value.message;
      if (!message?.body) return [];
      const user = message.actorId ? users[message.actorId] : undefined;
      const name = user?.displayName || user?.login;
      return [
        {
          id: event.id,
          authorId: message.actorId || undefined,
          ...(name ? { authorName: name } : {}),
          ...(user?.login ? { authorLogin: user.login } : {}),
          body: message.body
        }
      ];
    });
    return { messages, ...(cursor ? { cursor } : {}), olderOmitted };
  }

  const client = { getMessage, readThread };

  return {
    viewerId,
    getMessage,
    createMessage,
    postMessage,
    refreshTyping,
    addReaction,
    readThread,
    addressedMessage: (event: RealtimeEvent, options: AddressingOptions = {}) =>
      addressedMessage(client, event, { viewerId, ...options }),
    conversationKey: (message: ChattoMessage) => conversationKey(viewerId, message),
    reply: (message: ChattoMessage, body: string, signal?: AbortSignal) =>
      postMessage(replyDestination(message), body, signal),
    readBotThread: (location: ThreadLocation, signal?: AbortSignal, options?: ThreadReadOptions) =>
      readBotThread(client, viewerId, location, signal, options)
  };
}

export type BotApi = ReturnType<typeof createBotApi>;

/**
 * Wait until the connection accepted its token, then create the bot client:
 * the {@link createBotApi} helpers and an ordered realtime event loop.
 * Rejects like {@link ChattoConnection.ready}: when the server rejects the
 * token, when the viewer read fails (for example because the server is
 * unreachable), when the connection closes, or when `signal` aborts.
 */
export async function createBotClient(
  chatto: ChattoConnection,
  { signal }: { signal?: AbortSignal } = {}
) {
  const { viewerId } = await chatto.ready({ signal });

  /**
   * Handle realtime events in order until `signal` aborts. The connection
   * keeps receiving events while a handler runs; up to 1000 wait in memory.
   * Resolves on abort. Rejects when `onEvent` or `onStatus` throws, when the
   * server ends the session, or when the connection closes.
   */
  async function consumeEvents({ signal, onEvent, onStatus }: ConsumeEventsOptions): Promise<void> {
    signal.throwIfAborted();
    let queue: RealtimeEvent[] = [];
    let wake: (() => void) | undefined;
    let failure: { error: unknown } | undefined;
    const fail = (error: unknown) => {
      failure ??= { error };
      wake?.();
    };
    let lastStatus: string | undefined;
    // Report outside reactive tracking, and only changes. A failing status
    // callback stops consumption; it must not throw into the connection code
    // that changed the status.
    const report = (status: RealtimeStatus) => {
      const key = JSON.stringify(status);
      if (key === lastStatus && !('gap' in status && status.gap)) return;
      lastStatus = key;
      try {
        untrack(() => onStatus?.(status));
      } catch (error) {
        fail(error);
      }
    };
    // A reset gap is reported with the next `ready`, so hosts are not told
    // `ready` while the stream reconnects or a replacement snapshot loads.
    let pendingGap = false;
    const stopEvents = chatto.onEvent((event) => {
      if (queue.length >= MAX_QUEUED_EVENTS) {
        // Keep the bot responsive: drop the backlog and report the loss. The
        // stream itself is healthy, so report at once when it is connected.
        queue = [];
        if (untrack(() => chatto.status) === 'connected') report({ state: 'ready', gap: true });
        else pendingGap = true;
      }
      queue.push(event);
      wake?.();
    });
    const stopResets = chatto.onReset(({ gap }) => {
      if (gap) pendingGap = true;
    });
    let connectedBefore = false;
    const stopEffects = effectRoot(() => {
      effect(() => {
        if (chatto.closed) fail(new Error('The Chatto connection is closed'));
        else if (chatto.sessionEnded)
          fail(new Error('Chatto ended the session; the API key can be revoked'));
      });
      effect(() => {
        const status = chatto.status;
        // A closed connection or ended session does not reconnect; the loop
        // rejects instead of reporting a reconnect.
        if (failure || untrack(() => chatto.closed || chatto.sessionEnded)) return;
        if (status === 'connected') {
          connectedBefore = true;
          report({ state: 'ready', gap: pendingGap });
          pendingGap = false;
        } else if (status === 'connecting') {
          report({ state: connectedBefore ? 'reconnecting' : 'connecting' });
        } else if (status === 'disconnected') {
          report({ state: 'reconnecting' });
        }
      });
    });
    // Abort wakes the loop directly; no promise outlives one wait.
    const onAbort = () => wake?.();
    signal.addEventListener('abort', onAbort, { once: true });
    try {
      while (!signal.aborted) {
        if (failure) throw failure.error;
        const event = queue.shift();
        if (!event) {
          await new Promise<void>((resolve) => (wake = resolve));
          wake = undefined;
          continue;
        }
        await onEvent(event);
      }
    } finally {
      stopEvents();
      stopResets();
      stopEffects();
      signal.removeEventListener('abort', onAbort);
    }
  }

  return { ...createBotApi(chatto, viewerId), chatto, consumeEvents };
}

export type BotClient = Awaited<ReturnType<typeof createBotClient>>;
