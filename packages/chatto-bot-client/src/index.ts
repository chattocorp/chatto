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
import { effect, effectRoot } from '@chatto/client/reactivity';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { addressedMessage, type AddressingOptions } from './addressing.js';
import type {
  ChattoMessage,
  Destination,
  RealtimeStatus,
  ThreadLocation,
  ThreadMessage
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
  ThreadMessage
} from './types.js';

/** Timeout of one request. The client never retries requests. */
const REQUEST_TIMEOUT_MS = 10_000;
/** Total timeout of a complete thread read. */
const THREAD_READ_TIMEOUT_MS = 30_000;
/** Largest message body in Unicode code points. */
const MESSAGE_CHUNK_LENGTH = 8000;
/** Thread events requested per page. */
const THREAD_PAGE_SIZE = 100;

/** Thread text classified relative to this bot; other bots also have the human role. */
export interface BotThreadMessage extends ThreadMessage {
  role: 'bot' | 'human';
}

/** Read thread text with roles relative to a known bot identity. */
export async function readBotThread(
  client: { readThread(location: ThreadLocation, signal?: AbortSignal): Promise<ThreadMessage[]> },
  viewerId: string,
  location: ThreadLocation,
  signal?: AbortSignal
): Promise<BotThreadMessage[]> {
  return (await client.readThread(location, signal)).map((message) => ({
    ...message,
    role: message.authorId === viewerId ? 'bot' : 'human'
  }));
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
  signal: AbortSignal;
  /**
   * Handle one event. Events are handled in order; the next event waits until
   * the returned promise resolves. A rejection stops consumption.
   */
  onEvent: (event: RealtimeEvent) => void | Promise<void>;
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

  /** Read all history pages within 30 seconds, root first. Returns textual
   * messages; rejects missing pages and pagination that does not advance. */
  async function readThread(
    location: ThreadLocation,
    signal?: AbortSignal
  ): Promise<ThreadMessage[]> {
    const readSignal = AbortSignal.any([
      ...(signal ? [signal] : []),
      AbortSignal.timeout(THREAD_READ_TIMEOUT_MS)
    ]);
    const rootId = location.threadRootId;
    let root: ThreadMessage | undefined;
    let replies: ThreadMessage[] = [];
    let before: string | undefined;
    const cursors = new Set<string>();
    const seen = new Set<string>();
    const textOf = (event: {
      id: string;
      event: { case: string | undefined; value?: unknown };
    }): ThreadMessage[] => {
      if (event.event.case !== 'messagePosted') return [];
      const message = (event.event.value as { message?: { actorId: string; body?: string } })
        .message;
      if (!message?.body) return [];
      return [{ id: event.id, authorId: message.actorId || undefined, body: message.body }];
    };
    while (true) {
      const { page } = await threads.getThreadEvents(
        {
          roomId: location.roomId,
          threadRootEventId: rootId,
          limit: THREAD_PAGE_SIZE,
          cursor: before ? { case: 'before', value: before } : { case: undefined }
        },
        callOptions(readSignal)
      );
      if (!page) throw new Error('Chatto did not return the thread page');
      const pageReplies: ThreadMessage[] = [];
      for (const event of page.events) {
        if (!event.id || seen.has(event.id)) continue;
        seen.add(event.id);
        const [text] = textOf(event);
        if (event.id === rootId) root = text ?? root;
        else if (text) pageReplies.push(text);
      }
      replies = [...pageReplies, ...replies];
      if (!page.hasOlder) break;
      before = page.startCursor;
      if (!before || cursors.has(before)) throw new Error('Thread pagination did not advance');
      cursors.add(before);
    }
    return [...(root ? [root] : []), ...replies];
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
    readBotThread: (location: ThreadLocation, signal?: AbortSignal) =>
      readBotThread(client, viewerId, location, signal)
  };
}

export type BotApi = ReturnType<typeof createBotApi>;

/**
 * Wait until the connection accepted its token, then create the bot client:
 * the {@link createBotApi} helpers and an ordered realtime event loop.
 * Rejects when the server rejects the token or when `signal` aborts.
 */
export async function createBotClient(
  chatto: ChattoConnection,
  { signal }: { signal?: AbortSignal } = {}
) {
  const { viewerId } = await chatto.ready({ signal });

  /**
   * Handle realtime events in order until `signal` aborts or `onEvent` rejects.
   * The connection keeps receiving events while a handler runs; they wait in
   * memory. Resolves on abort; rejects with the handler's error.
   */
  async function consumeEvents({ signal, onEvent, onStatus }: ConsumeEventsOptions): Promise<void> {
    signal.throwIfAborted();
    const queue: RealtimeEvent[] = [];
    let wake: (() => void) | undefined;
    let resets = 0;
    const stopEvents = chatto.onEvent((event) => {
      queue.push(event);
      wake?.();
    });
    const stopResets = chatto.onReset(() => {
      resets++;
      // The first snapshot starts the stream. A later one replaces a stream
      // that the server could not resume.
      if (resets > 1) onStatus?.({ state: 'ready', gap: true });
    });
    let connectedBefore = false;
    const stopStatus = effectRoot(() => {
      effect(() => {
        const status = chatto.connection.status;
        if (status === 'connected') {
          onStatus?.({ state: 'ready', gap: false });
          connectedBefore = true;
        } else if (status === 'connecting') {
          onStatus?.({ state: connectedBefore ? 'reconnecting' : 'connecting' });
        } else if (status === 'disconnected') {
          onStatus?.({ state: 'reconnecting' });
        }
      });
    });
    const aborted = new Promise<void>((resolve) =>
      signal.addEventListener('abort', () => resolve(), { once: true })
    );
    try {
      while (!signal.aborted) {
        const event = queue.shift();
        if (!event) {
          await Promise.race([new Promise<void>((resolve) => (wake = resolve)), aborted]);
          wake = undefined;
          continue;
        }
        await onEvent(event);
      }
    } finally {
      stopEvents();
      stopResets();
      stopStatus();
    }
  }

  return { ...createBotApi(chatto, viewerId), chatto, consumeEvents };
}

export type BotClient = Awaited<ReturnType<typeof createBotClient>>;
