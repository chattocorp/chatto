import { afterEach, expect, test, vi } from "vitest";
import { RealtimeEvent, RoomKind, type ConsumeRealtimeOptions } from "@chatto/client";
import type { EventSourceContext } from "runling/web";
import { chattoSource } from "./realtime.ts";
import { RegistrationError } from "./routing.ts";
import type { createChattoBot } from "../workflows/chat.ts";

const mocks = vi.hoisted(() => ({
  rpc: vi.fn(), consumeRealtime: vi.fn(), request: vi.fn(),
  bot: vi.fn((_options: Parameters<typeof createChattoBot>[0]) => ({ route: () => {} })),
}));
vi.mock("@chatto/client", async importOriginal => {
  const actual = await importOriginal<typeof import("@chatto/client")>();
  return {
    ...actual,
    createChattoClient: (options: Parameters<typeof actual.createChattoClient>[0]) => ({
      ...actual.createChattoClient({ ...options, fetch: async (url, init) => {
        const method = String(url).split("chatto.api.v1.")[1];
        if (method === "ViewerService/GetViewer" || method === "MessageService/GetMessage") {
          return Response.json(await mocks.rpc(method, JSON.parse(init!.body as string), init!.signal));
        }
        return mocks.request(url, init);
      } }),
      consumeRealtime: mocks.consumeRealtime,
    }),
  };
});
vi.mock("../workflows/chat.ts", () => ({ createChattoBot: mocks.bot }));
afterEach(() => { vi.unstubAllEnvs(); vi.clearAllMocks(); });

test.each(["bot", "human", "wrong-thread", "missing", "unavailable"])("verifies unmentioned replies against %s targets", async kind => {
  vi.stubEnv("CHATTO_URL", "https://chat.example");
  vi.stubEnv("CHATTO_API_KEY", "key");
  vi.stubEnv("CHATTO_ALLOWED_USER_ID", "allowed");
  const warning = vi.spyOn(console, "warn").mockImplementation(() => {});
  mocks.rpc.mockImplementation(async (method: string) => {
    if (method === "ViewerService/GetViewer") return { user: { profile: { id: "bot" } } };
    if (kind === "unavailable") throw new Error("private error");
    return { message: kind === "missing" ? undefined : {
      id: "reply-target", actorId: kind === "human" ? "human" : "bot", roomId: "room",
      threadRootEventId: kind === "wrong-thread" ? "other" : "root",
    } };
  });
  mocks.consumeRealtime.mockImplementation(async (options: ConsumeRealtimeOptions) => {
    for (const actorId of ["other", "bot", "allowed"]) await options.onEvent(new RealtimeEvent({
      id: actorId, actorId, event: { case: "messagePosted", value: {
        roomId: "room", roomKind: RoomKind.CHANNEL, threadRootEventId: "root",
        inReplyTo: "reply-target", bodyPlaintext: "Follow-up",
      } },
    }));
  });
  const dispatch = vi.fn().mockResolvedValue([]);
  try {
    await chattoSource({ signal: new AbortController().signal, state: new Map(), dispatch });
    expect(mocks.rpc).toHaveBeenCalledTimes(2); // Filter other senders before lookup.
    expect(dispatch).toHaveBeenCalledTimes(kind === "bot" ? 1 : 0);
    if (kind === "bot") expect(dispatch.mock.calls[0]![1].triggers).toEqual(["reply"]);
    expect(JSON.stringify(warning.mock.calls)).not.toContain("private error");
  } finally { warning.mockRestore(); }
});

test.each([undefined, "", "  allowed-user  "])("filters senders before routing with allowed user %s", async configured => {
  vi.stubEnv("CHATTO_URL", "https://chat.example");
  vi.stubEnv("CHATTO_API_KEY", "key");
  vi.stubEnv("CHATTO_ALLOWED_USER_ID", configured);
  mocks.rpc.mockResolvedValue({ user: { profile: { id: "bot" } } });
  const dispatch = vi.fn().mockResolvedValue([]);
  mocks.consumeRealtime.mockImplementation(async (options: ConsumeRealtimeOptions) => {
    for (const actorId of ["allowed-user", "other-user"]) {
      for (const roomKind of [RoomKind.DM, RoomKind.CHANNEL]) {
        for (const bodyPlaintext of ["hello", "follow-up", "/cancel"]) {
          await options.onEvent(new RealtimeEvent({ id: `${actorId}-${roomKind}-${bodyPlaintext}`, actorId,
            event: { case: "messagePosted", value: {
              roomId: "room", roomKind, bodyPlaintext,
              threadRootEventId: bodyPlaintext === "hello" ? "" : "existing-thread",
              mentions: [{ includesViewer: true }],
            } },
          }));
        }
      }
    }
  });
  await chattoSource({ signal: new AbortController().signal, state: new Map(), dispatch });
  expect(dispatch).toHaveBeenCalledTimes(configured ? 6 : 12);
  if (configured) {
    for (const [, delivery] of dispatch.mock.calls) expect(delivery.message.author_id).toBe("allowed-user");
  }
});

