import { afterEach, expect, test, vi } from "vitest";
import { createWorkflowContext } from "runling";
import { runConversationTask } from "./conversation-task.ts";

const destination = { roomId: "dm", threadRootId: "thread" };

function inbox() {
  const listeners = new Set<() => void>();
  return {
    listeners,
    drain: () => [] as string[],
    prepend: (_messages: string[]) => {},
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
  };
}

afterEach(() => vi.useRealTimers());

test("the bridge pauses typing while idle, resumes while busy, and stops on completion", async () => {
  vi.useFakeTimers();
  const source = inbox();
  const done = Promise.withResolvers<string>();
  const typing = vi.fn(async () => {});
  let onBusy!: (busy: boolean) => void;
  const run = runConversationTask(createWorkflowContext(), source, async (_ctx, update) => {
    onBusy = update;
    return done.promise;
  }, { destination, typing, post: async () => {} });

  await vi.advanceTimersByTimeAsync(0);
  expect(typing).toHaveBeenCalledOnce();
  onBusy(false);
  await vi.advanceTimersByTimeAsync(6000);
  expect(typing).toHaveBeenCalledOnce();
  onBusy(true);
  await vi.advanceTimersByTimeAsync(3000);
  expect(typing).toHaveBeenCalledTimes(2);

  done.resolve("finished");
  expect(await run).toBe("finished");
  await vi.advanceTimersByTimeAsync(6000);
  expect(typing).toHaveBeenCalledTimes(2);
  expect(source.listeners.size).toBe(0);
});

test("a failed post cancels the child and releases routing and typing", async () => {
  vi.useFakeTimers();
  const source = inbox();
  const typing = vi.fn(async () => {});
  let signal!: AbortSignal;
  const run = runConversationTask(createWorkflowContext(), source, async ctx => {
    signal = ctx.signal;
    await ctx.emit("Hello");
    return new Promise<void>(resolve => {
      ctx.signal.addEventListener("abort", () => resolve(), { once: true });
    });
  }, {
    destination,
    typing,
    post: async () => { throw new Error("Chatto unavailable"); },
  });

  await expect(run).rejects.toThrow("Chatto unavailable");
  expect(signal.aborted).toBe(true);
  expect(source.listeners.size).toBe(0);
  const calls = typing.mock.calls.length;
  await vi.advanceTimersByTimeAsync(6000);
  expect(typing).toHaveBeenCalledTimes(calls);
});
