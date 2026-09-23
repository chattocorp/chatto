import { afterEach, expect, test, vi } from "vitest";
import { RealtimeServerFrame, RealtimeSubscribe, RealtimeCloseCode, RealtimeRecovery } from "@chatto/api-types/realtime/v1/realtime_pb";
import { createChattoClient, type RealtimeCheckpoint, type RealtimeStatus } from "./index.js";

class Socket extends EventTarget {
  binaryType = "";
  sent: Uint8Array[] = [];
  close = vi.fn();
  send(value: Uint8Array) { this.sent.push(value); }
  open() { this.dispatchEvent(new Event("open")); }
  frame(frame: RealtimeServerFrame) {
    this.dispatchEvent(new MessageEvent("message", { data: new Uint8Array(frame.toBinary()).buffer }));
  }
  disconnect() { this.dispatchEvent(new Event("close")); }
}

function fixture(onEvent = vi.fn(async () => {}), maxPendingFrames?: number, cursor?: string) {
  vi.useFakeTimers();
  const sockets: Socket[] = [];
  const urls: string[] = [];
  const controller = new AbortController();
  const checkpoint: RealtimeCheckpoint = { cursor };
  const statuses: RealtimeStatus[] = [];
  const client = createChattoClient({ serverUrl: "https://chat.example/path?secret=no", apiKey: "private-token",
    webSocket(url) { urls.push(url); const socket = new Socket(); sockets.push(socket); return socket as unknown as WebSocket; },
  });
  const result = client.consumeRealtime({ signal: controller.signal, checkpoint, onEvent,
    onStatus: value => statuses.push(value), maxPendingFrames });
  // Observe terminal failures immediately, including tests that inspect them later.
  void result.catch(() => {});
  return { sockets, urls, controller, checkpoint, statuses, result };
}

const event = (id: string) => new RealtimeServerFrame({ frame: { case: "event", value: {
  id, cursor: id, event: { case: "messagePosted", value: { bodyPlaintext: "hello" } },
} } });
const caughtUp = (cursor: string, recovery = RealtimeRecovery.RESUMED) => new RealtimeServerFrame({
  frame: { case: "caughtUp", value: { cursor, recovery } },
});
afterEach(() => vi.useRealTimers());

test("authenticates in binary subscription and commits cursors only after ordered acceptance", async () => {
  let release!: () => void;
  const handler = vi.fn(() => new Promise<void>(resolve => { release = resolve; }));
  const f = fixture(handler);
  f.sockets[0]!.open();
  expect(f.urls).toEqual(["wss://chat.example/api/realtime"]);
  expect(RealtimeSubscribe.fromBinary(f.sockets[0]!.sent[0]!)).toMatchObject({ protocolVersion: 4, bearerToken: "private-token", initialState: 1 });
  f.sockets[0]!.frame(event("first"));
  f.sockets[0]!.frame(new RealtimeServerFrame({ frame: { case: "heartbeat", value: { cursor: "after" } } }));
  await vi.advanceTimersByTimeAsync(0);
  expect(f.checkpoint.cursor).toBeUndefined();
  release();
  await vi.advanceTimersByTimeAsync(0);
  expect(f.checkpoint.cursor).toBe("after");
  f.controller.abort();
  await f.result;
  expect(vi.getTimerCount()).toBe(0);
});

test("resumes after a disconnect and reports replay gaps", async () => {
  const f = fixture();
  f.sockets[0]!.frame(caughtUp("initial", RealtimeRecovery.LIVE_ONLY));
  await vi.advanceTimersByTimeAsync(0);
  expect(f.statuses).toContainEqual({ state: "ready", recovery: "live-only", gap: false });
  f.sockets[0]!.disconnect();
  await vi.advanceTimersByTimeAsync(1000);
  f.sockets[1]!.open();
  expect(RealtimeSubscribe.fromBinary(f.sockets[1]!.sent[0]!).resumeCursor).toBe("initial");
  f.sockets[1]!.frame(caughtUp("new", RealtimeRecovery.LIVE_ONLY));
  await vi.advanceTimersByTimeAsync(0);
  expect(f.statuses).toContainEqual({ state: "ready", recovery: "live-only", gap: true });
  f.controller.abort();
  await f.result;
});

