import { z } from "zod";
import { expect, expectTypeOf, test } from "vitest";
import { createWorkflowContext, type WorkflowContext } from "./context.ts";
import { spawn } from "./spawn.ts";
import { ChannelClosedError, ChannelFullError } from "./channel.ts";
import { task } from "./workflow.ts";
import { runWorkflow } from "./runner.ts";
import { Type } from "typebox";

type Command = { add: number };
type Update = { total: number };

const accumulator = task(
  async (ctx: WorkflowContext<Command, Update>, initial: number) => {
    let total = initial;

    for await (const message of ctx.inbox) {
      total += message.add;
      await ctx.emit({ total });
    }

    return total;
  },
);

test("sends data while the task runs, streams updates, then returns its typed result", async () => {
  const handle = spawn(createWorkflowContext(), accumulator, 10);

  expectTypeOf(handle.send).parameter(0).toEqualTypeOf<Command>();
  expectTypeOf(handle.updates).toEqualTypeOf<AsyncIterable<Update>>();
  expectTypeOf(handle.result).toEqualTypeOf<Promise<number>>();

  await handle.send({ add: 2 });
  const updates = handle.updates[Symbol.asyncIterator]();

  expect(await updates.next()).toEqual({ done: false, value: { total: 12 } });

  await handle.send({ add: 3 });
  handle.closeInput();

  expect(await updates.next()).toEqual({ done: false, value: { total: 15 } });
  expect((await updates.next()).done).toBe(true);
  expect(await handle.result).toBe(15);

  await expect(handle.send({ add: 1 })).rejects.toBeInstanceOf(
    ChannelClosedError,
  );
});

test("normal task calls have an empty inbox and no-op output", async () => {
  const ctx = createWorkflowContext();

  expect(await accumulator(ctx, 4)).toBe(4);
  for (let i = 0; i < 100; i++) await ctx.emit(i);
});

test("separate child channels share usage but not cancellation", async () => {
  const parent = createWorkflowContext();
  const child = task(async (ctx: WorkflowContext<Command, Update>) => {
    ctx.recordUsage({ input: 1, output: 0, cacheRead: 0, cacheWrite: 0 });
    return accumulator(ctx, 0);
  });

  const a = spawn(parent, child);
  const b = spawn(parent, child);

  a.cancel(new Error("stop a"));
  await expect(a.result).rejects.toThrow("stop a");
  expect(parent.signal.aborted).toBe(false);

  // Cancelling one child must leave its sibling able to communicate.
  await b.send({ add: 5 });
  b.closeInput();

  expect(await b.result).toBe(5);

  const updates: Update[] = [];
  for await (const value of b.updates) updates.push(value);

  expect(updates).toEqual([{ total: 5 }]);
  expect(parent.usage.input).toBe(2);
});

test("parent cancellation releases readers and rejects the result", async () => {
  const ctx = createWorkflowContext();
  const handle = spawn(ctx, accumulator, 0);
  const next = handle.updates[Symbol.asyncIterator]().next();

  try {
    ctx.abort("parent stopped");
  } catch {
    // abort() both signals cancellation and throws to stop the caller.
  }

  await expect(handle.result).rejects.toThrow("parent stopped");
  await expect(next).rejects.toThrow("parent stopped");
});

test("an already cancelled parent does not start work", async () => {
  const ctx = createWorkflowContext();

  try {
    ctx.abort("stop");
  } catch {
    // abort() both signals cancellation and throws to stop the caller.
  }

  let called = false;
  const handle = spawn(ctx, () => {
    called = true;
  });
  await expect(handle.result).rejects.toThrow("stop");
  expect(called).toBe(false);
});

test.each([false, true])(
  "sync and async failures reject both result and updates: %s",
  async (asynchronous) => {
    const failure = new Error("work failed");
    const handle = spawn(createWorkflowContext(), () => {
      if (asynchronous) return Promise.reject(failure);
      throw failure;
    });
    await expect(handle.result).rejects.toBe(failure);
    await expect(handle.updates[Symbol.asyncIterator]().next()).rejects.toBe(
      failure,
    );
  },
);

