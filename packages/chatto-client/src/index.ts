export { startTyping, withTyping, type TypingUpdate } from "./typing.js";
import { messageHelpers } from "./messages.js";
export type { ChattoMessage } from "./messages.js";
import { consumeRealtime, type ConsumeRealtimeOptions, type WebSocketFactory } from "./realtime.js";
export type { ConsumeRealtimeOptions, RealtimeCheckpoint, RealtimeStatus, WebSocketFactory } from "./realtime.js";
export { RealtimeEvent } from "@chatto/api-types/realtime/v1/realtime_pb";
export { RoomKind } from "@chatto/api-types/api/v1/rooms_pb";

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
export type ChattoPost = (destination: Destination, body: string, signal: AbortSignal) => Promise<void>;

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
  body: string;
}

interface ThreadEvent {
  id?: string;
  messagePosted?: { message?: { actorId?: string; body?: string } };
}

/** Create a bearer-authenticated client for the Chatto 0.5 Connect JSON API. */
export function createChattoClient(options: ChattoClientOptions) {
  const base = new URL(options.serverUrl);
  if (!["http:", "https:"].includes(base.protocol) || base.username || base.password) {
    throw new Error("Use an HTTP or HTTPS Chatto server URL without credentials");
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
      throw new Error("Expected a Chatto resource service and method");
    }
    let response: Response;
    try {
      response = await request(new URL(`/api/connect/chatto.api.v1.${method}`, base), {
        method: "POST",
        headers: {
          Authorization: `Bearer ${options.apiKey}`,
          "Content-Type": "application/json",
          "Connect-Protocol-Version": "1",
        },
        body: JSON.stringify(body),
        redirect: "error",
        signal: AbortSignal.any([...(signal ? [signal] : []), AbortSignal.timeout(10_000)]),
      });
    } catch {
      if (signal?.aborted) throw signal.reason;
      throw new Error("Chatto API request did not complete");
    }
    if (!response.ok) throw new Error(`Chatto API returned HTTP ${response.status}`);
    try {
      return await response.json() as T;
    } catch {
      if (signal?.aborted) throw signal.reason;
      throw new Error("Chatto API returned invalid JSON");
    }
  }

  /** Send one message without retrying; callers own any uncertain delivery outcome. */
  function createMessage(destination: Destination, body: string, signal?: AbortSignal, inReplyTo?: string) {
    return rpc<{ message?: { id?: string } }>("MessageService/CreateMessage", {
      roomId: destination.roomId, body, threadRootEventId: destination.threadRootId,
      ...((inReplyTo ?? destination.inReplyTo) ? { inReplyTo: inReplyTo ?? destination.inReplyTo } : {}),
    }, signal);
  }

  /** Split at 8000 Unicode code points and send in order; a failed chunk stops delivery. */
  async function postMessage(destination: Destination, body: string, signal?: AbortSignal): Promise<void> {
    const characters = Array.from(body);
    for (let offset = 0; offset < Math.max(1, characters.length); offset += 8000) {
      await createMessage(destination, characters.slice(offset, offset + 8000).join(""), signal);
    }
  }

  /** Refresh presence once; the host owns periodic refresh and cancellation. */
  async function refreshTyping(destination: Destination, signal?: AbortSignal): Promise<void> {
    await rpc("RoomService/RefreshTypingIndicator", {
      roomId: destination.roomId, threadRootEventId: destination.threadRootId,
    }, signal);
  }

  /** Add a reaction to a message event, which can differ from the thread root. */
  async function addReaction(roomId: string, messageEventId: string, emoji: string, signal?: AbortSignal): Promise<void> {
    await rpc("MessageService/AddReaction", { roomId, messageEventId, emoji }, signal);
  }

  /** Read all history pages with a 30-second total limit, preserving root-first order. */
  async function readThread(location: ThreadLocation, signal?: AbortSignal): Promise<ThreadMessage[]> {
    const rootId = location.threadRootId;
    const readSignal = AbortSignal.any([...(signal ? [signal] : []), AbortSignal.timeout(30_000)]);
    let root: ThreadEvent | undefined;
    let replies: ThreadEvent[] = [];
    let before: string | undefined;
    const cursors = new Set<string>();
    do {
      const { page } = await rpc<{
        page?: { events?: ThreadEvent[]; hasOlder?: boolean; startCursor?: string };
      }>("ThreadService/GetThreadEvents", {
        roomId: location.roomId, threadRootEventId: rootId, limit: 100,
        ...(before ? { before } : {}),
      }, readSignal);
      if (!page) throw new Error("Chatto did not return the thread page");
      const events = page.events ?? [];
      root ??= events.find(event => event.id === rootId);
      replies = [...events.filter(event => event.id !== rootId), ...replies];
      if (!page.hasOlder) break;
      before = page.startCursor;
      if (!before || cursors.has(before)) throw new Error("Thread pagination did not advance");
      cursors.add(before);
    } while (true);

    const seen = new Set<string>();
    return [...(root ? [root] : []), ...replies].flatMap(event => {
      if (!event.id || seen.has(event.id)) return [];
      seen.add(event.id);
      const message = event.messagePosted?.message;
      if (!message?.body) return [];
      return [{
        id: event.id, authorId: message.actorId,
        body: message.body,
      }];
    });
  }

  return {
    ...messageHelpers(rpc),
    rpc, createMessage, postMessage, refreshTyping, addReaction, readThread,
    /** Consume ordered events until cancellation or a terminal failure. */
    consumeRealtime: (settings: ConsumeRealtimeOptions) => consumeRealtime(base, options.apiKey, options.webSocket, settings),
  };
}

/** The small integration client, with no Runling or UI-framework dependency. */
export type ChattoClient = ReturnType<typeof createChattoClient>;
