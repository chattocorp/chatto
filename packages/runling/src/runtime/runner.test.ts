import { createWorkflowContext } from "./context.ts";
import { log } from "./log.ts";
import { stripVTControlCharacters } from "node:util";
import { afterEach, describe, expect, expectTypeOf, test } from "vitest";
import {
  executeWorkflow,
  formatDuration,
  formatWorkflowDetails,
  normalizeWorkflowResult,
  runWorkflow,
  shouldUseTui,
} from "./runner.ts";
import type { RunlingEvent } from "./events.ts";
import type { InputRequest } from "./input.ts";
import { input as askInput } from "./input.ts";
import { Type } from "typebox";
import { task } from "./workflow.ts";

const initialExitCode = process.exitCode;

afterEach(() => {
  process.exitCode = initialExitCode ?? 0;
});

describe("executeWorkflow", () => {
  test("supplies an explicit context to the workflow", async () => {
    const execution = await executeWorkflow(function (...args) {
      expect(args).toHaveLength(1);
      expect(args[0]?.usage).toEqual({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0 });
      expect(args[0]?.recordUsage).toBeTypeOf("function");
      return "done";
    });
    expect(execution.output).toBe("done");
  });

  test("logs a workflow summary", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(async (ctx) => "Made the change");
    } finally {
      console.log = originalLog;
    }

    expect(logs.some((line) => line.includes("Made the change"))).toBe(true);
  });

  test("accepts structured workflow results", async () => {
    const execution = await executeWorkflow(
      async () => ({
        summary: "Opened the pull request",
        details: "## Summary\n\nImplemented the requested change.",
        outputs: { pullRequestUrl: "https://example.com/pull/1" },
      }),
    );

    expect(execution.ok).toBe(true);
    expect(execution.result).toEqual({
      summary: "Opened the pull request",
      details: "## Summary\n\nImplemented the requested change.",
      outputs: { pullRequestUrl: "https://example.com/pull/1" },
    });
  });

  test("preserves raw result details when stdout is not interactive", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(
        async () => ({
          summary: "Review complete",
          details: "## Findings\n\nSomething worth reading.",
        }),
          { terminal: { isTTY: false, columns: 80 } },
      );
    } finally {
      console.log = originalLog;
    }

    expect(logs).toContain("\n## Findings\n\nSomething worth reading.\n");
  });

  test("renders result details when stdout is interactive", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(
        async () => ({
          summary: "Review complete",
          details: "## Findings\n\nSomething worth reading.",
        }),
          { terminal: { isTTY: true, columns: 80 } },
      );
    } finally {
      console.log = originalLog;
    }

    const details = logs.find((line) => line.includes("Findings"));
    expect(details).toBeDefined();
    expect(details).toContain("\x1b[");
    expect(stripVTControlCharacters(details ?? "")).not.toContain("##");
  });

  test("emits only the execution document to stdout in JSON mode", async () => {
    const markdown = "## Details\n\n**Still raw.**";
    const output: string[] = [];
    const errors: string[] = [];
    const originalLog = console.log;
    const originalError = console.error;
    console.log = (message: string) => output.push(message);
    console.error = (message: string) => errors.push(message);

    try {
      await executeWorkflow(
        async () => ({
          summary: "Done",
          details: markdown,
          outputs: { count: 2 },
        }),
          { json: true, terminal: { isTTY: true, columns: 80 } },
      );
    } finally {
      console.log = originalLog;
      console.error = originalError;
    }

    expect(output).toHaveLength(1);
    expect(JSON.parse(output[0] ?? "")).toMatchObject({
      ok: true,
      error: null,
      result: { summary: "Done", details: markdown, outputs: { count: 2 } },
    });
    expect(output[0]).not.toContain("\x1b[");
    expect(errors.some((line) => line.includes("Runling starting"))).toBe(true);
    expect(errors.some((line) => line.includes("Finished in "))).toBe(true);
  });

  test("indents workflow log output below the runling greeting", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(async (ctx) => {
        log.info("inside the workflow");
        return "Made the change";
      });
    } finally {
      console.log = originalLog;
    }

    const greeting = logs.find((line) => line.includes("Runling starting"));
    const inside = logs.find((line) => line.includes("inside the workflow"));
    const summary = logs.find((line) => line.includes("Made the change"));
    const finished = logs.find((line) => line.includes("Finished in "));

    expect(greeting).toBeDefined();
    expect(greeting).not.toMatch(/^\s/);
    expect(inside).toBeDefined();
    expect(inside).toMatch(/^ {2}/);
    expect(summary).toBeDefined();
    expect(summary).not.toMatch(/^\s/);
    expect(finished).not.toMatch(/^\s/);
  });

  test("logs failures and applies a nonzero exit code", async () => {
    const errors: string[] = [];
    const originalError = console.error;
    console.error = (message: string) => errors.push(message);

    try {
      await executeWorkflow(async (ctx) => {
        throw "Tests failed";
      });
    } finally {
      console.error = originalError;
    }

    expect(errors.some((line) => line.includes("Tests failed"))).toBe(true);
    expect(process.exitCode).toBe(1);
  });

  test("logs the elapsed time after a successful run", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(async (ctx) => undefined);
    } finally {
      console.log = originalLog;
    }

    expect(logs.some((line) => line.includes("Finished in "))).toBe(true);
  });

  test("logs the elapsed time after a failed run", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(async (ctx) => {
        throw "Tests failed";
      });
    } finally {
      console.log = originalLog;
    }

    expect(logs.some((line) => line.includes("Finished in "))).toBe(true);
  });

  test("logs accumulated token usage from agent interactions", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(async (ctx) => {
        ctx.recordUsage({ input: 100, output: 20, cacheRead: 500, cacheWrite: 10 });
        ctx.recordUsage({ input: 50, output: 25, cacheRead: 550, cacheWrite: 15 });
      });
    } finally {
      console.log = originalLog;
    }

    expect(
      logs.some((line) =>
        line.includes(
          "Total token usage: in 150, out 45, cache read 1,050, cache write 25",
        ),
      ),
    ).toBe(true);
    expect(logs.some((line) => line.includes("Finished in "))).toBe(true);
  });

  test("omits the token usage line when no tokens were recorded", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(async (ctx) => undefined);
    } finally {
      console.log = originalLog;
    }

    expect(logs.some((line) => line.includes("Total token usage"))).toBe(false);
  });

  test("resets token usage totals between executions", async () => {
    createWorkflowContext().recordUsage({ input: 999, output: 999, cacheRead: 999, cacheWrite: 999 });
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);

    try {
      await executeWorkflow(async (ctx) => {
        ctx.recordUsage({ input: 10, output: 5, cacheRead: 0, cacheWrite: 0 });
      });
    } finally {
      console.log = originalLog;
    }

    expect(
      logs.some((line) =>
        line.includes("Total token usage: in 10, out 5"),
      ),
    ).toBe(true);
    expect(logs.some((line) => line.includes("999"))).toBe(false);
  });
});

