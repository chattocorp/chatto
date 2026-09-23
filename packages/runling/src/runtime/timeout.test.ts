import { afterEach, expect, test, vi } from "vitest";
import { createWorkflowContext } from "./context.ts";
import { input } from "./input.ts";
import { observeRunlingEvents, type RunlingEvent } from "./events.ts";
import { runWorkflow } from "./runner.ts";
import { TimeoutError } from "./timeout.ts";

// Fake clocks make answer/deadline ordering explicit without real sleeps.
afterEach(() => vi.useRealTimers());

test("parallel questions have independent deadlines and retain one completion each", async () => {
  vi.useFakeTimers();
  const ctx = createWorkflowContext();
  const answers = new Map<string, ReturnType<typeof Promise.withResolvers<string>>>();
  ctx.onInput = ({ message }) => {
    const answer = Promise.withResolvers<string>();
    answers.set(message, answer);
    return answer.promise;
  };
  const events: RunlingEvent[] = [];
  await observeRunlingEvents(event => events.push(event), async () => {
    const first = input(ctx, "First", { timeout: 0.1 });
    const failed = expect(first).rejects.toBeInstanceOf(TimeoutError);
    const second = input(ctx, "Second", { timeout: 1 });
    await vi.advanceTimersByTimeAsync(100);
    await failed;
    expect(ctx.signal.aborted).toBe(false);
    answers.get("Second")!.resolve("yes");
    expect(await second).toBe("yes");
    answers.get("First")!.reject(new Error("late failure"));
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(events.filter(e => e.type === "input.finished")).toMatchObject([
    { status: "failed", reason: "timeout" }, { status: "answered", value: "yes" },
  ]);
  expect(vi.getTimerCount()).toBe(0);
});

test("an answer before the deadline disposes the timer and leaves the handler signal usable", async () => {
  vi.useFakeTimers();
  const ctx = createWorkflowContext();
  let signal: AbortSignal | undefined;
  ctx.onInput = async request => { signal = request.signal; return "answer"; };
  expect(await input(ctx, "Question", { timeout: 1 })).toBe("answer");
  expect(vi.getTimerCount()).toBe(0);
  await vi.advanceTimersByTimeAsync(2000);
  expect(signal?.aborted).toBe(false);
});

test("cancellation wins before the question deadline", async () => {
  vi.useFakeTimers();
  const ctx = createWorkflowContext();
  ctx.onInput = () => new Promise(() => {});
  const cancel = new AbortController();
  const events: RunlingEvent[] = [];
  await observeRunlingEvents(e => events.push(e), async () => {
    const result = input(ctx, "Question", { timeout: 1, signal: cancel.signal });
    const rejected = expect(result).rejects.toThrow("cancelled by host");
    cancel.abort(new Error("cancelled by host"));
    await rejected;
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(events.filter(e => e.type === "input.finished")).toMatchObject([{ status: "failed", reason: "cancelled" }]);
  expect(vi.getTimerCount()).toBe(0);
});

test("workflow timeout includes input waits and preserves usage and its first abort reason", async () => {
  vi.useFakeTimers();
  let ctx!: ReturnType<typeof createWorkflowContext>;
  const events: RunlingEvent[] = [];
  const run = runWorkflow(async context => {
    ctx = context;
    ctx.recordUsage({ input: 5, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.1 });
    try { await input(ctx, "Question", { timeout: 100 }); }
    finally { expect(() => ctx.abort("later reason")).toThrow(TimeoutError); }
  }, { input: undefined, timeout: 2, onInput: () => new Promise(() => {}), onEvent: e => events.push(e) });
  await vi.advanceTimersByTimeAsync(2000);
  expect(await run).toMatchObject({ ok: false, error: "Workflow timed out after 2 seconds", usage: { input: 5, cost: 0.1 } });
  expect(ctx.signal.reason).toBeInstanceOf(TimeoutError);
  expect(events.filter(e => e.type === "input.finished")).toMatchObject([{ reason: "timeout" }]);
  expect(vi.getTimerCount()).toBe(0);
});

test("input timeout is catchable without failing the workflow", async () => {
  vi.useFakeTimers();
  const run = runWorkflow(async ctx => {
    try { return await input(ctx, "Question", { timeout: 1 }); }
    catch (error) { if (error instanceof TimeoutError) return "default"; throw error; }
  }, { input: undefined, timeout: 10, onInput: () => new Promise(() => {}) });
  await vi.advanceTimersByTimeAsync(1000);
  expect(await run).toMatchObject({ ok: true, output: "default" });
  expect(vi.getTimerCount()).toBe(0);
});

test.each(["success", "failure"])("workflow %s disposes its deadline", async kind => {
  vi.useFakeTimers();
  let ctx!: ReturnType<typeof createWorkflowContext>;
  await runWorkflow(context => { ctx = context; if (kind === "failure") throw new Error("failed"); return 1; }, { input: undefined, timeout: 1 });
  expect(vi.getTimerCount()).toBe(0);
  await vi.advanceTimersByTimeAsync(2000);
  expect(ctx.signal.aborted).toBe(false);
});

test("concurrent workflow deadlines are isolated", async () => {
  vi.useFakeTimers();
  const slow = runWorkflow(ctx => input(ctx, "Wait"), { input: undefined, timeout: 1, onInput: () => new Promise(() => {}) });
  const answer = Promise.withResolvers<string>();
  const other = runWorkflow(ctx => input(ctx, "Other"), { input: undefined, timeout: 10, onInput: () => answer.promise });
  await vi.advanceTimersByTimeAsync(1000);
  expect((await slow).ok).toBe(false);
  answer.resolve("finished");
  expect(await other).toMatchObject({ ok: true, output: "finished" });
});

test("zero timeout skips user work", async () => {
  const work = vi.fn();
  expect(await runWorkflow(work, { input: undefined, timeout: 0 })).toMatchObject({ ok: false });
  expect(work).not.toHaveBeenCalled();
  const ctx = createWorkflowContext();
  ctx.onInput = work;
  await expect(input(ctx, "Question", { timeout: 0 })).rejects.toBeInstanceOf(TimeoutError);
  expect(work).not.toHaveBeenCalled();
});

test.each([-1, NaN, Infinity, 2_147_483.648])("invalid timeout %s rejects before user work", async timeout => {
  const work = vi.fn();
  await expect(runWorkflow(work, { input: undefined, timeout })).rejects.toThrow(RangeError);
  const ctx = createWorkflowContext();
  ctx.onInput = work;
  await expect(input(ctx, "Question", { timeout })).rejects.toThrow(RangeError);
  expect(work).not.toHaveBeenCalled();
});

test("a throwing event listener cannot leave an input timer running", async () => {
  vi.useFakeTimers();
  const ctx = createWorkflowContext();
  ctx.onInput = vi.fn(async () => "unused");
  await expect(observeRunlingEvents(() => { throw new Error("listener failed"); }, () =>
    input(ctx, "Question", { timeout: 60 }),
  )).rejects.toThrow("listener failed");
  expect(ctx.onInput).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
});