test("retains conversations across reloads, resets cursor for a new key, and isolates a new identity", async () => {
  vi.stubEnv("CHATTO_URL", "https://chat.example");
  vi.stubEnv("CHATTO_API_KEY", "first-key");
  mocks.rpc.mockResolvedValue({ user: { profile: { id: "bot" } } });
  const checkpoints: unknown[] = [];
  mocks.consumeRealtime.mockImplementation(async (options: ConsumeRealtimeOptions) => {
    checkpoints.push(options.checkpoint);
    options.checkpoint!.cursor = "accepted";
  });
  const ctx: EventSourceContext = { signal: new AbortController().signal, state: new Map(), dispatch: vi.fn() };
  await chattoSource(ctx);
  await chattoSource(ctx);
  expect(checkpoints[1]).toBe(checkpoints[0]);
  expect(mocks.bot).toHaveBeenNthCalledWith(2, expect.objectContaining({ state: expect.any(Object) }));
  const firstState = mocks.bot.mock.calls[0]![0]!.state;
  expect(mocks.bot.mock.calls[1]![0]!.state).toBe(firstState);

  vi.stubEnv("CHATTO_API_KEY", "second-key");
  await chattoSource(ctx);
  expect(checkpoints[2]).not.toBe(checkpoints[1]);
  expect(mocks.bot.mock.calls[2]![0]!.state).toBe(firstState);

  mocks.rpc.mockResolvedValue({ user: { profile: { id: "different-bot" } } });
  await chattoSource(ctx);
  expect(checkpoints[3]).not.toBe(checkpoints[2]);
  expect(mocks.bot.mock.calls[3]![0]!.state).not.toBe(firstState);
});

test("passes implementation configuration through the realtime source and captures it per generation", async () => {
  vi.stubEnv("CHATTO_URL", "https://chat.example");
  vi.stubEnv("CHATTO_API_KEY", "key");
  vi.stubEnv("CHATTO_SOURCE_DIRECTORY", "/configured/chatto");
  vi.stubEnv("CHATTO_IMPLEMENTATION_REPOSITORY", "example/chatto");
  vi.stubEnv("CHATTO_SOURCE_REF", "main");
  vi.stubEnv("CHATTO_IMPLEMENTATION_MODEL", "test/worker");
  mocks.rpc.mockResolvedValue({ user: { profile: { id: "bot" } } });
  mocks.consumeRealtime.mockResolvedValue(undefined);
  const ctx: EventSourceContext = { signal: new AbortController().signal, state: new Map(), dispatch: vi.fn() };
  await chattoSource(ctx);
  const original = mocks.bot.mock.calls[0]![0]!;
  expect(original.implementation).toEqual({ directory: "/configured/chatto", repository: "example/chatto", baseBranch: "main", model: "test/worker" });
  vi.stubEnv("CHATTO_SOURCE_REF", "next");
  await chattoSource(ctx);
  expect(mocks.bot.mock.calls[1]![0]!.implementation?.baseBranch).toBe("next");
  expect(original.implementation?.baseBranch).toBe("main");
  vi.stubEnv("CHATTO_IMPLEMENTATION_REPOSITORY", "");
  await chattoSource(ctx);
  expect(mocks.bot.mock.calls[2]![0]!.implementation).toBeUndefined();
});

test("existing conversation callbacks keep their server and credentials after reload", async () => {
  vi.stubEnv("CHATTO_URL", "https://original.example");
  vi.stubEnv("CHATTO_API_KEY", "original-key");
  mocks.rpc.mockResolvedValue({ user: { profile: { id: "bot" } } });
  mocks.consumeRealtime.mockResolvedValue(undefined);
  mocks.request.mockImplementation(async () => new Response(JSON.stringify({ page: { events: [] } })));
  const ctx: EventSourceContext = { signal: new AbortController().signal, state: new Map(), dispatch: vi.fn() };
  await chattoSource(ctx);
  const original = mocks.bot.mock.calls[0]![0]!;
  vi.stubEnv("CHATTO_URL", "https://replacement.example");
  vi.stubEnv("CHATTO_API_KEY", "replacement-key");
  await chattoSource(ctx);
  const destination = { roomId: "room", threadRootId: "root" };
  const delivery = { version: 1 as const, id: "message", type: "message.created" as const,
    triggers: ["direct_message"], occurred_at: "", bot_id: "bot", room_id: "room", thread_root_id: "root",
    message: { id: "message", author_id: "human", body: "hello" } };
  await original.post!(destination, "reply", ctx.signal);
  await original.typing!(destination, ctx.signal);
  await original.readThread!(delivery, ctx.signal);
  await original.acknowledge!(delivery, ctx.signal);
  expect(mocks.request).toHaveBeenCalledTimes(4);
  for (const [url, options] of mocks.request.mock.calls) {
    expect(url.origin).toBe("https://original.example");
    expect(options.headers.Authorization).toBe("Bearer original-key");
  }
});

test.each(["recover", "exhaust", "abort", "other"])("registration retry: %s", async mode => {
  vi.stubEnv("CHATTO_URL", "https://chat.example");
  vi.stubEnv("CHATTO_API_KEY", "key");
  mocks.rpc.mockResolvedValue({ user: { profile: { id: "bot" } } });
  const controller = new AbortController();
  const failure = new Error("Event routing failed", { cause: new RegistrationError(new Error("disk")) });
  const dispatch = vi.fn().mockImplementation(async () => {
    if (mode === "abort") controller.abort();
    if (mode === "other") throw new Error("Other routing failure");
    if (mode === "recover" && dispatch.mock.calls.length === 2) return [];
    throw failure;
  });
  mocks.consumeRealtime.mockImplementation(async (options: ConsumeRealtimeOptions) => {
    await options.onEvent(new RealtimeEvent({ id: "message", actorId: "human", event: {
      case: "messagePosted", value: { roomId: "room", roomKind: RoomKind.DM, bodyPlaintext: "hello" },
    } }));
  });
  const result = chattoSource({ signal: controller.signal, state: new Map(), dispatch });
  if (mode === "recover") await expect(result).resolves.toBeUndefined();
  else await expect(result).rejects.toBeInstanceOf(Error);
  expect(dispatch).toHaveBeenCalledTimes(mode === "recover" ? 2 : mode === "exhaust" ? 3 : 1);
});
