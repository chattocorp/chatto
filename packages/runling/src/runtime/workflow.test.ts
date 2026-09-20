import { createWorkflowContext, type WorkflowContext } from "./context.ts";
import { describe, expect, expectTypeOf, test } from "vitest";
import { Type } from "typebox";
import { task, isTask, isSchemaTask } from "./workflow.ts";
import { observeRunlingEvents, type RunlingEvent } from "./events.ts";

describe("task", () => {
  test("wraps a normal variadic function and preserves sync results and this", () => {
    const add = task(function add(this: { base: number }, ctx, ...values: number[]) {
      return this.base + values.reduce((a, b) => a + b, 0);
    });
    expectTypeOf<ReturnType<typeof add>>().toEqualTypeOf<number>();
    expect(add.call({ base: 10 }, createWorkflowContext(), 1, 2, 3)).toBe(16);
    expect(isTask(add)).toBe(true);
    expect(isSchemaTask(add)).toBe(false);
  });

  test("preserves an ordinary async function's promise", async () => {
    const promise = Promise.resolve("done" as const);
    const run = task((ctx) => promise);
    expect(run(createWorkflowContext())).toBe(promise);
    expectTypeOf<ReturnType<typeof run>>().toEqualTypeOf<Promise<"done">>();
    await expect(run(createWorkflowContext())).resolves.toBe("done");
  });

  test("schema input is explicit and invalid input does not start an activity", () => {
    const events: RunlingEvent[] = [];
    const echo = task({ name: "Echo", input: Type.String(), output: Type.String() }, (ctx, input) => input);
    expectTypeOf<Parameters<typeof echo>>().toEqualTypeOf<[WorkflowContext, string]>();
    observeRunlingEvents(e => events.push(e), () => {
      // @ts-expect-error Input is required.
      expect(() => echo(createWorkflowContext())).toThrow('Task "Echo" input is invalid');
      expect(events).toEqual([]);
      expect(echo(createWorkflowContext(), "")).toBe("");
    });
    expect(isSchemaTask(echo)).toBe(true);
  });

  test("nested tasks preserve inputs and activity parentage", async () => {
    const events: RunlingEvent[] = [];
    const child = task(function child(ctx, value: string) { return `${value} child`; });
    const parent = task(async function parent(ctx, value: string) { return child(ctx, value); });
    await observeRunlingEvents(e => events.push(e), async () => {
      expect(await parent(createWorkflowContext(), "hello")).toBe("hello child");
    });
    const starts = events.filter(e => e.type === "step.started");
    expect(starts).toHaveLength(2);
    expect(starts[1]?.activityId).toBe(starts[0]?.id);
  });

  test.each(["input", "output"] as const)("rejects invalid %s schemas", boundary => {
    expect(() => task({ name: "Invalid", input: Type.String(), output: Type.String(), [boundary]: { type: "invalid" } } as never, (ctx) => "unused")).toThrow("schema is invalid");
  });

  test("validates sync and async outputs", async () => {
    const definition = { name: "Broken", input: Type.String(), output: Type.String() };
    const sync = task(definition, (ctx, _input) => 42);
    const asyncTask = task(definition, async (ctx, _input) => 42);
    expect(() => sync(createWorkflowContext(), "input")).toThrow('Task "Broken" output is invalid');
    await expect(asyncTask(createWorkflowContext(), "input")).rejects.toThrow('Task "Broken" output is invalid');
  });

  test("preserves thrown errors and records failure", async () => {
    const error = new Error("failed");
    const events: RunlingEvent[] = [];
    await observeRunlingEvents(e => events.push(e), async () => {
      expect(() => task((ctx) => { throw error; })(createWorkflowContext())).toThrow(error);
      await expect(task(async (ctx) => { throw error; })(createWorkflowContext())).rejects.toBe(error);
    });
    expect(events.filter(e => e.type === "step.finished").map(e => e.status)).toEqual(["failed", "failed"]);
  });

  test("preserves null input", () => {
    const run = task({ name: "Null", input: Type.Null(), output: Type.Null() }, (ctx, value) => value);
    expect(run(createWorkflowContext(), null)).toBeNull();
  });
});

test("passes the exact supplied context through parallel nested tasks", async () => {
  const ctx = createWorkflowContext();
  const seen: WorkflowContext[] = [];
  const child = task(async (context, value: number) => {
    await Promise.resolve();
    seen.push(context);
    context.recordUsage({ input: value, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0 });
    return value;
  });
  const parent = task((context) => Promise.all([child(context, 1), child(context, 2)]));
  expect(await parent(ctx)).toEqual([1, 2]);
  expect(seen).toEqual([ctx, ctx]);
  expect(seen.every(context => context === ctx)).toBe(true);
  expect(ctx.usage.input).toBe(3);
});

test("requires context even when the callback ignores it", () => {
  const run = task(() => "done");
  expectTypeOf<Parameters<typeof run>>().toEqualTypeOf<[WorkflowContext]>();
  expect(run(createWorkflowContext())).toBe("done");
  if (false) {
    // @ts-expect-error Context is required at the call boundary.
    run();
  }
});

test("preserves optional task arguments", () => {
  const run = task((ctx, value = 2) => value);
  expectTypeOf<Parameters<typeof run>>().toEqualTypeOf<[WorkflowContext, number?]>();
  expect(run(createWorkflowContext())).toBe(2);
});

test("preserves generic task signatures", () => {
  const identity = task(<Value>(ctx: WorkflowContext, value: Value): Value => value);
  const result = identity(createWorkflowContext(), { message: "hello" });
  expectTypeOf(result).toEqualTypeOf<{ message: string }>();
  expect(result).toEqual({ message: "hello" });
});

test("rejects new tasks on an aborted context without starting an activity", () => {
  const ctx = createWorkflowContext();
  const events: RunlingEvent[] = [];
  let called = false;
  const run = task(() => { called = true; });
  try { ctx.abort("Stop"); } catch {}
  observeRunlingEvents(event => events.push(event), () => {
    expect(() => run(ctx)).toThrow(ctx.signal.reason);
  });
  expect(called).toBe(false);
  expect(events).toEqual([]);
});
