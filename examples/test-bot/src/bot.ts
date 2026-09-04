import { createClient, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { MessageService } from "@chatto/api-types/api/v1/messages_connect";
import { RoomDirectoryService } from "@chatto/api-types/api/v1/room_directory_connect";
import { ThreadService } from "@chatto/api-types/api/v1/threads_connect";
import { UserService } from "@chatto/api-types/api/v1/user_service_connect";
import { ViewerService } from "@chatto/api-types/api/v1/viewer_connect";
import type { Message } from "@chatto/api-types/api/v1/message_types_pb";
import {
  RealtimeEvent,
  RealtimeInitialState,
  RealtimeServerFrame,
  RealtimeSubscribe,
} from "@chatto/api-types/realtime/v1/realtime_pb";
import { readFile } from "node:fs/promises";
import {
  loadTestBotState,
  rememberProcessedEvent,
  saveTestBotState,
  type TestBotState,
} from "./state.js";
import {
  createAIResponder,
  type AIResponder,
  type TestBotAIConfig,
} from "./ai.js";

const REALTIME_PROTOCOL_VERSION = 4;
const MINIMUM_RECONNECT_DELAY_MS = 250;
const MAXIMUM_RECONNECT_DELAY_MS = 10_000;

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

interface ReplyTarget {
  roomId: string;
  sourceEventId: string;
  threadRootEventId: string;
  directMention: boolean;
  sourceActorId: string;
  sourceBody?: string;
}

interface BotAPI {
  viewerId: string;
  followedThreads: Set<string>;
  isBotActor(actorId: string): Promise<boolean>;
  loadConversation(target: ReplyTarget): Promise<ConversationMessage[]>;
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
  state: TestBotState,
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
  const rooms = await createClient(RoomDirectoryService, transport).listRooms(
    {},
  );
  const messages = createClient(MessageService, transport);
  const threads = createClient(ThreadService, transport);
  const users = createClient(UserService, transport);
  const botActors = new Map<string, boolean>([[viewerId, true]]);
  const followedThreads = new Set<string>(
    state.resumeCursor ? state.followedThreadKeys : [],
  );
  if (!state.resumeCursor) {
    let offset = 0;
    for (;;) {
      const response = await threads.listFollowedThreads({
        page: { limit: 500, offset },
      });
      for (const followed of response.threads) {
        const roomId = followed.room?.id ?? followed.rootMessage?.roomId;
        const rootId =
          followed.thread?.threadRootEventId ?? followed.rootMessage?.id;
        if (roomId && rootId) followedThreads.add(threadKey(roomId, rootId));
      }
      if (!response.page?.hasMore || response.threads.length === 0) break;
      offset += response.threads.length;
    }
  }
  log({
    status: "api_ready",
    viewer_id: viewerId,
    visible_rooms: rooms.rooms.length,
    followed_threads: followedThreads.size,
  });
  return {
    viewerId,
    followedThreads,
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
      if (target.sourceBody === undefined) return [];
      const source = {
        eventId: target.sourceEventId,
        actorId: target.sourceActorId,
        body: target.sourceBody,
      };
      if (target.threadRootEventId === target.sourceEventId) {
        return [source];
      }
      const response = await threads.getThreadEvents({
        roomId: target.roomId,
        threadRootEventId: target.threadRootEventId,
        limit: 40,
      });
      const conversation = (response.page?.events ?? []).flatMap((event) => {
        if (event.event.case !== "messagePosted") return [];
        const message = event.event.value.message;
        return message?.body === undefined
          ? []
          : [conversationMessage(message)];
      });
      if (!conversation.some((message) => message.eventId === source.eventId)) {
        conversation.push(source);
      }
      return conversation;
    },
    async postReply(target, body): Promise<string> {
      const response = await messages.createMessage({
        roomId: target.roomId,
        body,
        threadRootEventId: target.threadRootEventId,
        inReplyTo: target.sourceEventId,
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

/** Build an identity-minimized transcript for the configured AI provider. */
export function conversationPrompt(
  messages: ConversationMessage[],
  viewerId: string,
): string {
  const people = new Map<string, number>();
  let nextPerson = 1;
  const lines = messages.slice(-40).map((message) => {
    let label = "Assistant";
    if (message.actorId !== viewerId) {
      let number = people.get(message.actorId);
      if (!number) {
        number = nextPerson++;
        people.set(message.actorId, number);
      }
      label = `Person ${number}`;
    }
    return `${label}: ${message.body.slice(0, 4_000)}`;
  });
  const selected: string[] = [];
  let length = 0;
  for (let index = lines.length - 1; index >= 0; index--) {
    const line = lines[index];
    if (!line) continue;
    const addedLength = line.length + (selected.length > 0 ? 2 : 0);
    if (length + addedLength > 32_000) break;
    selected.unshift(line);
    length += addedLength;
  }
  return selected.join("\n\n");
}

function threadKey(roomId: string, threadRootEventId: string): string {
  return `${roomId}\u0000${threadRootEventId}`;
}

/** Return a target for a direct mention or a new message in a followed thread. */
export function messageReplyTarget(
  event: RealtimeEvent,
  viewerId: string,
  followedThreads: ReadonlySet<string>,
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
  const directlyMentioned = message.mentions.some(
    (mention) => mention.userId === viewerId && mention.cause.case === "direct",
  );
  const threadRootEventId = message.inThread || event.id;
  if (
    !directlyMentioned &&
    (!message.inThread ||
      !followedThreads.has(threadKey(message.roomId, message.inThread)))
  ) {
    return undefined;
  }
  return {
    roomId: message.roomId,
    sourceEventId: event.id,
    threadRootEventId,
    directMention: directlyMentioned,
    sourceActorId: event.actorId,
    sourceBody: message.bodyPlaintext,
  };
}

function applyThreadFollowEvent(api: BotAPI, event: RealtimeEvent): void {
  if (event.event.case !== "threadViewerStateChanged") return;
  const change = event.event.value;
  if (!change.roomId || !change.threadRootEventId) return;
  const key = threadKey(change.roomId, change.threadRootEventId);
  if (change.isFollowing) api.followedThreads.add(key);
  else api.followedThreads.delete(key);
}

async function replyToMessage(
  api: BotAPI,
  ai: AIResponder,
  event: RealtimeEvent,
  signal: AbortSignal,
): Promise<void> {
  const target = messageReplyTarget(event, api.viewerId, api.followedThreads);
  if (!target) return;
  if (await api.isBotActor(target.sourceActorId)) return;
  if (target.directMention) {
    api.followedThreads.add(threadKey(target.roomId, target.threadRootEventId));
  }
  const conversation = await api.loadConversation(target);
  if (conversation.length === 0) return;
  const body = await ai.respond(
    conversationPrompt(conversation, api.viewerId),
    signal,
  );
  const replyEventId = await api.postReply(target, body);
  log({
    status: "ai_replied",
    source_event_id: target.sourceEventId,
    thread_root_event_id: target.threadRootEventId,
    reply_event_id: replyEventId,
    trigger: target.directMention ? "direct_mention" : "followed_thread",
  });
}

async function commitState(
  stateFile: string,
  state: TestBotState,
  followedThreads: ReadonlySet<string>,
  resumeCursor?: string,
): Promise<void> {
  if (resumeCursor) state.resumeCursor = resumeCursor;
  state.followedThreadKeys = [...followedThreads];
  await saveTestBotState(stateFile, state);
}

async function runRealtimeSession(
  config: TestBotConfig,
  apiKey: string,
  api: BotAPI,
  ai: AIResponder,
  state: TestBotState,
  signal: AbortSignal,
): Promise<SessionResult> {
  const resumed = Boolean(state.resumeCursor);
  const socket = new WebSocket(realtimeUrl(config.serverUrl));
  let processing = Promise.resolve();
  let requestedResult:
    Omit<SessionResult, "caughtUp" | "processingFailed"> | undefined;
  let processingFailed = false;
  let caughtUp = false;

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
      processing = processing
        .then(async () => {
          const frame = RealtimeServerFrame.fromBinary(
            await messageDataToBytes(message.data),
          );
          switch (frame.frame.case) {
            case "event": {
              const event = frame.frame.value;
              const firstDelivery = !state.processedEventIds.includes(event.id);
              if (firstDelivery) {
                log({
                  status: "event",
                  event: event.event.case ?? "unknown",
                  event_id: event.id,
                  ...(event.actorId ? { actor_id: event.actorId } : {}),
                });
                applyThreadFollowEvent(api, event);
                await replyToMessage(api, ai, event, signal);
                rememberProcessedEvent(state, event.id);
              } else {
                log({ status: "duplicate_ignored", event_id: event.id });
              }
              await commitState(
                config.stateFile,
                state,
                api.followedThreads,
                event.resumeCursor,
              );
              return;
            }
            case "heartbeat":
              if (frame.frame.value.resumeCursor) {
                await commitState(
                  config.stateFile,
                  state,
                  api.followedThreads,
                  frame.frame.value.resumeCursor,
                );
              }
              return;
            case "caughtUp":
              await commitState(
                config.stateFile,
                state,
                api.followedThreads,
                frame.frame.value.cursor,
              );
              caughtUp = true;
              log({ status: "caught_up", resumed });
              return;
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
        .catch((error: unknown) => {
          if (processingFailed) return;
          processingFailed = true;
          log({ status: "frame_failed", error: safeErrorKind(error) });
          socket.close(1003, "invalid realtime frame");
        });
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
        void processing.finally(() => {
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
  const ai = await createAIResponder(config.ai);
  log({ status: "ai_ready", provider: ai.provider, model: ai.model });
  let attempt = 0;
  while (!signal.aborted) {
    try {
      const apiKey = await readAPIKey(config.apiKeyFile);
      const api = await connectPublicAPI(config, apiKey, state);
      const session = await runRealtimeSession(
        config,
        apiKey,
        api,
        ai,
        state,
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
