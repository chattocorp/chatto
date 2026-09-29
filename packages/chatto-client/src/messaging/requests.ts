/**
 * Request helpers for messages, threads, reactions, typing, and addressing.
 *
 * A {@link Connection} and a stateless `Api` both expose these helpers. They
 * need no realtime connection. Each request has a ten-second timeout and is
 * never retried: a failed write can still have reached the server.
 */

import type { ServiceType } from '@bufbuild/protobuf';
import type { Client } from '@connectrpc/connect';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type { RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { withTyping } from './typing.js';
import type {
  ChattoMessage,
  Destination,
  RequestOptions,
  ThreadLocation,
  ThreadMessage,
  ThreadRead,
  ThreadReadOptions
} from './types.js';

/** Timeout of one request. */
const REQUEST_TIMEOUT_MS = 10_000;
/** Total timeout of a complete thread read. */
const THREAD_READ_TIMEOUT_MS = 30_000;
/** Largest message body in Unicode code points. */
const MESSAGE_CHUNK_LENGTH = 8000;
/** Thread events requested per page. */
const THREAD_PAGE_SIZE = 100;

/** Why a message is addressed to the viewer; see `addressedMessage`. */
export type AddressingReason = 'direct_message' | 'mention' | 'reply';

/** Options for `addressedMessage`. */
export interface AddressingOptions extends RequestOptions {
  /** The reasons to recognize. Default: all three. */
  reasons?: readonly AddressingReason[];
}

/** A textual message addressed to the viewer, the account of the API key. */
export interface AddressedMessage extends ChattoMessage {
  body: string;
  /** When the message was posted, as an ISO 8601 timestamp. */
  occurredAt?: string;
  reasons: AddressingReason[];
}

/** Anything that creates typed service clients, such as a connection or an `Api`. */
export interface ServiceSource {
  service<T extends ServiceType>(service: T): Client<T>;
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

/**
 * The default conversation scope: viewer, room, thread, and sender. Keep keys
 * of different servers apart. Hosts can use their own key when participants
 * should share a conversation.
 */
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

/**
 * Call options for one request: the caller's cancellation and the request
 * timeout. A cancelled caller sends nothing.
 */
function callOptions(signal: AbortSignal | undefined, timeoutMs = REQUEST_TIMEOUT_MS) {
  signal?.throwIfAborted();
  return { signal, timeoutMs };
}

/** Request helpers bound to one server and one viewer. */
export class MessagingRequests {
  readonly #messages: Client<typeof MessageService>;
  readonly #rooms: Client<typeof RoomService>;
  readonly #threads: Client<typeof ThreadService>;
  readonly #viewerId: (options: RequestOptions) => Promise<string>;

  /**
   * `viewerId` returns the account of the source's token. Thread reads and
   * addressing use it.
   */
  constructor(source: ServiceSource, viewerId: (options: RequestOptions) => Promise<string>) {
    this.#messages = source.service(MessageService);
    this.#rooms = source.service(RoomService);
    this.#threads = source.service(ThreadService);
    this.#viewerId = viewerId;
  }

  /**
   * Read one visible message. Returns undefined when it is missing or does
   * not match the room. Request failures reject.
   */
  async getMessage(
    { roomId, messageId }: { roomId: string; messageId: string },
    { signal }: RequestOptions = {}
  ): Promise<ChattoMessage | undefined> {
    const { message } = await this.#messages.getMessage(
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
      // A message without text has an empty body; report it as absent.
      ...(message.body ? { body: message.body } : {})
    };
  }

  /**
   * Send one message. An explicit `inReplyTo` takes precedence over the
   * destination's. Returns the ID of the new message. Rejects when the
   * response has no message ID, although the server can have created it.
   */
  async createMessage(
    destination: Destination,
    body: string,
    { signal, inReplyTo }: RequestOptions & { inReplyTo?: string } = {}
  ): Promise<{ id: string }> {
    const response = await this.#messages.createMessage(
      {
        roomId: destination.roomId,
        body,
        threadRootEventId: destination.threadRootId,
        inReplyTo: inReplyTo ?? destination.inReplyTo ?? ''
      },
      callOptions(signal)
    );
    const id = response.message?.id;
    if (!id) throw new Error('Chatto did not return the ID of the new message');
    return { id };
  }

  /**
   * Send text of any length: it is split at 8000 Unicode code points and the
   * parts are sent in order. A failed part stops delivery; earlier parts can
   * already have been delivered. Returns the IDs of the new messages, in order.
   */
  async postMessage(
    destination: Destination,
    body: string,
    options: RequestOptions = {}
  ): Promise<{ ids: string[] }> {
    const characters = Array.from(body);
    const ids: string[] = [];
    for (let offset = 0; offset < Math.max(1, characters.length); offset += MESSAGE_CHUNK_LENGTH) {
      const { id } = await this.createMessage(
        destination,
        characters.slice(offset, offset + MESSAGE_CHUNK_LENGTH).join(''),
        options
      );
      ids.push(id);
    }
    return { ids };
  }

  /** Post in the message's thread with a reference to the message; see {@link postMessage}. */
  reply(
    message: ChattoMessage,
    body: string,
    options: RequestOptions = {}
  ): Promise<{ ids: string[] }> {
    return this.postMessage(replyDestination(message), body, options);
  }

  /** Refresh the thread's typing indicator once. */
  async refreshTyping(destination: Destination, { signal }: RequestOptions = {}): Promise<void> {
    await this.#rooms.refreshTypingIndicator(
      { roomId: destination.roomId, threadRootEventId: destination.threadRootId },
      callOptions(signal)
    );
  }

  /**
   * Show the typing indicator in the destination's thread while `work` runs.
   * Typing failures are ignored; the result or error is the work's.
   */
  withTyping<Result>(
    destination: Destination,
    work: () => Promise<Result>,
    { signal = new AbortController().signal }: RequestOptions = {}
  ): Promise<Result> {
    return withTyping(
      signal,
      (refreshSignal) => this.refreshTyping(destination, { signal: refreshSignal }),
      work
    );
  }

  /** Add a reaction to a message, which can differ from the thread root. */
  async addReaction(
    { roomId, messageId }: { roomId: string; messageId: string },
    emoji: string,
    { signal }: RequestOptions = {}
  ): Promise<void> {
    await this.#messages.addReaction(
      { roomId, messageEventId: messageId, emoji },
      callOptions(signal)
    );
  }

  /**
   * Read a thread within 30 seconds. Without `after`, returns the root and the
   * newest `limit` replies (100 by default); `olderOmitted` tells whether older
   * replies exist. With `after`, a `cursor` from an earlier read, returns only
   * newer messages and reads as many pages as needed. Rejects an empty `after`
   * cursor (omit `after` to read from the start), missing pages, and
   * pagination that does not advance.
   */
  async readThread(
    location: ThreadLocation,
    { signal, after, limit = THREAD_PAGE_SIZE }: ThreadReadOptions = {}
  ): Promise<ThreadRead> {
    if (after === '') throw new Error('A thread cursor must not be empty');
    const readSignal = AbortSignal.any([
      ...(signal ? [signal] : []),
      AbortSignal.timeout(THREAD_READ_TIMEOUT_MS)
    ]);
    const viewerId = await this.#viewerId({ signal: readSignal });
    const rootId = location.threadRootId;
    const readPage = async (cursor?: string) => {
      const { page } = await this.#threads.getThreadEvents(
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
          body: message.body,
          fromViewer: message.actorId === viewerId
        }
      ];
    });
    return { messages, ...(cursor ? { cursor } : {}), olderOmitted };
  }

  /**
   * Recognize a realtime event as a message addressed to the viewer: a
   * textual direct message, a mention of the viewer, or a reply to one of the
   * viewer's messages. Only an unmentioned reply needs a lookup, which
   * verifies the author, room, and thread. The viewer's own messages,
   * other events, and messages without text return undefined. Lookup errors
   * and cancellation reject.
   */
  async addressedMessage(
    event: RealtimeEvent,
    { signal, reasons: enabled = ['direct_message', 'mention', 'reply'] }: AddressingOptions = {}
  ): Promise<AddressedMessage | undefined> {
    signal?.throwIfAborted();
    if (event.event.case !== 'messagePosted' || !event.id || !event.actorId) return;
    const viewerId = await this.#viewerId({ signal });
    if (event.actorId === viewerId) return;
    const message = event.event.value;
    if (message.bodyPlaintext === undefined || !message.roomId) return;
    const reasons: AddressingReason[] = [];
    if (enabled.includes('direct_message') && message.roomKind === RoomKind.DM)
      reasons.push('direct_message');
    if (enabled.includes('mention') && message.mentions.some((mention) => mention.includesViewer))
      reasons.push('mention');
    if (enabled.includes('reply') && !reasons.length && message.inReplyTo) {
      const target = await this.getMessage(
        { roomId: message.roomId, messageId: message.inReplyTo },
        { signal }
      );
      signal?.throwIfAborted();
      if (
        target?.authorId === viewerId &&
        (target.threadRootId || target.id) === (message.threadRootEventId || message.inReplyTo)
      )
        reasons.push('reply');
    }
    if (!reasons.length) return;
    return {
      id: event.id,
      roomId: message.roomId,
      authorId: event.actorId,
      body: message.bodyPlaintext,
      ...(message.threadRootEventId ? { threadRootId: message.threadRootEventId } : {}),
      ...(message.inReplyTo ? { inReplyTo: message.inReplyTo } : {}),
      ...(event.createdAt ? { occurredAt: event.createdAt.toDate().toISOString() } : {}),
      reasons
    };
  }
}