describe("runWorkflow", () => {
  test("preserves the task's resolved output type", async () => {
    const done = task(
      { name: "Done", input: Type.String(), output: Type.String() },
      (ctx) => "done" as const,
    );
    const execution = await runWorkflow(done, { input: "input" });

    expectTypeOf(execution.output).toEqualTypeOf<"done" | null>();
  });

  test("does not use the removed prompt option as workflow input", async () => {
    let started = false;
    const echo = task(
      { name: "Echo", input: Type.String(), output: Type.String() },
      (ctx, input) => { started = true; return input; },
    );
    // @ts-expect-error Callers must provide input, not a legacy prompt option.
    const execution = await runWorkflow(echo, { prompt: "Legacy fallback" });
    expect(execution.ok).toBe(false);
    expect(execution.error).toContain('Task "Echo" input is invalid');
    expect(started).toBe(false);
    expect(await runWorkflow(echo, { input: "" })).toMatchObject({ ok: true, output: "" });
  });
  test("runs headlessly with host-provided input and event handling", async () => {
    const events: RunlingEvent[] = [];
    const requested = Promise.withResolvers<InputRequest>();
    const answer = Promise.withResolvers<string>();
    const joke = task(
      {
        name: "Tell joke",
        input: Type.String(),
        output: Type.String(),
      },
      async (ctx, input) => {
        expect(input).toBe("Make me laugh");
        const topic = await askInput(ctx, "What is the topic?");
        return `A joke about ${topic}`;
      },
    );
    const running = runWorkflow(
      joke,
      {
        input: "Make me laugh",
        onInput: (request) => {
          requested.resolve(request);
          return answer.promise;
        },
        onEvent: (event) => events.push(event),
      },
    );

    expect((await requested.promise).message).toBe("What is the topic?");
    answer.resolve("robots");
    const execution = await running;

    expect(execution).toMatchObject({
      ok: true,
      error: null,
      output: "A joke about robots",
      result: { summary: "A joke about robots" },
    });
    expect(events).toContainEqual(
      expect.objectContaining({
        type: "input.finished",
        status: "answered",
        value: "robots",
      }),
    );
  });

  test("captures failures without changing the process exit code", async () => {
    const exitCode = process.exitCode;
    const failing = task(
      { name: "Fail", input: Type.String(), output: Type.String() },
      async (ctx) => {
        throw new Error("Nope");
      },
    );
    const execution = await runWorkflow(failing, { input: "" });

    expect(execution).toMatchObject({ ok: false, error: "Nope", result: null });
    expect(process.exitCode).toBe(exitCode);
  });
});

