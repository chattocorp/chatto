import {
  Code,
  ConnectError,
  createClient,
  type Interceptor,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { MessageService } from "@chatto/api-types/api/v1/messages_connect";
import { RoomDirectoryService } from "@chatto/api-types/api/v1/room_directory_connect";
import { RoomService } from "@chatto/api-types/api/v1/rooms_connect";
import { ThreadService } from "@chatto/api-types/api/v1/threads_connect";
import { UserService } from "@chatto/api-types/api/v1/user_service_connect";
import { ViewerService } from "@chatto/api-types/api/v1/viewer_connect";
import type { Message } from "@chatto/api-types/api/v1/message_types_pb";
import type { RoomTimelineEvent } from "@chatto/api-types/api/v1/room_timeline_pb";
import { RoomKind } from "@chatto/api-types/api/v1/rooms_pb";
import {
  RealtimeEvent,
  RealtimeInitialState,
  RealtimeServerFrame,
  RealtimeSubscribe,
} from "@chatto/api-types/realtime/v1/realtime_pb";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import {
  loadTestBotState,
  rememberProcessedEvent,
  serialTestBotStateSaver,
  type TestBotStateSaver,
  type TestBotState,
} from "./state.js";
import {
  createAIResponder,
  type AIConversation,
  type AIResponder,
  type TestBotAIConfig,
} from "./ai.js";

const REALTIME_PROTOCOL_VERSION = 4;
const MINIMUM_RECONNECT_DELAY_MS = 250;
const MAXIMUM_RECONNECT_DELAY_MS = 10_000;
const MAXIMUM_CONCURRENT_REPLIES = 8;
const MAXIMUM_CONVERSATION_MESSAGES = 40;
const MAXIMUM_CONVERSATION_CHARACTERS = 32_000;
const MAXIMUM_MESSAGE_CHARACTERS = 4_000;
const TYPING_REFRESH_INTERVAL_MS = 2_000;

/** Runtime configuration for the public-API example bot. */
export interface TestBotConfig {
  serverUrl: string;
  apiKeyFile: string;
  stateFile: string;
  ai: TestBotAIConfig;
}

interface SessionResult {
  reconnect: boolean;
  caughtUp: boolean;
  processingFailed: boolean;
  retryAfterMs?: number;
}

interface ReplyTargetBase {
  roomId: string;
  sourceEventId: string;
  sourceActorId: string;
  sourceBody: string;
}

/** Source message and conversation placement for one TestBot reply. */
export type ReplyTarget = ReplyTargetBase &
  (
    | {
        scope: { case: "thread"; threadRootEventId: string };
      }
    | {
        scope: { case: "directMessage" };
      }
  );

/** Narrow public API operations required by the reply workflow. */
export interface BotAPI {
  /** Authenticated TestBot user ID. */
  viewerId: string;
  /** Return the public kind of a visible room. */
  roomKind(roomId: string): Promise<RoomKind>;
  /** Return whether the source actor is another bot. */
  isBotActor(actorId: string): Promise<boolean>;
  /** Read the current conversation that contains the source message. */
  loadConversation(target: ReplyTarget): Promise<ConversationMessage[]>;
  /** Publish one live-only typing indicator for the target conversation. */
  updateTypingIndicator(target: ReplyTarget): Promise<void>;
  /** Create a reply and return its message event ID. */
  postReply(target: ReplyTarget, body: string): Promise<string>;
}

/** Minimal message data sent to the configured AI provider. */
export interface ConversationMessage {
  eventId: string;
  actorId: string;
  body: string;
}

function log(record: Record<string, boolean | number | string>): void {
  console.log(JSON.stringify({ component: "test_bot", ...record }));
}

function safeErrorKind(error: unknown): string {
  return error instanceof Error && error.name ? error.name : "UnknownError";
}

function realtimeUrl(serverUrl: string): string {
  const url = new URL("/api/realtime", serverUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

async function messageDataToBytes(data: unknown): Promise<Uint8Array> {
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  if (ArrayBuffer.isView(data)) {
    return new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
  }
  if (data instanceof Blob) return new Uint8Array(await data.arrayBuffer());
  throw new Error("unsupported WebSocket message data");
}

function retryAfterMilliseconds(seconds: bigint, nanos: number): number {
  return Number(seconds) * 1_000 + Math.ceil(nanos / 1_000_000);
}

async function readAPIKey(apiKeyFile: string): Promise<string> {
  const apiKey = (await readFile(apiKeyFile, "utf8")).trim();
  if (!apiKey) throw new Error("bot API key file is empty");
  return apiKey;
}

async function connectPublicAPI(
  config: TestBotConfig,
  apiKey: string,
): Promise<BotAPI> {
  const authorization: Interceptor = (next) => async (request) => {
    request.header.set("Authorization", `Bearer ${apiKey}`);
    return next(request);
  };
  const transport = createConnectTransport({
    baseUrl: new URL("/api/connect", config.serverUrl).toString(),
    interceptors: [authorization],
  });
  const viewer = await createClient(ViewerService, transport).getViewer({});
  const viewerId = viewer.user?.profile?.id;
  if (!viewerId) throw new Error("viewer response did not contain a user ID");
  const roomDirectory = createClient(RoomDirectoryService, transport);
  const rooms = await roomDirectory.listRooms({});
  const messages = createClient(MessageService, transport);
  const roomOperations = createClient(RoomService, transport);
  const threads = createClient(ThreadService, transport);
  const users = createClient(UserService, transport);
  const botActors = new Map<string, boolean>([[viewerId, true]]);
  const roomKinds = new Map<string, RoomKind>();
  for (const entry of rooms.rooms) {
    if (entry.room?.id) roomKinds.set(entry.room.id, entry.room.kind);
  }
  log({
    status: "api_ready",
    viewer_id: viewerId,
    visible_rooms: rooms.rooms.length,
  });
  return {
    viewerId,
    async roomKind(roomId): Promise<RoomKind> {
      const cached = roomKinds.get(roomId);
      if (cached !== undefined) return cached;
      const response = await roomDirectory.getRoom({ roomId });
      const room = response.room?.room;
      if (!room) throw new Error("room response did not contain a room");
      roomKinds.set(room.id, room.kind);
      return room.kind;
    },
    async isBotActor(actorId): Promise<boolean> {
      const cached = botActors.get(actorId);
      if (cached !== undefined) return cached;
      const response = await users.getUser({
        target: { case: "userId", value: actorId },
      });
      const user = response.user?.user;
      if (!user) throw new Error("user response did not contain a user");
      botActors.set(actorId, user.isBot);
      return user.isBot;
    },
    async loadConversation(target): Promise<ConversationMessage[]> {
      const source = {
        eventId: target.sourceEventId,
        actorId: target.sourceActorId,
        body: target.sourceBody,
      };
      if (
        target.scope.case === "thread" &&
        target.scope.threadRootEventId === target.sourceEventId
      ) {
        return [source];
      }
      let events: RoomTimelineEvent[];
      try {
        if (target.scope.case === "directMessage") {
          const response = await roomOperations.getRoomEventsAround({
            roomId: target.roomId,
            eventId: target.sourceEventId,
            limit: MAXIMUM_CONVERSATION_MESSAGES,
          });
          events = response.page?.events ?? [];
        } else {
          const response = await threads.getThreadEventsAround({
            roomId: target.roomId,
            threadRootEventId: target.scope.threadRootEventId,
            eventId: target.sourceEventId,
            limit: MAXIMUM_CONVERSATION_MESSAGES,
          });
          events = response.page?.events ?? [];
        }
      } catch (error) {
        if (ConnectError.from(error).code === Code.NotFound) return [source];
        throw error;
      }
      const conversation = conversationThroughSource(events, source.eventId);
      if (!conversation.some((message) => message.eventId === source.eventId)) {
        conversation.push(source);
      }
      return conversation;
    },
    async updateTypingIndicator(target): Promise<void> {
      const response = await roomOperations.updateTypingIndicator({
        roomId: target.roomId,
        threadRootEventId:
          target.scope.case === "thread"
            ? target.scope.threadRootEventId
            : undefined,
      });
      if (!response.updated) {
        throw new Error("typing indicator was not accepted");
      }
    },
    async postReply(target, body): Promise<string> {
      const response = await messages.createMessage({
        roomId: target.roomId,
        body,
        ...(target.scope.case === "thread"
          ? {
              threadRootEventId: target.scope.threadRootEventId,
              inReplyTo: target.sourceEventId,
            }
          : {}),
      });
      const replyEventId = response.message?.id;
      if (!replyEventId) {
        throw new Error("message response did not contain an event ID");
      }
      return replyEventId;
    },
  };
}

function conversationMessage(message: Message): ConversationMessage {
  return {
    eventId: message.id,
    actorId: message.actorId,
    body: message.body ?? "",
  };
}

function conversationThroughSource(
  events: RoomTimelineEvent[],
  sourceEventId: string,
): ConversationMessage[] {
  const sourceIndex = events.findIndex((event) => event.id === sourceEventId);
  return events
    .slice(0, sourceIndex >= 0 ? sourceIndex + 1 : events.length)
    .flatMap((event) => {
      if (event.event.case !== "messagePosted") return [];
      const message = event.event.value.message;
      return message?.body === undefined ? [] : [conversationMessage(message)];
    });
}

function threadKey(roomId: string, threadRootEventId: string): string {
  return `${roomId}\0${threadRootEventId}`;
}

function conversationKey(target: ReplyTarget): string {
  return target.scope.case === "thread"
    ? threadKey(target.roomId, target.scope.threadRootEventId)
    : `${target.roomId}\0direct-message`;
}

function conversationSessionId(target: ReplyTarget): string {
  const kind = target.scope.case === "thread" ? "thread" : "dm";
  return `chatto-${kind}-${createHash("sha256")
    .update(conversationKey(target))
    .digest("hex")
    .slice(0, 32)}`;
}

function participantLabel(key: string, actorId: string): string {
  const suffix = createHash("sha256")
    .update(key)
    .update("\0")
    .update(actorId)
    .digest("hex")
    .slice(0, 8);
  return `Person ${suffix}`;
}

/** Build a bounded, identity-minimized Pi conversation from one Chatto scope. */
export function chatAIConversation(
  messages: ConversationMessage[],
  viewerId: string,
  target: ReplyTarget,
): AIConversation {
  const sourceIndex = messages.findIndex(
    (message) => message.eventId === target.sourceEventId,
  );
  if (sourceIndex < 0) {
    throw new Error("conversation snapshot did not contain the source message");
  }
  const key = conversationKey(target);
  const available = messages
    .slice(0, sourceIndex + 1)
    .slice(-MAXIMUM_CONVERSATION_MESSAGES)
    .map((message) => {
      const body = message.body.slice(0, MAXIMUM_MESSAGE_CHARACTERS);
      return message.actorId === viewerId
        ? ({ role: "assistant", content: body } as const)
        : ({
            role: "user",
            content: `${participantLabel(key, message.actorId)}: ${body}`,
          } as const);
    });
  const selected: typeof available = [];
  let length = 0;
  for (let index = available.length - 1; index >= 0; index--) {
    const turn = available[index];
    if (!turn) continue;
    const addedLength = turn.content.length;
    if (length + addedLength > MAXIMUM_CONVERSATION_CHARACTERS) break;
    selected.unshift(turn);
    length += addedLength;
  }
  if (selected.at(-1)?.role !== "user") {
    throw new Error("conversation did not end with a human message");
  }
  return {
    sessionId: conversationSessionId(target),
    turns: selected,
  };
}

/** Return a reply target for a DM message or a direct mention in a channel. */
export function messageReplyTarget(
  event: RealtimeEvent,
  viewerId: string,
  roomKind: RoomKind,
): ReplyTarget | undefined {
  if (event.actorId === viewerId || event.event.case !== "messagePosted") {
    return undefined;
  }
  const message = event.event.value;
  if (
    !event.id ||
    !event.actorId ||
    !message.roomId ||
    message.echoOfEventId ||
    message.bodyPlaintext === undefined
  ) {
    return undefined;
  }
  if (roomKind !== RoomKind.CHANNEL && roomKind !== RoomKind.DM) return undefined;
  if (roomKind === RoomKind.CHANNEL) {
    const directlyMentioned = message.mentions.some(
      (mention) =>
        mention.userId === viewerId && mention.cause.case === "direct",
    );
    if (!directlyMentioned) return undefined;
  }
  const base = {
    roomId: message.roomId,
    sourceEventId: event.id,
    sourceActorId: event.actorId,
    sourceBody: message.bodyPlaintext,
  };
  if (roomKind === RoomKind.DM) {
    return { ...base, scope: { case: "directMessage" } };
  }
  return {
    ...base,
    scope: {
      case: "thread",
      threadRootEventId: message.inThread || event.id,
    },
  };
}

function candidateRoomId(
  event: RealtimeEvent,
  viewerId: string,
): string | undefined {
  if (event.actorId === viewerId || event.event.case !== "messagePosted") {
    return undefined;
  }
  return event.event.value.roomId || undefined;
}

function targetLogFields(
  target: ReplyTarget,
): Record<string, boolean | number | string> {
  return target.scope.case === "thread"
    ? { thread_root_event_id: target.scope.threadRootEventId }
    : { direct_message: true };
}

/** Refresh a conversation typing indicator until the operation stops. */
export async function refreshTypingIndicator(
  api: Pick<BotAPI, "updateTypingIndicator">,
  target: ReplyTarget,
  signal: AbortSignal,
  intervalMs = TYPING_REFRESH_INTERVAL_MS,
): Promise<void> {
  while (!signal.aborted) {
    await wait(intervalMs, signal);
    if (signal.aborted) return;
    await api.updateTypingIndicator(target);
  }
}

/** Publish typing activity while Pi prepares one final reply. */
export async function replyToMessage(
  api: BotAPI,
  ai: AIResponder,
  event: RealtimeEvent,
  signal: AbortSignal,
): Promise<void> {
  const roomId = candidateRoomId(event, api.viewerId);
  if (!roomId) return;
  const target = messageReplyTarget(
    event,
    api.viewerId,
    await api.roomKind(roomId),
  );
  if (!target) return;
  if (await api.isBotActor(target.sourceActorId)) return;
  if (signal.aborted) throw new DOMException("Aborted", "AbortError");

  const typingController = new AbortController();
  const abortTyping = () => typingController.abort(signal.reason);
  if (signal.aborted) {
    abortTyping();
  } else {
    signal.addEventListener("abort", abortTyping, { once: true });
  }
  let typingStarted = false;
  let typingRefresh = Promise.resolve();
  try {
    await api.updateTypingIndicator(target);
    typingStarted = true;
    log({
      status: "typing_started",
      source_event_id: target.sourceEventId,
      ...targetLogFields(target),
    });
    typingRefresh = refreshTypingIndicator(
      api,
      target,
      typingController.signal,
    ).catch((error: unknown) => {
      log({ status: "typing_failed", error: safeErrorKind(error) });
    });
  } catch (error) {
    log({ status: "typing_failed", error: safeErrorKind(error) });
  }
  log({
    status: "ai_reply_started",
    source_event_id: target.sourceEventId,
    ...targetLogFields(target),
    trigger:
      target.scope.case === "directMessage"
        ? "direct_message"
        : "direct_mention",
  });

  try {
    const conversation = await api.loadConversation(target);
    if (conversation.length === 0) {
      throw new Error("conversation did not contain the source message");
    }
    const body = await ai.respond(
      chatAIConversation(conversation, api.viewerId, target),
      signal,
    );
    const replyEventId = await api.postReply(target, body);
    log({
      status: "ai_replied",
      source_event_id: target.sourceEventId,
      ...targetLogFields(target),
      reply_event_id: replyEventId,
      trigger:
        target.scope.case === "directMessage"
          ? "direct_message"
          : "direct_mention",
    });
  } finally {
    typingController.abort();
    signal.removeEventListener("abort", abortTyping);
    await typingRefresh;
    if (typingStarted) {
      log({
        status: "typing_stopped",
        source_event_id: target.sourceEventId,
        ...targetLogFields(target),
      });
    }
  }
}

async function commitState(
  saveState: TestBotStateSaver,
  state: TestBotState,
  resumeCursor?: string,
): Promise<void> {
  if (resumeCursor) state.resumeCursor = resumeCursor;
  await saveState(state);
}

type WorkOutcome = { ok: true } | { ok: false; error: unknown };

/** Run bounded work concurrently and commit its results in submission order. */
export class OrderedConcurrentProcessor {
  readonly #maximumConcurrency: number;
  readonly #waiters: Array<() => void> = [];
  readonly #work = new Set<Promise<WorkOutcome>>();
  #active = 0;
  #commitTail = Promise.resolve();

  constructor(maximumConcurrency: number) {
    if (!Number.isInteger(maximumConcurrency) || maximumConcurrency < 1) {
      throw new Error("maximum concurrency must be a positive integer");
    }
    this.#maximumConcurrency = maximumConcurrency;
  }

  async #acquire(): Promise<void> {
    if (this.#active < this.#maximumConcurrency) {
      this.#active += 1;
      return;
    }
    await new Promise<void>((resolve) => this.#waiters.push(resolve));
  }

  #release(): void {
    const next = this.#waiters.shift();
    if (next) {
      next();
      return;
    }
    this.#active -= 1;
  }

  async #run(work: () => Promise<void>): Promise<WorkOutcome> {
    await this.#acquire();
    try {
      await work();
      return { ok: true };
    } catch (error) {
      return { ok: false, error };
    } finally {
      this.#release();
    }
  }

  /** Start work when a slot is free, then run its commit after earlier commits. */
  enqueue(
    work: () => Promise<void>,
    commit: () => Promise<void>,
  ): Promise<void> {
    const outcome = this.#run(work);
    this.#work.add(outcome);
    void outcome.then(() => this.#work.delete(outcome));
    const committed = this.#commitTail.then(async () => {
      const result = await outcome;
      if (!result.ok) throw result.error;
      await commit();
    });
    this.#commitTail = committed;
    return committed;
  }

  /** Wait for all work submitted so far and its ordered commits. */
  async wait(): Promise<void> {
    let commitError: unknown;
    let commitFailed = false;
    await this.#commitTail.catch((error: unknown) => {
      commitFailed = true;
      commitError = error;
    });
    await Promise.all(this.#work);
    if (commitFailed) throw commitError;
  }
}