test("honors server delay even when the socket closes immediately after its close frame", async () => {
  const f = fixture();
  f.sockets[0]!.frame(new RealtimeServerFrame({ frame: { case: "close", value: {
    code: RealtimeCloseCode.TEMPORARILY_UNAVAILABLE, reconnect: true, retryAfter: { seconds: 2n },
  } } }));
  f.sockets[0]!.disconnect();
  await vi.advanceTimersByTimeAsync(1999);
  expect(f.sockets).toHaveLength(1);
  await vi.advanceTimersByTimeAsync(1);
  expect(f.sockets).toHaveLength(2);
  f.controller.abort(); await f.result;
});

test.each([RealtimeCloseCode.AUTHENTICATION_REQUIRED, RealtimeCloseCode.UNSUPPORTED_PROTOCOL])("stops on terminal close %s without leaking the remote message", async code => {
  const f = fixture();
  f.sockets[0]!.frame(new RealtimeServerFrame({ frame: { case: "close", value: {
    code, reconnect: true, message: "private-token",
  } } }));
  await expect(f.result).rejects.toThrow("Realtime subscription was rejected or ended");
  expect(f.sockets).toHaveLength(1);
  expect(vi.getTimerCount()).toBe(0);
});

test.each([new Uint8Array([255]), new Uint8Array([0x9a, 0x06, 0])])("does not cross undecodable or unknown top-level frames", async bytes => {
  const f = fixture();
  f.checkpoint.cursor = "safe";
  f.sockets[0]!.dispatchEvent(new MessageEvent("message", { data: bytes.buffer }));
  f.sockets[0]!.frame(caughtUp("unsafe"));
  await expect(f.result).rejects.toThrow();
  expect(f.checkpoint.cursor).toBe("safe");
});

test("skips unknown semantic events using their common cursor metadata", async () => {
  const handler = vi.fn(async () => {});
  const f = fixture(handler);
  f.sockets[0]!.frame(new RealtimeServerFrame({ frame: { case: "event", value: { id: "future", cursor: "future-cursor" } } }));
  await vi.advanceTimersByTimeAsync(0);
  expect(handler).not.toHaveBeenCalled();
  expect(f.checkpoint.cursor).toBe("future-cursor");
  f.controller.abort(); await f.result;
});

test("overflow reconnects after the current handler settles without accepting queued frames", async () => {
  let release!: () => void;
  const f = fixture(vi.fn(() => new Promise<void>(resolve => { release = resolve; })), 1);
  f.sockets[0]!.frame(event("accepted"));
  await vi.advanceTimersByTimeAsync(0);
  f.sockets[0]!.frame(event("queued"));
  f.sockets[0]!.frame(event("overflow"));
  await vi.advanceTimersByTimeAsync(1000);
  expect(f.sockets).toHaveLength(1);
  release();
  await vi.advanceTimersByTimeAsync(1000);
  expect(f.checkpoint.cursor).toBe("accepted");
  f.sockets[1]!.open();
  expect(RealtimeSubscribe.fromBinary(f.sockets[1]!.sent[0]!).resumeCursor).toBe("accepted");
  f.controller.abort(); await f.result;
});

test("handler failure retains the preceding checkpoint and sanitizes the error", async () => {
  const f = fixture(vi.fn(async () => { throw new Error("private message"); }));
  f.checkpoint.cursor = "before";
  f.sockets[0]!.frame(event("failed"));
  await expect(f.result).rejects.toThrow("Realtime event handler failed");
  expect(f.checkpoint.cursor).toBe("before");
});

test("resync before the first caught-up frame still reports a gap", async () => {
  const f = fixture(undefined, undefined, "previous-session");
  f.sockets[0]!.frame(new RealtimeServerFrame({ frame: { case: "close", value: {
    code: RealtimeCloseCode.RESYNC_REQUIRED, reconnect: true,
  } } }));
  await vi.advanceTimersByTimeAsync(1000);
  f.sockets[1]!.open();
  expect(RealtimeSubscribe.fromBinary(f.sockets[1]!.sent[0]!).resumeCursor).toBeUndefined();
  f.sockets[1]!.frame(caughtUp("live", RealtimeRecovery.LIVE_ONLY));
  await vi.advanceTimersByTimeAsync(0);
  expect(f.statuses).toContainEqual({ state: "ready", recovery: "live-only", gap: true });
  f.controller.abort(); await f.result;
});

test("cancellation interrupts reconnect backoff and closes all timers", async () => {
  const f = fixture();
  f.sockets[0]!.disconnect();
  await vi.advanceTimersByTimeAsync(0);
  expect(f.statuses).toContainEqual({ state: "reconnecting" });
  f.controller.abort(); await f.result;
  expect(vi.getTimerCount()).toBe(0);
  expect(f.sockets).toHaveLength(1);
});
