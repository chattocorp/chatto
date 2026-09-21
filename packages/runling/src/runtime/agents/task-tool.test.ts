import { expect, expectTypeOf, test, vi } from "vitest";
import { Type } from "typebox";
import { createWorkflowContext } from "../context.ts";
import { task } from "../workflow.ts";
import { taskTool } from "./task-tool.ts";

const definition = {
  name: "greet",
  label: "Greet",
  description: "Return a greeting",
  parameters: Type.Object({ name: Type.String() }),
};

test("preserves metadata, inferred arguments, context extensions and synchronous results", async () => {
  const ctx = { ...createWorkflowContext(), prefix: "Hello" };
  const tool = taskTool(ctx, definition, (received, input) => {
    expect(received).toBe(ctx);
    expectTypeOf(input).toEqualTypeOf<{ name: string }>();
    expectTypeOf(received.prefix).toEqualTypeOf<string>();
    return `${received.prefix}, ${input.name}`;
  });

  expect(tool.parameters).toBe(definition.parameters);
  expect(tool.name).toBe(definition.name);
  expect(await tool.execute("id", { name: "Ada" })).toEqual({
    content: [{ type: "text", text: "Hello, Ada" }],
    details: {},
  });
});

test("ordinary task calls retain input/output validation and async results", async () => {
  const greet = task({
    name: "Greet",
    input: definition.parameters,
    output: Type.String(),
  }, async (_ctx, input) => input.name);
  const tool = taskTool(createWorkflowContext(), definition, greet);

  expect((await tool.execute("id", { name: "Ada" })).content[0]?.text).toBe("Ada");
  await expect(tool.execute("bad", { name: 1 } as never)).rejects.toThrow();
});

test.each(["root", "tool"])("rejects pre-cancelled %s signals before calling the task", async (source) => {
  const root = new AbortController();
  const turn = new AbortController();
  const reason = new Error("cancelled");
  (source === "root" ? root : turn).abort(reason);
  const run = vi.fn(() => "unused");
  const tool = taskTool({ ...createWorkflowContext(), signal: root.signal }, definition, run);

  await expect(tool.execute("id", { name: "Ada" }, turn.signal)).rejects.toBe(reason);
  expect(run).not.toHaveBeenCalled();
});

test("concurrent tools share accounting but isolate cancellation", async () => {
  const ctx = createWorkflowContext();
  const first = new AbortController();
  const second = new AbortController();
  const contexts: typeof ctx[] = [];
  const release = Promise.withResolvers<void>();
  const tool = taskTool(ctx, definition, async (received, input) => {
    contexts.push(received);
    received.recordUsage({ input: 1, output: 0, cacheRead: 0, cacheWrite: 0 });
    await release.promise;
    received.signal.throwIfAborted();
    return input.name;
  });

  const a = tool.execute("a", { name: "a" }, first.signal);
  const b = tool.execute("b", { name: "b" }, second.signal);
  const reason = new Error("cancel a");
  first.abort(reason);
  release.resolve();

  await expect(a).rejects.toBe(reason);
  expect((await b).content[0]?.text).toBe("b");
  expect(ctx.signal.aborted).toBe(false);
  expect(ctx.usage.input).toBe(2);
  expect(contexts[0]?.usage).toBe(ctx.usage);
  expect(contexts[1]?.signal.aborted).toBe(false);
});

test("propagates task errors unchanged", async () => {
  const error = new Error("failed");
  const tool = taskTool(createWorkflowContext(), definition, () => { throw error; });
  await expect(tool.execute("id", { name: "Ada" })).rejects.toBe(error);
});

test("the callback can preserve schema transforms and choose result formatting", async () => {
  const { z } = await import("zod");
  const transform = task({
    name: "Transform",
    input: z.object({ name: z.string().transform(value => value.length) }),
    output: z.number().transform(value => `Length: ${value}`),
  }, (_ctx, input) => input.name);
  const tool = taskTool(createWorkflowContext(), definition, transform);

  expect((await tool.execute("id", { name: "Ada" })).content[0]?.text).toBe("Length: 3");
});

test("root cancellation reaches every active tool context", async () => {
  const controller = new AbortController();
  const ctx = { ...createWorkflowContext(), signal: controller.signal };
  const signals: AbortSignal[] = [];
  const release = Promise.withResolvers<void>();
  const tool = taskTool(ctx, definition, async (received) => {
    signals.push(received.signal);
    await release.promise;
    received.signal.throwIfAborted();
    return "done";
  });
  const a = tool.execute("a", { name: "a" }, new AbortController().signal);
  const b = tool.execute("b", { name: "b" });
  controller.abort(new Error("stop all"));
  release.resolve();

  await expect(a).rejects.toThrow("stop all");
  await expect(b).rejects.toThrow("stop all");
  expect(signals.every(signal => signal.aborted)).toBe(true);
});
