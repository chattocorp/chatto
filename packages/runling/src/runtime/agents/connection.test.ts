import { expect, expectTypeOf, test, vi } from "vitest";
import { createChannel } from "../channel.ts";
import { createWorkflowContext } from "../context.ts";
import { emptyTokenUsage } from "../usage.ts";
import { connectAgent } from "./index.ts";
import type { AgentRunOptions } from "../agent.ts";

const report = {
  outcome: "completed" as const,
  summary: "Done",
  usage: emptyTokenUsage(),
};

function deferred<T = void>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function fixture() {
  const ctx = createWorkflowContext();
  const inbox = createChannel<string>();
  const done = deferred<typeof report>();
  const worker = {
    runOutcome: vi.fn(
      (_ctx, _prompt, _options?: AgentRunOptions) => done.promise,
    ),
    steer: vi.fn(async () => true),
    dispose: vi.fn(),
  };
  const onText = vi.fn(async (_text: string) => {});
  const onDelivery = vi.fn(async (_text: string, _consumed: boolean) => {});
  const connection = connectAgent(ctx, worker, { inbox, onText, onDelivery });
  return { ctx, inbox, done, worker, onText, onDelivery, connection };
}

test("keeps context and usage explicit; runs without I/O and does not dispose the agent", async () => {
  const f = fixture();
  await using connection = connectAgent(f.ctx, f.worker);
  const result = connection.runOutcome("Hello");
  f.done.resolve(report);

  expect(await result).toBe(report);
  expect(f.worker.runOutcome).toHaveBeenCalledWith(
    f.ctx,
    "Hello",
    expect.any(Object),
  );
  expectTypeOf(result).toEqualTypeOf<
    Promise<import("../agent.ts").AgentResult>
  >();
  await connection.dispose();
  expect(f.worker.dispose).not.toHaveBeenCalled();
});

test("starts the agent before flushing buffered input and consumes each message once", async () => {
  const f = fixture();
  await using connection = f.connection;
  await f.inbox.send("before");
  const run = connection.runOutcome("Go");
  await f.inbox.send("during");

  await vi.waitFor(() =>
    expect(f.onDelivery.mock.calls).toEqual([
      ["before", true],
      ["during", true],
    ]),
  );
  expect(f.worker.runOutcome.mock.invocationCallOrder[0]).toBeLessThan(
    f.worker.steer.mock.invocationCallOrder[0]!,
  );
  f.done.resolve(report);
  await run;
});

test.each(["false", "reject", "throw", "absent"] as const)(
  "reports undelivered messages when steering is %s",
  async (mode) => {
    const f = fixture();
    const steer =
      mode === "absent"
        ? undefined
        : vi.fn(() => {
            if (mode === "throw") throw new Error("sync");
            return mode === "reject"
              ? Promise.reject(new Error("async"))
              : Promise.resolve(false);
          });
    const inbox = createChannel<string>();
    await using connection = connectAgent(
      f.ctx,
      { ...f.worker, steer },
      {
        inbox,
        onDelivery: f.onDelivery,
      },
    );
    const run = connection.runOutcome("Go");
    await inbox.send("message");
    await vi.waitFor(() =>
      expect(f.onDelivery).toHaveBeenCalledWith("message", false),
    );
    f.done.resolve(report);
    await run;
  },
);

test("uses one iterator across turns and reports messages received between turns as missed", async () => {
  const f = fixture();
  await using connection = f.connection;
  const first = connection.runOutcome("Implement");
  f.done.resolve(report);
  await first;

  await f.inbox.send("between");
  await vi.waitFor(() =>
    expect(f.onDelivery).toHaveBeenCalledWith("between", false),
  );
  const second = deferred<typeof report>();
  f.worker.runOutcome.mockReturnValueOnce(second.promise);
  const run = connection.runOutcome("Repair");
  await f.inbox.send("repair");
  await vi.waitFor(() =>
    expect(f.onDelivery).toHaveBeenCalledWith("repair", true),
  );
  second.resolve(report);
  await run;
});

