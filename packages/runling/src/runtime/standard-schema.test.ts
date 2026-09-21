import { createWorkflowContext, type WorkflowContext } from "./context.ts";
import { describe, expect, expectTypeOf, test } from "vitest";
import { z } from "zod";
import * as v from "valibot";
import type { StandardSchemaV1 } from "@standard-schema/spec";
import { Type } from "typebox";
import { task, isSchemaTask } from "./workflow.ts";
import { isWorkflowSchema, validateSchema } from "./schema.ts";
import { observeRunlingEvents, type RunlingEvent } from "./events.ts";
import { runWorkflow } from "./runner.ts";

describe("Standard Schema tasks", () => {
  test("infers Zod input and output types and applies both transforms", async () => {
    const length = task({
      name: "Length",
      input: z.string().transform(value => value.length),
      output: z.number().transform(value => ({ length: value })),
    }, (ctx, input) => {
      expectTypeOf(input).toEqualTypeOf<number>();
      return input;
    });
    expectTypeOf<Parameters<typeof length>>().toEqualTypeOf<[WorkflowContext, string]>();
    expectTypeOf<ReturnType<typeof length>>().toEqualTypeOf<Promise<{ length: number }>>();
    expect(isSchemaTask(length)).toBe(true);
    await expect(length(createWorkflowContext(), "hello")).resolves.toEqual({ length: 5 });
    const execution = await runWorkflow(length, { input: "hello" });
    expect(execution.ok).toBe(true);
    expect(execution.output).toEqual({ length: 5 });
  });

  test("accepts Valibot and infers its parsed input", async () => {
    const length = task({
      name: "Length",
      input: v.pipe(v.string(), v.transform(value => value.length)),
      output: v.number(),
    }, (ctx, value) => {
      expectTypeOf(value).toEqualTypeOf<number>();
      return value;
    });
    expectTypeOf<Parameters<typeof length>>().toEqualTypeOf<[WorkflowContext, string]>();
    expectTypeOf<ReturnType<typeof length>>().toEqualTypeOf<Promise<number>>();
    await expect(length(createWorkflowContext(), "hello")).resolves.toBe(5);
    await expect(length(createWorkflowContext(), 42 as never)).rejects.toThrow('Task "Length" input is invalid');
  });

  test("supports mixed Standard Schema and TypeBox boundaries", async () => {
    const first = task({ name: "First", input: z.string(), output: Type.String() }, (ctx, value) => value);
    const second = task({ name: "Second", input: Type.String(), output: z.string() }, (ctx, value) => value);
    expectTypeOf<ReturnType<typeof first>>().toEqualTypeOf<Promise<string>>();
    expectTypeOf<ReturnType<typeof second>>().toEqualTypeOf<Promise<string>>();
    await expect(first(createWorkflowContext(), "one")).resolves.toBe("one");
    await expect(second(createWorkflowContext(), "two")).resolves.toBe("two");
  });

  test("waits for async input validation before starting an activity", async () => {
    const events: RunlingEvent[] = [];
    const echo = task({
      name: "Async",
      input: z.string().refine(async value => value === "ok", "Expected ok"),
      output: z.string(),
    }, (ctx, value) => value);
    await observeRunlingEvents(event => events.push(event), async () => {
      await expect(echo(createWorkflowContext(), "bad")).rejects.toThrow("Expected ok");
      expect(events).toEqual([]);
      await expect(echo(createWorkflowContext(), "ok")).resolves.toBe("ok");
    });
    expect(events.filter(event => event.type === "step.finished")).toMatchObject([{ status: "completed" }]);
  });

  test("records async output validation failure inside the task activity", async () => {
    const events: RunlingEvent[] = [];
    const echo = task({
      name: "Async output",
      input: z.string(),
      output: z.string().refine(async () => false, "Output rejected"),
    }, async (ctx, value) => value);
    await observeRunlingEvents(event => events.push(event), async () => {
      await expect(echo(createWorkflowContext(), "hello")).rejects.toThrow('Task "Async output" output is invalid');
    });
    expect(events.filter(event => event.type === "step.finished")).toMatchObject([{ status: "failed" }]);
  });

  test("preserves implementation errors and this", async () => {
    const error = new Error("failed");
    const echo = task({ name: "Echo", input: z.string(), output: z.string() }, function (this: { fail: boolean }, ctx, value) {
      if (this.fail) throw error;
      return value;
    });
    await expect(echo.call({ fail: true }, createWorkflowContext(), "hello")).rejects.toBe(error);
    await expect(echo.call({ fail: false }, createWorkflowContext(), "hello")).resolves.toBe("hello");
  });

  test("normalizes issue paths and accepts callable schema objects", async () => {
    const schema = Object.assign(() => {}, {
      "~standard": {
        version: 1 as const,
        vendor: "test",
        validate: () => ({ issues: [{ message: "Invalid", path: [{ key: "a/b~c" }, 0] }] }),
      },
    }) satisfies StandardSchemaV1;
    expect(isWorkflowSchema(schema)).toBe(true);
    expect(await validateSchema(schema, null)).toEqual({ issues: [{ message: "Invalid", path: "/a~1b~0c/0" }] });
  });

  test.each([
    { version: 2, vendor: "test", validate: () => ({ value: "ok" }) },
    { version: 1, validate: () => ({ value: "ok" }) },
    { version: 1, vendor: "test" },
  ])("rejects malformed Standard Schema metadata", standard => {
    expect(isWorkflowSchema({ "~standard": standard })).toBe(false);
  });

  test("requires the implementation result to match the output schema input", () => {
    // @ts-expect-error The implementation must return a string for the output parser.
    task({ name: "Invalid", input: z.string(), output: z.string() }, (ctx) => 42);
  });
});

test("does not start a task aborted during asynchronous schema validation", async () => {
  const ctx = createWorkflowContext();
  const started = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const events: RunlingEvent[] = [];
  let called = false;
  const run = task({
    name: "Validate", input: z.string().refine(async () => {
      started.resolve();
      await release.promise;
      return true;
    }), output: z.string(),
  }, (_ctx, input) => { called = true; return input; });
  await observeRunlingEvents(event => events.push(event), async () => {
    const result = run(ctx, "hello");
    await started.promise;
    try { ctx.abort("Stop validation"); } catch {}
    release.resolve();
    await expect(result).rejects.toBe(ctx.signal.reason);
  });
  expect(called).toBe(false);
  expect(events).toEqual([]);
});