async function runRealtimeSession(
  config: TestBotConfig,
  apiKey: string,
  api: BotAPI,
  ai: AIResponder,
  state: TestBotState,
  saveState: TestBotStateSaver,
  signal: AbortSignal,
): Promise<SessionResult> {
  const resumed = Boolean(state.resumeCursor);
  const socket = new WebSocket(realtimeUrl(config.serverUrl));
  const workController = new AbortController();
  const abortWork = () => workController.abort(signal.reason);
  signal.addEventListener("abort", abortWork, { once: true });
  const processor = new OrderedConcurrentProcessor(MAXIMUM_CONCURRENT_REPLIES);
  const scheduledEventIds = new Set<string>();
  let intake = Promise.resolve();
  let requestedResult:
    Omit<SessionResult, "caughtUp" | "processingFailed"> | undefined;
  let processingFailed = false;
  let caughtUp = false;

  const fail = (error: unknown) => {
    if (processingFailed) return;
    processingFailed = true;
    workController.abort(error);
    log({ status: "processing_failed", error: safeErrorKind(error) });
    socket.close(1011, "event processing failed");
  };

  const enqueue = (work: () => Promise<void>, commit: () => Promise<void>) => {
    void processor.enqueue(work, commit).catch(fail);
  };

  const result = await new Promise<SessionResult>((resolve) => {
    const abort = () => {
      if (socket.readyState === WebSocket.OPEN) {
        socket.close(1000, "shutdown");
      }
    };
    signal.addEventListener("abort", abort, { once: true });

    socket.addEventListener(
      "open",
      () => {
        if (signal.aborted) {
          socket.close(1000, "shutdown");
          return;
        }
        const encoded = new RealtimeSubscribe({
          protocolVersion: REALTIME_PROTOCOL_VERSION,
          bearerToken: apiKey,
          resumeCursor: state.resumeCursor,
          initialState: RealtimeInitialState.LIVE_ONLY,
        }).toBinary();
        const payload = new Uint8Array(encoded.byteLength);
        payload.set(encoded);
        socket.send(payload.buffer);
      },
      { once: true },
    );

    socket.addEventListener("message", (message) => {
      intake = intake
        .then(async () => {
          if (processingFailed) return;
          const frame = RealtimeServerFrame.fromBinary(
            await messageDataToBytes(message.data),
          );
          switch (frame.frame.case) {
            case "event": {
              const event = frame.frame.value;
              const firstDelivery =
                !state.processedEventIds.includes(event.id) &&
                !scheduledEventIds.has(event.id);
              if (firstDelivery) {
                scheduledEventIds.add(event.id);
                log({
                  status: "event",
                  event: event.event.case ?? "unknown",
                  event_id: event.id,
                  ...(event.actorId ? { actor_id: event.actorId } : {}),
                });
              }
              enqueue(
                firstDelivery
                  ? () => replyToMessage(api, ai, event, workController.signal)
                  : async () => undefined,
                async () => {
                  if (firstDelivery) {
                    rememberProcessedEvent(state, event.id);
                    scheduledEventIds.delete(event.id);
                  } else {
                    log({ status: "duplicate_ignored", event_id: event.id });
                  }
                  await commitState(saveState, state, event.resumeCursor);
                },
              );
              return;
            }
            case "heartbeat": {
              const resumeCursor = frame.frame.value.resumeCursor;
              enqueue(
                async () => undefined,
                async () => {
                  if (resumeCursor) {
                    await commitState(saveState, state, resumeCursor);
                  }
                },
              );
              return;
            }
            case "caughtUp": {
              const cursor = frame.frame.value.cursor;
              enqueue(
                async () => undefined,
                async () => {
                  await commitState(saveState, state, cursor);
                  caughtUp = true;
                  log({ status: "caught_up", resumed });
                },
              );
              return;
            }
            case "snapshot":
              log({
                status: "snapshot",
                rooms: frame.frame.value.rooms.length,
                users: frame.frame.value.users.length,
                active_calls: frame.frame.value.activeCalls.length,
              });
              return;
            case "close": {
              const close = frame.frame.value;
              requestedResult = {
                reconnect: close.reconnect,
                ...(close.retryAfter
                  ? {
                      retryAfterMs: retryAfterMilliseconds(
                        close.retryAfter.seconds,
                        close.retryAfter.nanos,
                      ),
                    }
                  : {}),
              };
              log({
                status: "server_close",
                code: close.code,
                reconnect: close.reconnect,
              });
              socket.close(1000, "server close");
              return;
            }
            default:
              throw new Error("unknown realtime frame");
          }
        })
        .catch(fail);
    });

    socket.addEventListener(
      "error",
      (error) => {
        log({ status: "socket_error", error: safeErrorKind(error) });
      },
      { once: true },
    );
    socket.addEventListener(
      "close",
      () => {
        signal.removeEventListener("abort", abort);
        void intake
          .then(() => processor.wait())
          .catch(() => undefined)
          .finally(() => {
            signal.removeEventListener("abort", abortWork);
            resolve({
              ...(signal.aborted
                ? { reconnect: false }
                : (requestedResult ?? { reconnect: true })),
              caughtUp,
              processingFailed,
            });
          });
      },
      { once: true },
    );
  });
  return result;
}