test("serializes asynchronous text delivery and waits before returning the report", async () => {
  const f = fixture();
  await using connection = f.connection;
  const posted = deferred();
  f.onText.mockImplementationOnce(() => posted.promise);
  const run = connection.runOutcome("Go");
  const onText = f.worker.runOutcome.mock.calls[0]![2]!.onText!;
  onText("first");
  onText("second");
  f.done.resolve(report);
  const settled = vi.fn();
  void run.then(settled);

  await vi.waitFor(() => expect(f.onText).toHaveBeenCalledWith("first"));
  expect(f.onText).toHaveBeenCalledTimes(1);
  expect(settled).not.toHaveBeenCalled();
  posted.resolve();
  await run;
  expect(f.onText.mock.calls).toEqual([["first"], ["second"]]);
});

test.each(["text", "delivery", "input"] as const)(
  "propagates %s failure and cancels agent work",
  async (source) => {
    const f = fixture();
    await using connection = f.connection;
    const failure = new Error(source);
    const run = connection.runOutcome("Go");
    const rejected = expect(run).rejects.toBe(failure);

    if (source === "text") {
      f.onText.mockRejectedValueOnce(failure);
      f.worker.runOutcome.mock.calls[0]![2]!.onText!("Working");
    } else if (source === "delivery") {
      f.onDelivery.mockRejectedValueOnce(failure);
      await f.inbox.send("message");
    } else {
      f.inbox.fail(failure);
    }

    await rejected;
    expect(f.worker.runOutcome.mock.calls[0]![2]!.signal!.aborted).toBe(true);
    await expect(connection.runOutcome("Again")).rejects.toBe(failure);
    // Late rejection from uncooperative work is still observed.
    f.done.reject(new Error("late"));
  },
);

test("closing input leaves the active interaction running", async () => {
  const f = fixture();
  await using connection = f.connection;
  const run = connection.runOutcome("Go");
  f.inbox.close();
  f.done.resolve(report);
  expect(await run).toBe(report);
});

test("forwards agent failure and permits a subsequent turn", async () => {
  const f = fixture();
  await using connection = f.connection;
  const run = connection.runOutcome("Go");
  f.done.reject(new Error("agent failed"));
  await expect(run).rejects.toThrow("agent failed");

  f.worker.runOutcome.mockResolvedValueOnce(report);
  expect(await connection.runOutcome("Retry")).toBe(report);
});

test("cleans active state after a synchronous agent failure", async () => {
  const f = fixture();
  await using connection = f.connection;
  f.worker.runOutcome.mockImplementationOnce(() => {
    throw new Error("sync");
  });
  await expect(connection.runOutcome("Go")).rejects.toThrow("sync");
  f.worker.runOutcome.mockResolvedValueOnce(report);
  expect(await connection.runOutcome("Retry")).toBe(report);
});

test("rejects overlapping turns without disturbing the first", async () => {
  const f = fixture();
  await using connection = f.connection;
  const run = connection.runOutcome("First");
  await expect(connection.runOutcome("Second")).rejects.toThrow("active turn");
  expect(f.worker.runOutcome).toHaveBeenCalledTimes(1);
  f.done.resolve(report);
  await run;
});

test.each(["context", "turn", "dispose"] as const)(
  "cancels pending work through %s",
  async (source) => {
    const f = fixture();
    const turn = new AbortController();
    const run = f.connection.runOutcome("Go", { signal: turn.signal });
    const rejected = expect(run).rejects.toThrow();
    if (source === "context") {
      try {
        f.ctx.abort("stop");
      } catch {}
    } else if (source === "turn") {
      turn.abort(new Error("stop"));
    } else {
      await f.connection.dispose();
    }
    await rejected;
    expect(f.worker.runOutcome.mock.calls[0]![2]!.signal!.aborted).toBe(true);
    await f.connection.dispose();
    await expect(f.inbox.send("late")).rejects.toThrow();
  },
);

