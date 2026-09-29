export { startTyping, withTyping, type TypingUpdate } from './typing.js';
import { messageHelpers } from './messages.js';
export type { ChattoMessage } from './messages.js';
import { consumeRealtime, type ConsumeRealtimeOptions, type WebSocketFactory } from './realtime.js';
export type {
  ConsumeRealtimeOptions,
  RealtimeCheckpoint,
  RealtimeStatus,
  WebSocketFactory
} from './realtime.js';
export { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
export { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';

/** Connection settings supplied by the host; this package never reads environment files. */
export interface ChattoClientOptions {
  serverUrl: string;
  apiKey: string;
  /** Optional transport for tests or host-specific networking. */
  fetch?: typeof fetch;
  /** Must connect directly, without following redirects. Defaults to the host WebSocket. */
  webSocket?: WebSocketFactory;
}

/** A thread destination, independent of the workflow that sends the message. */
export interface Destination {
  roomId: string;
  threadRootId: string;
  /** Message that prompted this reply, within the destination thread. */
  inReplyTo?: string;
}

/** Send ordered thread messages; workflow adapters supply their cancellation signal. */
export type ChattoPost = (
  destination: Destination,
  body: string,
  signal: AbortSignal
) => Promise<void>;

/** Refresh a thread's typing indicator once. */
export type ChattoTyping = (destination: Destination, signal: AbortSignal) => Promise<void>;

/** Locate a thread independently of a bot or webhook delivery. */
export interface ThreadLocation {
  roomId: string;
  threadRootId: string;
}

/** A textual thread message; attachments and non-message events are omitted. */
export interface ThreadMessage {
  id: string;
  authorId?: string;
  /** The author's display name, or login when no display name is set. */
  authorName?: string;
  authorLogin?: string;
  body: string;
}

/** Messages from one thread read, with a cursor for reading newer messages later. */
export interface ThreadRead {
  messages: ThreadMessage[];
  /** Pass as `after` to read only newer messages. Absent when the thread has no messages. */
  cursor?: string;
  /** True when older replies exist that this read left out. */
  olderOmitted: boolean;
}

interface ThreadEvent {
  id?: string;
  messagePosted?: { message?: { actorId?: string; body?: string } };
}

interface ThreadPage {
  events?: ThreadEvent[];
  hasOlder?: boolean;
  hasNewer?: boolean;
  startCursor?: string;
  endCursor?: string;
  includes?: { users?: Record<string, { login?: string; displayName?: string }> };
}

/** Most replies that one thread page returns. */
const THREAD_PAGE_LIMIT = 100;

/** Create a bearer-authenticated client for the Chatto 0.5 Connect JSON API. */
export function createChattoClient(options: ChattoClientOptions) {
  const base = new URL(options.serverUrl);
  if (!['http:', 'https:'].includes(base.protocol) || base.username || base.password) {
    throw new Error('Use an HTTP or HTTPS Chatto server URL without credentials');
  }
  const request = options.fetch ?? globalThis.fetch;

  /**
   * Call a public resource RPC without retries or redirects.
   * Response types describe protobuf JSON, not generated protobuf classes.
   * Errors omit server bodies, URLs, credentials, and transport error details.
   */
  async function rpc<T>(method: string, body: object, signal?: AbortSignal): Promise<T> {
    signal?.throwIfAborted();
    if (!/^[A-Za-z][A-Za-z0-9]*Service\/[A-Za-z][A-Za-z0-9]*$/.test(method)) {
      throw new Error('Expected a Chatto resource service and method');
    }
    let response: Response;
    try {
      response = await request(new URL(`/api/connect/chatto.api.v1.${method}`, base), {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${options.apiKey}`,
          'Content-Type': 'application/json',
          'Connect-Protocol-Version': '1'
        },
        body: JSON.stringify(body),
        redirect: 'error',
        signal: AbortSignal.any([...(signal ? [signal] : []), AbortSignal.timeout(10_000)])
      });
    } catch {
      if (signal?.aborted) throw signal.reason;
      throw new Error('Chatto API request did not complete');
    }
    if (!response.ok) throw new Error(`Chatto API returned HTTP ${response.status}`);
    try {
      return (await response.json()) as T;
    } catch {
      if (signal?.aborted) throw signal.reason;
      throw new Error('Chatto API returned invalid JSON');
    }
  }

  /** Send one message without retrying; callers own any uncertain delivery outcome. */
  function createMessage(
    destination: Destination,
    body: string,
    signal?: AbortSignal,
    inReplyTo?: string
  ) {
    return rpc<{ message?: { id?: string } }>(
      'MessageService/CreateMessage',
      {
        roomId: destination.roomId,
        body,
        threadRootEventId: destination.threadRootId,
        ...((inReplyTo ?? destination.inReplyTo)
          ? { inReplyTo: inReplyTo ?? destination.inReplyTo }
          : {})
      },
      signal
    );
  }

  /** Split at 8000 Unicode code points and send in order; a failed chunk stops delivery. */
  async function postMessage(
    destination: Destination,
    body: string,
    signal?: AbortSignal
  ): Promise<void> {
    const characters = Array.from(body);
    for (let offset = 0; offset < Math.max(1, characters.length); offset += 8000) {
      await createMessage(destination, characters.slice(offset, offset + 8000).join(''), signal);
    }
  }

  /** Refresh presence once; the host owns periodic refresh and cancellation. */
  async function refreshTyping(destination: Destination, signal?: AbortSignal): Promise<void> {
    await rpc(
      'RoomService/RefreshTypingIndicator',
      {
        roomId: destination.roomId,
        threadRootEventId: destination.threadRootId
      },
      signal
    );
  }

  /** Add a reaction to a message event, which can differ from the thread root. */
  async function addReaction(
    roomId: string,
    messageEventId: string,
    emoji: string,
    signal?: AbortSignal
  ): Promise<void> {
    await rpc('MessageService/AddReaction', { roomId, messageEventId, emoji }, signal);
  }

  /** Read all history pages with a 30-second total limit, preserving root-first order. */
  /**
   * Read a thread. Without `after`, returns the root and the newest `limit` replies (100 by
   * default); `olderOmitted` tells whether older replies exist. With `after`, a cursor from an
   * earlier read, returns only the messages posted after it. Authors carry the names that
   * Chatto includes with each page. Rejects missing pages and repeated pagination cursors.
   */
  async function readThread(
    location: ThreadLocation,
    signal?: AbortSignal,
    { after, limit = THREAD_PAGE_LIMIT }: { after?: string; limit?: number } = {}
  ): Promise<ThreadRead> {
    const rootId = location.threadRootId;
    const readSignal = AbortSignal.any([...(signal ? [signal] : []), AbortSignal.timeout(30_000)]);
    const readPage = async (cursor?: { after: string }) => {
      const { page } = await rpc<{ page?: ThreadPage }>(
        'ThreadService/GetThreadEvents',
        { roomId: location.roomId, threadRootEventId: rootId, limit, ...cursor },
        readSignal
      );
      if (!page) throw new Error('Chatto did not return the thread page');
      return page;
    };
    const pages: ThreadPage[] = [];
    let cursor = after;
    let olderOmitted = false;
    if (after === undefined) {
      const page = await readPage();
      pages.push(page);
      cursor = page.endCursor || undefined;
      olderOmitted = !!page.hasOlder;
    } else {
      const seen = new Set<string>();
      do {
        if (seen.has(cursor!)) throw new Error('Thread pagination did not advance');
        seen.add(cursor!);
        const page = await readPage({ after: cursor! });
        pages.push(page);
        if (!page.hasNewer) {
          cursor = page.endCursor || cursor;
          break;
        }
        if (!page.endCursor) throw new Error('Thread pagination did not advance');
        cursor = page.endCursor;
      } while (true);
    }

    const users = Object.assign(
      {},
      ...pages.map((page) => page.includes?.users ?? {})
    ) as NonNullable<NonNullable<ThreadPage['includes']>['users']>;
    const events = pages.flatMap((page) => page.events ?? []);
    const root = events.find((event) => event.id === rootId);
    const seen = new Set<string>();
    const messages = [
      ...(root ? [root] : []),
      ...events.filter((event) => event.id !== rootId)
    ].flatMap((event) => {
      if (!event.id || seen.has(event.id)) return [];
      seen.add(event.id);
      const message = event.messagePosted?.message;
      if (!message?.body) return [];
      const user = message.actorId ? users[message.actorId] : undefined;
      const name = user?.displayName || user?.login;
      return [
        {
          id: event.id,
          authorId: message.actorId,
          ...(name ? { authorName: name } : {}),
          ...(user?.login ? { authorLogin: user.login } : {}),
          body: message.body
        }
      ];
    });
    return { messages, ...(cursor ? { cursor } : {}), olderOmitted };
  }

  return {
    ...messageHelpers(rpc),
    rpc,
    createMessage,
    postMessage,
    refreshTyping,
    addReaction,
    readThread,
    /** Consume ordered events until cancellation or a terminal failure. */
    consumeRealtime: (settings: ConsumeRealtimeOptions) =>
      consumeRealtime(base, options.apiKey, options.webSocket, settings)
  };
}

/** The small integration client, with no Runling or UI-framework dependency. */
export type ChattoClient = ReturnType<typeof createChattoClient>;
