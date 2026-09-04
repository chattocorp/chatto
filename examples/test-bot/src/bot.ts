import { createClient, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { MessageService } from "@chatto/api-types/api/v1/messages_connect";
import { RoomDirectoryService } from "@chatto/api-types/api/v1/room_directory_connect";
import { ViewerService } from "@chatto/api-types/api/v1/viewer_connect";
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

const REALTIME_PROTOCOL_VERSION = 4;
const MINIMUM_RECONNECT_DELAY_MS = 250;
const MAXIMUM_RECONNECT_DELAY_MS = 10_000;

/** Runtime configuration for the public-API example bot. */
export interface TestBotConfig {
  serverUrl: string;
  apiKeyFile: string;
  stateFile: string;
}

interface SessionResult {
  reconnect: boolean;
  caughtUp: boolean;
  retryAfterMs?: number;
}

interface MentionReplyTarget {
  roomId: string;
  sourceEventId: string;
  threadRootEventId: string;
}

interface BotAPI {
  viewerId: string;
  replyToMention(target: MentionReplyTarget): Promise<string>;
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
  const rooms = await createClient(RoomDirectoryService, transport).listRooms(
    {},
  );
  const messages = createClient(MessageService, transport);
  log({
    status: "api_ready",
    viewer_id: viewerId,
    visible_rooms: rooms.rooms.length,
  });
  return {
    viewerId,
    async replyToMention(target): Promise<string> {
      const response = await messages.createMessage({
        roomId: target.roomId,
        body: "Hello! I received your mention.",
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

/** Return a reply target only for a direct mention of this bot. */
export function mentionReplyTarget(
  event: RealtimeEvent,
  viewerId: string,
): MentionReplyTarget | undefined {
  if (event.actorId === viewerId || event.event.case !== "messagePosted") {
    return undefined;
  }
  const message = event.event.value;
  if (!event.id || !message.roomId || message.echoOfEventId) return undefined;
  const directlyMentioned = message.mentions.some(
    (mention) =>
      mention.userId === viewerId && mention.cause.case === "direct",
  );
  if (!directlyMentioned) return undefined;
  return {
    roomId: message.roomId,
    sourceEventId: event.id,
    threadRootEventId: message.inThread || event.id,
  };
}

async function replyToDirectMention(
  api: BotAPI,
  event: RealtimeEvent,
): Promise<void> {
  const target = mentionReplyTarget(event, api.viewerId);
  if (!target) return;
  const replyEventId = await api.replyToMention(target);
  log({
    status: "mention_replied",
    source_event_id: target.sourceEventId,
    thread_root_event_id: target.threadRootEventId,
    reply_event_id: replyEventId,
  });
}

async function commitState(
  stateFile: string,
  state: TestBotState,
  resumeCursor?: string,
): Promise<void> {
  if (resumeCursor) state.resumeCursor = resumeCursor;
  await saveTestBotState(stateFile, state);
}

async function runRealtimeSession(
  config: TestBotConfig,
  apiKey: string,
  api: BotAPI,
  state: TestBotState,
  signal: AbortSignal,
): Promise<SessionResult> {
  const resumed = Boolean(state.resumeCursor);
  const socket = new WebSocket(realtimeUrl(config.serverUrl));
  let processing = Promise.resolve();
  let requestedResult: Omit<SessionResult, "caughtUp"> | undefined;
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
                await replyToDirectMention(api, event);
                rememberProcessedEvent(state, event.id);
              } else {
                log({ status: "duplicate_ignored", event_id: event.id });
              }
              await commitState(config.stateFile, state, event.resumeCursor);
              return;
            }
            case "heartbeat":
              if (frame.frame.value.resumeCursor) {
                await commitState(
                  config.stateFile,
                  state,
                  frame.frame.value.resumeCursor,
                );
              }
              return;
            case "caughtUp":
              await commitState(
                config.stateFile,
                state,
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
  let attempt = 0;
  while (!signal.aborted) {
    try {
      const apiKey = await readAPIKey(config.apiKeyFile);
      const api = await connectPublicAPI(config, apiKey);
      const session = await runRealtimeSession(
        config,
        apiKey,
        api,
        state,
        signal,
      );
      if (session.caughtUp) attempt = 0;
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