function reconnectDelay(attempt: number, requestedDelay?: number): number {
  if (requestedDelay !== undefined) {
    return Math.min(
      MAXIMUM_RECONNECT_DELAY_MS,
      Math.max(MINIMUM_RECONNECT_DELAY_MS, requestedDelay),
    );
  }
  return Math.min(
    MAXIMUM_RECONNECT_DELAY_MS,
    MINIMUM_RECONNECT_DELAY_MS * 2 ** attempt,
  );
}

function wait(delayMs: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const finish = () => {
      clearTimeout(timer);
      signal.removeEventListener("abort", finish);
      resolve();
    };
    const timer = setTimeout(finish, delayMs);
    signal.addEventListener("abort", finish, { once: true });
  });
}

/** Run until the process receives a shutdown signal or the server forbids reconnect. */
export async function runTestBot(
  config: TestBotConfig,
  signal: AbortSignal,
): Promise<void> {
  const state = await loadTestBotState(config.stateFile);
  const saveState = serialTestBotStateSaver(config.stateFile);
  let ai: AIResponder | undefined;
  let attempt = 0;
  while (!signal.aborted) {
    try {
      if (!ai) {
        ai = await createAIResponder(config.ai);
        log({ status: "ai_ready", provider: ai.provider, model: ai.model });
      }
      const apiKey = await readAPIKey(config.apiKeyFile);
      const api = await connectPublicAPI(config, apiKey);
      const session = await runRealtimeSession(
        config,
        apiKey,
        api,
        ai,
        state,
        saveState,
        signal,
      );
      if (session.caughtUp && !session.processingFailed) attempt = 0;
      if (!session.reconnect || signal.aborted) return;
      const delayMs = reconnectDelay(attempt, session.retryAfterMs);
      log({ status: "reconnecting", delay_ms: delayMs });
      await wait(delayMs, signal);
      attempt = Math.min(attempt + 1, 5);
    } catch (error) {
      const delayMs = reconnectDelay(attempt);
      log({
        status: "waiting",
        error: safeErrorKind(error),
        delay_ms: delayMs,
      });
      await wait(delayMs, signal);
      attempt = Math.min(attempt + 1, 5);
    }
  }
}
