import { afterEach, expect, test, vi } from "vitest";
import { createWorkflowContext } from "runling";
import { createChattoTyping, withChattoTyping } from "./typing.ts";
const destination = { roomId: "dm", threadRootId: "thread" };
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

test("sends thread-scoped typing through ConnectRPC", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response('{}'));
  vi.stubGlobal("fetch", fetcher);
  await createChattoTyping("https://chat.example", "key")(destination, new AbortController().signal);
  expect(String(fetcher.mock.calls[0]![0])).toBe("https://chat.example/api/connect/chatto.api.v1.RoomService/UpdateTypingIndicator");
  expect(JSON.parse(fetcher.mock.calls[0]![1].body)).toEqual({ roomId: "dm", threadRootEventId: "thread" });
});

test("refreshes during work and stops when work finishes", async () => {
  vi.useFakeTimers();
  const work = Promise.withResolvers<string>();
  const typing = vi.fn(async () => {});
  const run = withChattoTyping(createWorkflowContext(), destination, typing, () => work.promise);
  await vi.advanceTimersByTimeAsync(6000);
  expect(typing).toHaveBeenCalledTimes(3);
  work.resolve("done");
  expect(await run).toBe("done");
  await vi.advanceTimersByTimeAsync(9000);
  expect(typing).toHaveBeenCalledTimes(3);
  expect(vi.getTimerCount()).toBe(0);
});

test("typing failures do not block work, and an in-flight update is aborted on completion", async () => {
  vi.useFakeTimers();
  let signal!: AbortSignal;
  const pending = Promise.withResolvers<void>();
  const typing = vi.fn(async (_destination, value: AbortSignal) => { signal = value; return pending.promise; });
  expect(await withChattoTyping(createWorkflowContext(), destination, typing, async () => "done")).toBe("done");
  expect(signal.aborted).toBe(true);
  pending.reject(new Error("offline"));
  await vi.advanceTimersByTimeAsync(10000);
  expect(typing).toHaveBeenCalledOnce();
});

test("stops after a failed turn", async () => {
  vi.useFakeTimers();
  const typing = vi.fn(async () => { throw new Error("no typing permission"); });
  await expect(withChattoTyping(createWorkflowContext(), destination, typing, async () => { throw new Error("turn failed"); })).rejects.toThrow("turn failed");
  await vi.advanceTimersByTimeAsync(10000);
  expect(typing).toHaveBeenCalledOnce();
});