test("pre-cancelled context and turn never start an interaction", async () => {
  const f = fixture();
  await using connection = f.connection;
  await expect(
    connection.runOutcome("Go", {
      signal: AbortSignal.abort(new Error("turn")),
    }),
  ).rejects.toThrow("turn");
  try {
    f.ctx.abort("context");
  } catch {}
  await expect(connection.runOutcome("Go")).rejects.toThrow("context");
  expect(f.worker.runOutcome).not.toHaveBeenCalled();
});

test("disposal is idempotent, releases the iterator, and prevents reuse", async () => {
  const f = fixture();
  const first = f.connection.dispose();
  expect(f.connection.dispose()).toBe(first);
  await first;
  await expect(f.connection.runOutcome("Go")).rejects.toThrow("disposed");
  await expect(f.inbox.send("late")).rejects.toThrow();
  expect(f.worker.dispose).not.toHaveBeenCalled();
});

test("disposes despite an uncooperative iterable and a pending steering promise", async () => {
  const next = vi.fn(() => new Promise<IteratorResult<string>>(() => {}));
  const returned = vi.fn(() => new Promise<IteratorResult<string>>(() => {}));
  const f = fixture();
  const connection = connectAgent(f.ctx, f.worker, {
    inbox: { [Symbol.asyncIterator]: () => ({ next, return: returned }) },
  });
  const run = connection.runOutcome("Go");
  const rejected = expect(run).rejects.toThrow("disposed");
  await connection.dispose();
  await rejected;
  expect(returned).toHaveBeenCalledOnce();

  const g = fixture();
  g.worker.steer.mockImplementation(() => new Promise(() => {}));
  const work = g.connection.runOutcome("Go");
  const cancelled = expect(work).rejects.toThrow("disposed");
  await g.inbox.send("pending");
  await vi.waitFor(() => expect(g.worker.steer).toHaveBeenCalledOnce());
  await g.connection.dispose();
  await cancelled;
});

test("separate connections do not mix context, input, output, or cancellation", async () => {
  const a = fixture();
  const b = fixture();
  await using first = a.connection;
  await using second = b.connection;
  const failed = expect(first.runOutcome("A")).rejects.toThrow("disposed");
  const run = second.runOutcome("B");
  await a.inbox.send("only A");
  await b.inbox.send("only B");
  await vi.waitFor(() =>
    expect(b.onDelivery).toHaveBeenCalledWith("only B", true),
  );
  await first.dispose();
  await failed;
  expect(b.worker.runOutcome.mock.calls[0]![2]!.signal!.aborted).toBe(false);
  b.done.resolve(report);
  await run;
  expect(b.worker.steer).not.toHaveBeenCalledWith("only A");
});

test("finishes pending text on agent failure and ignores text after settlement", async () => {
  const f = fixture();
  await using connection = f.connection;
  const posted = deferred();
  f.onText.mockReturnValueOnce(posted.promise);
  const run = connection.runOutcome("Go");
  const rejected = expect(run).rejects.toThrow("agent failed");
  const onText = f.worker.runOutcome.mock.calls[0]![2]!.onText!;
  onText("progress");
  f.done.reject(new Error("agent failed"));

  await vi.waitFor(() => expect(f.onText).toHaveBeenCalledWith("progress"));
  posted.resolve();
  await rejected;
  onText("late");
  await Promise.resolve();
  expect(f.onText).toHaveBeenCalledTimes(1);
});

test("cancellation releases an output handler that never resolves", async () => {
  const f = fixture();
  f.onText.mockReturnValueOnce(new Promise(() => {}));
  const run = f.connection.runOutcome("Go");
  const rejected = expect(run).rejects.toThrow("disposed");
  f.worker.runOutcome.mock.calls[0]![2]!.onText!("Working");
  f.done.resolve(report);

  await vi.waitFor(() => expect(f.onText).toHaveBeenCalledOnce());
  await f.connection.dispose();
  await rejected;
});