describe("formatWorkflowDetails", () => {
  test("renders Markdown for an interactive terminal", () => {
    const formatted = formatWorkflowDetails("## Findings\n\n**Important**", {
      isTTY: true,
      columns: 80,
    });

    expect(formatted).toContain("\x1b[");
    expect(stripVTControlCharacters(formatted)).not.toContain("##");
    expect(stripVTControlCharacters(formatted)).not.toContain("**");
  });

  test("preserves Markdown for redirected output", () => {
    expect(
      formatWorkflowDetails("## Findings\n\n**Important**", {
        isTTY: false,
        columns: 80,
      }),
    ).toBe("## Findings\n\n**Important**");
  });
});

describe("shouldUseTui", () => {
  const interactive = { stdinIsTTY: true, stdoutIsTTY: true };

  test("uses the TUI for an interactive terminal", () => {
    expect(shouldUseTui({ json: false, log: false, verbose: false }, interactive)).toBe(true);
  });

  test("uses logs for redirected input or output", () => {
    expect(
      shouldUseTui({ json: false, log: false, verbose: false }, {
        stdinIsTTY: false,
        stdoutIsTTY: true,
      }),
    ).toBe(false);
    expect(
      shouldUseTui({ json: false, log: false, verbose: false }, {
        stdinIsTTY: true,
        stdoutIsTTY: false,
      }),
    ).toBe(false);
  });

  test("allows log, verbose, and JSON modes to override an interactive terminal", () => {
    expect(shouldUseTui({ json: false, log: true, verbose: false }, interactive)).toBe(false);
    expect(shouldUseTui({ json: false, log: false, verbose: true }, interactive)).toBe(false);
    expect(shouldUseTui({ json: true, log: false, verbose: false }, interactive)).toBe(false);
  });
});

describe("normalizeWorkflowResult", () => {
  test("rejects non-JSON values even when a summary is present", () => {
    const circular: Record<string, unknown> = { summary: "Invalid" };
    circular.self = circular;
    for (const output of [
      circular,
      { summary: "Invalid", extra: 1n },
      { summary: "Invalid", extra: () => "value" },
      { values: [undefined] },
    ]) {
      expect(() => normalizeWorkflowResult(output)).toThrow(
        "Workflow output must be valid JSON",
      );
    }
  });

  test("keeps string-returning workflows compatible", () => {
    expect(normalizeWorkflowResult("Made the change")).toEqual({
      summary: "Made the change",
    });
  });

  test("rejects outputs that cannot be represented as JSON", () => {
    expect(() =>
      normalizeWorkflowResult({
        summary: "Invalid",
        outputs: { value: Number.NaN },
      }),
    ).toThrow("Workflow output must be valid JSON");
  });
});