test("cancellation settles the handle even if work ignores the signal", async () => {
  let finish!: (value: number) => void;
  const handle = spawn(
    createWorkflowContext(),
    () =>
      new Promise<number>((resolve) => {
        finish = resolve;
      }),
  );
  handle.cancel();
  await expect(handle.result).rejects.toThrow("Task cancelled");
  finish(1);
});

test("unconsumed updates fail at the bound instead of deadlocking or growing forever", async () => {
  const handle = spawn(
    createWorkflowContext(),
    task(async (ctx) => {
      for (let i = 0; i <= 64; i++) await ctx.emit(i);
    }),
  );
  await expect(handle.result).rejects.toBeInstanceOf(ChannelFullError);
});

test("a consumer leaving updates closes output and a later emit fails", async () => {
  let proceed!: () => void;
  const wait = new Promise<void>((resolve) => {
    proceed = resolve;
  });
  const handle = spawn(
    createWorkflowContext(),
    task(async (ctx) => {
      await ctx.emit("first");
      await wait;
      await ctx.emit("second");
    }),
  );
  for await (const _ of handle.updates) break;
  proceed();
  await expect(handle.result).rejects.toBeInstanceOf(ChannelClosedError);
});

test("supports variadic, optional, synchronous, and bound task functions", async () => {
  const work = task(function (this: { base: number }, _ctx, a: number, b = 1) {
    return this.base + a + b;
  });

  expect(
    await spawn(createWorkflowContext(), work.bind({ base: 10 }), 2).result,
  ).toBe(13);
});

test("schema tasks still validate inputs and outputs and retain timeline nesting", async () => {
  const work = task(
    { name: "Child", input: Type.Number(), output: Type.Number() },
    (_ctx, value) => value + 1,
  );
  const events: any[] = [];
  const root = task(async (ctx) => await spawn(ctx, work, 1).result);
  const execution = await runWorkflow(root, {
    input: undefined,
    onEvent: (event) => events.push(event),
  });

  expect(execution.output).toBe(2);
  expect(
    events.filter((e) => e.type === "step.started").map((e) => e.label),
  ).toEqual(["Task", "Child"]);
  const bad = spawn(createWorkflowContext(), work, "bad" as unknown as number);
  await expect(bad.result).rejects.toThrow("input is invalid");
});

// Compile-only checks: channel types and task arguments must not widen to unknown/any.
function typeChecks() {
  const handle = spawn(createWorkflowContext(), accumulator, 0);
  // @ts-expect-error wrong message type
  handle.send({ add: "one" });
  // @ts-expect-error missing required task input
  spawn(createWorkflowContext(), accumulator);
  // @ts-expect-error wrong task input
  spawn(createWorkflowContext(), accumulator, "zero");
  const ctx = {} as WorkflowContext<Command, Update>;
  // @ts-expect-error wrong update type
  ctx.emit("bad");
}

test("cancellation before result settlement cannot report success with failed channels", async () => {
  const handle = spawn(createWorkflowContext(), () => 1);
  handle.cancel(new Error("cancelled before settlement"));
  await expect(handle.result).rejects.toThrow("cancelled before settlement");
  await expect(handle.updates[Symbol.asyncIterator]().next()).rejects.toThrow(
    "cancelled before settlement",
  );
});

test("schema transforms retain parsed return types when spawned", async () => {
  const child = task(
    {
      name: "Parsed",
      input: z.string().transform(Number),
      output: z.number().transform(String),
    },
    (_ctx, value) => value + 1,
  );
  const handle = spawn(createWorkflowContext(), child, "41");

  expectTypeOf(handle.result).toEqualTypeOf<Promise<string>>();
  expect(await handle.result).toBe("42");
});

test("a completed result is unchanged by late cancellation", async () => {
  const handle = spawn(createWorkflowContext(), () => 1);

  expect(await handle.result).toBe(1);
  handle.cancel();

  expect(await handle.result).toBe(1);
  expect((await handle.updates[Symbol.asyncIterator]().next()).done).toBe(true);
});

