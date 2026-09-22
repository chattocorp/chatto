import { expect, test, vi } from "vitest";
import { SourceManager } from "./source-manager.ts";
import { defineWebConfig, startWorkflow, type EventSourceContext } from "./web-config.ts";

const untilAbort = (signal: AbortSignal) => new Promise<void>(resolve => {
  if (signal.aborted) resolve();
  else signal.addEventListener("abort", () => resolve(), { once: true });
});

test("starts eagerly, waits for cleanup, and retains source state across generations", async () => {
  const calls: string[] = [];
  const manager = new SourceManager(() => async () => ({ id: "run" }));
  let state: Map<string, unknown>;
  let release!: () => void;
  const cleanup = new Promise<void>(resolve => { release = resolve; });
  await manager.replace(defineWebConfig({ sources: { bot: async ctx => {
    state = ctx.state; ctx.state.set("conversation", "active"); calls.push("first");
    await untilAbort(ctx.signal); await cleanup; calls.push("closed");
  } } }));
  const replacement = manager.replace(defineWebConfig({ sources: { bot: async ctx => {
    expect(ctx.state).toBe(state); expect(ctx.state.get("conversation")).toBe("active");
    calls.push("second"); await untilAbort(ctx.signal);
  } } }));
  await Promise.resolve();
  expect(calls).toEqual(["first"]);
  release(); await replacement;
  expect(calls).toEqual(["first", "closed", "second"]);
  await manager.close();
});

test("dispatch waits for registration, expires the router context, and rejects after shutdown", async () => {
  const register = vi.fn(async () => ({ id: "registered" }));
  const manager = new SourceManager(() => register);
  let context!: EventSourceContext;
  let lateStart!: () => Promise<unknown>;
  await manager.replace(defineWebConfig({ sources: { bot: async ctx => {
    context = ctx; await untilAbort(ctx.signal);
  } } }));
  const runs = await context.dispatch(ctx => {
    lateStart = () => ctx.start(() => {}, { input: undefined });
    void ctx.start(() => {}, { input: undefined });
  }, undefined);
  expect(runs).toEqual([{ id: "registered" }]);
  await expect(lateStart()).rejects.toThrow("routing has finished");
  await manager.close();
  await expect(context.dispatch(startWorkflow(() => {}), undefined)).rejects.toThrow("source has stopped");
  expect(register).toHaveBeenCalledOnce();
});

test("source failure is isolated and does not restart until replacement", async () => {
  const log = vi.spyOn(console, "error").mockImplementation(() => {});
  const source = vi.fn(async () => { throw new Error("private content"); });
  const manager = new SourceManager(() => async () => ({ id: "run" }));
  try {
    await manager.replace(defineWebConfig({ sources: { broken: source } }));
    await vi.waitFor(() => expect(log).toHaveBeenCalled());
    expect(JSON.stringify(log.mock.calls)).not.toContain("private content");
    expect(source).toHaveBeenCalledOnce();
    await manager.replace(defineWebConfig({ sources: { broken: source } }));
    expect(source).toHaveBeenCalledTimes(2);
  } finally { await manager.close(); log.mockRestore(); }
});

test("removing a source discards its state before the name is reused", async () => {
  const states: Map<string, unknown>[] = [];
  const source = async (ctx: EventSourceContext) => { states.push(ctx.state); await untilAbort(ctx.signal); };
  const manager = new SourceManager(() => async () => ({ id: "run" }));
  await manager.replace(defineWebConfig({ sources: { bot: source } }));
  await manager.replace(defineWebConfig({}));
  await manager.replace(defineWebConfig({ sources: { bot: source } }));
  expect(states).toHaveLength(2);
  expect(states[1]).not.toBe(states[0]);
  await manager.close();
});