describe("formatDuration", () => {
  test("formats sub-second durations in milliseconds", () => {
    expect(formatDuration(0)).toBe("0ms");
    expect(formatDuration(42.4)).toBe("42ms");
    expect(formatDuration(999)).toBe("999ms");
  });

  test("formats sub-minute durations in seconds", () => {
    expect(formatDuration(1000)).toBe("1.0s");
    expect(formatDuration(12_345)).toBe("12.3s");
  });

  test("formats durations in minutes and seconds", () => {
    expect(formatDuration(59_949)).toBe("59.9s");
    expect(formatDuration(59_950)).toBe("1m");
    expect(formatDuration(60_000)).toBe("1m");
    expect(formatDuration(61_000)).toBe("1m1s");
    expect(formatDuration(125_000)).toBe("2m5s");
  });

  test("formats durations in hours, minutes, and seconds", () => {
    expect(formatDuration(3_599_600)).toBe("1h");
    expect(formatDuration(3_600_000)).toBe("1h");
    expect(formatDuration(3_660_000)).toBe("1h1m");
    expect(formatDuration(3_661_000)).toBe("1h1m1s");
    expect(formatDuration(7_385_000)).toBe("2h3m5s");
  });
});

test("isolates workflow contexts and usage events across concurrent executions", async () => {
  const seen: unknown[] = [];
  const events: RunlingEvent[][] = [[], []];
  const run = task(async (ctx, count: number) => {
    seen.push(ctx);
    await Promise.all([1, 2].map(async () => {
      await Promise.resolve();
      ctx.recordUsage({ input: count, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.5 });
    }));
    return count;
  });
  const [first, second] = await Promise.all([
    runWorkflow(run, { input: 10, onEvent: event => events[0]!.push(event) }),
    runWorkflow(run, { input: 100, onEvent: event => events[1]!.push(event) }),
  ]);
  expect(seen[0]).not.toBe(seen[1]);
  expect(first.usage.input).toBe(20);
  expect(second.usage.input).toBe(200);
  expect(first.usage.cost).toBe(1);
  expect(events.map(list => list.filter(e => e.type === "usage.updated").map(e => e.usage.input)))
    .toEqual([[10, 20], [100, 200]]);
});

test("keeps parent accounting independent of a failed nested workflow execution", async () => {
  const parent = task(async (ctx) => {
    ctx.recordUsage({ input: 10, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.5 });
    const child = await runWorkflow(task((inner) => {
      expect(inner).not.toBe(ctx);
      inner.recordUsage({ input: 100, output: 0, cacheRead: 0, cacheWrite: 0, cost: 5 });
      throw new Error("Child failed");
    }), { input: undefined });
    expect(child.ok).toBe(false);
    expect(child.usage.input).toBe(100);
    expect(ctx.usage.input).toBe(10);
    throw new Error("Parent failed");
  });
  const result = await runWorkflow(parent, { input: undefined });
  expect(result.error).toBe("Parent failed");
  expect(result.usage.input).toBe(10);
});