test("schema tasks preserve channel types alongside parsed input and output", async () => {
  const child = task(
    {
      name: "Parsed channel task",
      input: z.string().transform(Number),
      output: z.number().transform(String),
    },
    async (ctx: WorkflowContext<Command, Update>, initial) => {
      let total = initial;
      for await (const message of ctx.inbox) {
        total += message.add;
        await ctx.emit({ total });
      }
      return total;
    },
  );
  const handle = spawn(createWorkflowContext(), child, "10");

  expectTypeOf(handle.send).parameter(0).toEqualTypeOf<Command>();
  expectTypeOf(handle.updates).toEqualTypeOf<AsyncIterable<Update>>();
  expectTypeOf(handle.result).toEqualTypeOf<Promise<string>>();

  if (false) {
    // @ts-expect-error A schema task still requires its declared message type.
    void handle.send("wrong");
    // @ts-expect-error Callers supply the schema's unparsed input.
    spawn(createWorkflowContext(), child, 10);
  }

  await handle.send({ add: 2 });
  handle.closeInput();

  const updates: Update[] = [];
  for await (const update of handle.updates) updates.push(update);
  expect(updates).toEqual([{ total: 12 }]);
  expect(await handle.result).toBe("12");
});

test("journals message identities, reads, receipts, and task links without changing values", async () => {
  const { observeRunlingEvents } = await import("./events.ts");
  const { connectAgent } = await import("./agents/index.ts");
  const events: import("./events.ts").RunlingEvent[] = [];
  let finish!: () => void;
  const done = new Promise<void>(resolve => { finish = resolve; });
  const child = task(async (ctx: WorkflowContext<string, string>) => {
    await using connection = connectAgent(ctx, {
      async runOutcome() {
        await done;
        return { outcome: "completed", summary: "Done", usage: ctx.usage } as const;
      },
      async steer(text) { return text !== "missed"; },
    }, { inbox: ctx.inbox });
    await connection.runOutcome("Go");
    await ctx.emit("result");
  });

  await observeRunlingEvents(event => events.push(event), async () => {
    const handle = spawn(createWorkflowContext(), child);
    await handle.send("same");
    await handle.send("same");
    await handle.send("missed");
    const { vi } = await import("vitest");
    await vi.waitFor(() => expect(events.filter(e => e.type === "message.receipt")).toHaveLength(3));
    finish();
    const updates = [];
    for await (const update of handle.updates) updates.push(update);
    await handle.result;
    expect(updates).toEqual(["result"]);
  });

  const sent = events.filter(e => e.type === "message.sent");
  expect(sent).toHaveLength(4);
  expect(new Set(sent.map(e => e.id)).size).toBe(4);
  expect(sent.map(e => e.payload)).toEqual(["same", "same", "missed", "result"]);
  expect(events.filter(e => e.type === "message.read")).toHaveLength(4);
  expect(events.filter(e => e.type === "message.receipt").map(e => e.consumed)).toEqual([true, true, false]);
  expect(events.filter(e => e.type === "task.linked")).toHaveLength(1);
  for (const message of sent) {
    expect(events.findIndex(e => e === message)).toBeLessThan(events.findIndex(e => e.type === "message.read" && e.id === message.id));
  }
});

test("message previews tolerate circular values and do not record rejected sends", async () => {
  const { observeRunlingEvents } = await import("./events.ts");
  const events: import("./events.ts").RunlingEvent[] = [];
  const circular: { self?: unknown } = {};
  circular.self = circular;

  await observeRunlingEvents(event => events.push(event), async () => {
    const handle = spawn(createWorkflowContext(), task(async (_ctx: WorkflowContext<object>) => {
      await new Promise(() => {});
    }));
    await handle.send(circular);
    for (let i = 1; i < 64; i++) await handle.send({ i });
    await expect(handle.send({ overflow: true })).rejects.toBeInstanceOf(ChannelFullError);
    handle.cancel();
    await expect(handle.result).rejects.toThrow("cancelled");
  });

  const sent = events.filter(event => event.type === "message.sent");
  expect(sent).toHaveLength(64);
  expect(sent[0]!.payload).toBe("[Value cannot be represented as JSON]");
  expect(events.some(event => event.type === "message.read")).toBe(false);
});