test("one agent can be reconnected to independent contexts without double accounting", async () => {
  const first = createWorkflowContext();
  const second = createWorkflowContext();
  const worker = {
    runOutcome: vi.fn(async (ctx: typeof first) => {
      ctx.recordUsage({
        input: 2,
        output: 1,
        cacheRead: 0,
        cacheWrite: 0,
        cost: 0.5,
      });
      return report;
    }),
  };
  {
    await using connection = connectAgent(first, worker);
    await connection.runOutcome("First");
    await connection.runOutcome("Repair");
  }
  {
    await using connection = connectAgent(second, worker);
    await connection.runOutcome("Other workflow");
  }
  expect(first.usage.input).toBe(4);
  expect(first.usage.cost).toBe(1);
  expect(second.usage.input).toBe(2);
  expect(second.usage.cost).toBe(0.5);
});

test("input claims a single consumer and cleanup tolerates a throwing return", async () => {
  const f = fixture();
  expect(() => connectAgent(f.ctx, f.worker, { inbox: f.inbox })).toThrow(
    "consumer",
  );
  const returned = vi.fn(() => {
    throw new Error("return failed");
  });
  const connection = connectAgent(f.ctx, f.worker, {
    inbox: {
      [Symbol.asyncIterator]: () => ({
        next: async () => ({ done: true as const, value: undefined }),
        return: returned,
      }),
    },
  });
  await connection.dispose();
  expect(returned).toHaveBeenCalledOnce();
  await f.connection.dispose();
});

test("cancellation wins over an already resolved result at the output boundary", async () => {
  const f = fixture();
  await using connection = f.connection;
  f.worker.runOutcome.mockImplementationOnce(async (_ctx, _prompt, options) => {
    options?.onText?.("before cancellation");
    try {
      f.ctx.abort("stop now");
    } catch {}
    return report;
  });

  await expect(connection.runOutcome("Go")).rejects.toThrow("stop now");
  expect(f.onText).not.toHaveBeenCalled();
});

test("an exhausted inbox does not reopen between interactions", async () => {
  const f = fixture();
  await using connection = f.connection;
  f.inbox.close();
  f.worker.runOutcome.mockResolvedValue(report);
  expect(await connection.runOutcome("First")).toBe(report);
  expect(await connection.runOutcome("Second")).toBe(report);
  expect(f.onDelivery).not.toHaveBeenCalled();
});

test("waits for a pending delivery callback before completing the turn", async () => {
  const f = fixture();
  await using connection = f.connection;
  const delivered = deferred();
  f.onDelivery.mockReturnValueOnce(delivered.promise);
  const run = connection.runOutcome("Go");
  const settled = vi.fn();
  void run.then(settled);
  await f.inbox.send("message");
  await vi.waitFor(() =>
    expect(f.onDelivery).toHaveBeenCalledWith("message", true),
  );
  f.done.resolve(report);
  await Promise.resolve();
  expect(settled).not.toHaveBeenCalled();

  delivered.resolve();
  await run;
  expect(settled).toHaveBeenCalledOnce();
});

test("forwards later messages while an earlier consumption acknowledgement is pending", async () => {
  const f = fixture();
  await using connection = f.connection;
  const first = Promise.withResolvers<boolean>();
  f.worker.steer.mockImplementationOnce(() => first.promise);
  const run = connection.runOutcome("Go");
  void run.catch(() => {});
  await f.inbox.send("first");
  await vi.waitFor(() => expect(f.worker.steer).toHaveBeenCalledOnce());
  await f.inbox.send("second");
  await vi.waitFor(() => expect(f.worker.steer).toHaveBeenCalledTimes(2));

  first.resolve(true);
  f.done.resolve(report);
  await run;
  expect(f.onDelivery.mock.calls).toEqual([
    ["first", true],
    ["second", true],
  ]);
});