test("emits context usage updates from callbacks created outside the run", async () => {
  const { AsyncResource } = await import("node:async_hooks");
  const resource = new AsyncResource("external-usage");
  const events: RunlingEvent[] = [];
  try {
    const execution = await runWorkflow(task((ctx) => {
      resource.runInAsyncScope(() => {
        ctx.recordUsage({ input: 10, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
      });
      return "done";
    }), { input: undefined, onEvent: event => events.push(event) });
    expect(execution.usage.input).toBe(10);
    expect(events.filter(event => event.type === "usage.updated").map(event => event.usage))
      .toEqual([execution.usage]);
  } finally {
    resource.emitDestroy();
  }
});

test("aborts nested tasks, runs cleanup, and preserves recorded usage", async () => {
  const calls: string[] = [];
  const child = task((ctx) => {
    ctx.recordUsage({ input: 10, output: 2, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
    ctx.abort("Budget exceeded");
    calls.push("unreachable");
  });
  const parent = task(async (ctx) => {
    try {
      child(ctx);
      calls.push("continued");
    } finally {
      calls.push("cleanup");
    }
  });
  const execution = await runWorkflow(parent, { input: undefined });
  expect(calls).toEqual(["cleanup"]);
  expect(execution).toMatchObject({
    ok: false, error: "Budget exceeded", output: null, result: null,
    usage: { input: 10, output: 2, cost: 0.25 },
  });
});

test("cannot turn a caught abort into a successful run", async () => {
  const run = task((ctx) => {
    try { ctx.abort("Stop"); } catch {}
    return "success";
  });
  const execution = await runWorkflow(run, { input: undefined });
  expect(execution).toMatchObject({ ok: false, error: "Stop", output: null, result: null });
});

test("cancels cooperative parallel work without cancelling another run", async () => {
  const started = Promise.withResolvers<void>();
  let cleanedUp = false;
  const waiting = task(async (ctx) => {
    try {
      await new Promise<void>(resolve => {
        ctx.signal.addEventListener("abort", () => resolve(), { once: true });
        started.resolve();
      });
    } finally { cleanedUp = true; }
  });
  const parent = task(async (ctx) => {
    const pending = waiting(ctx);
    await started.promise;
    try { ctx.abort("Stop siblings"); } finally { await pending; }
  });
  const [aborted, successful] = await Promise.all([
    runWorkflow(parent, { input: undefined }),
    runWorkflow(task((ctx) => {
      expect(ctx.signal.aborted).toBe(false);
      return "done";
    }), { input: undefined }),
  ]);
  expect(cleanedUp).toBe(true);
  expect(aborted).toMatchObject({ ok: false, error: "Stop siblings" });
  expect(successful).toMatchObject({ ok: true, output: "done" });
});


test("host handlers can be overridden by child contexts without changing the parent", async () => {
  const child = task(async (ctx) => {
    ctx.recordUsage({ input: 3, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
    return askInput(ctx, "child question");
  });
  const workflow = task(async (ctx) => {
    const override = { ...ctx, onInput: async () => "child answer" };
    const answers = await Promise.all([
      child(override),
      askInput(ctx, "parent question"),
    ]);
    expect(override.usage).toBe(ctx.usage);
    return answers;
  });
  const result = await runWorkflow(workflow, {
    input: undefined,
    onInput: async () => "host answer",
  });
  expect(result.output).toEqual(["child answer", "host answer"]);
  expect(result.usage.input).toBe(3);
});

test("completed executions retain usage snapshots after the context changes", async () => {
  const contexts: ReturnType<typeof createWorkflowContext>[] = [];
  const result = await runWorkflow(task((ctx) => {
    contexts.push(ctx);
    ctx.recordUsage({ input: 1, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
    return "done";
  }), { input: undefined });
  contexts[0]!.recordUsage({ input: 2, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.5 });
  expect(result.usage.input).toBe(1);
  expect(result.usage.cost).toBe(0.25);
  expect(contexts[0]!.usage.input).toBe(3);
});

test("passes text handlers through nested tasks and isolates concurrent runs", async () => {
  const first: string[] = [];
  const second: string[] = [];
  const leaf = task(async (ctx, text: string) => { await ctx.onText?.(text); });
  const root = task(async (ctx) => {
    await Promise.all([leaf(ctx, "one"), leaf(ctx, "two")]);
    const parent = ctx.onText;
    await leaf({ ...ctx, onText: async text => { await parent?.(`child: ${text}`); } }, "three");
    await leaf(ctx, "four");
  });
  const results = await Promise.all([
    runWorkflow(root, { input: undefined, onText: async text => { first.push(text); } }),
    runWorkflow(root, { input: undefined, onText: text => { second.push(text); } }),
  ]);
  expect(results.every(result => result.ok)).toBe(true);
  expect(first).toEqual(["one", "two", "child: three", "four"]);
  expect(second).toEqual(first);
  expect((await runWorkflow(root, { input: undefined })).ok).toBe(true);
});

test("awaits text delivery and reports handler failures through normal task errors", async () => {
  let release!: () => void;
  const delivered = new Promise<void>(resolve => { release = resolve; });
  let finished = false;
  const root = task(async ctx => { await ctx.onText?.("hello"); finished = true; });
  const execution = runWorkflow(root, { input: undefined, onText: () => delivered });
  expect(finished).toBe(false);
  release();
  expect((await execution).ok).toBe(true);
  expect(finished).toBe(true);
  const failure = await runWorkflow(root, { input: undefined, onText: async () => { throw new Error("delivery failed"); } });
  expect(failure.ok).toBe(false);
  expect(failure.error).toContain("delivery failed");
});

test("does not start a workflow when the host signal is already aborted", async () => {
  let called = false;
  const execution = await runWorkflow(() => { called = true; }, {
    input: undefined,
    signal: AbortSignal.abort(new Error("Cancelled by host")),
  });
  expect(called).toBe(false);
  expect(execution).toMatchObject({ ok: false, error: "Cancelled by host" });
});
