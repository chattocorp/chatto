import { expect, expectTypeOf, test, vi } from "vitest";
import { z } from "zod";
import * as v from "valibot";
import { toStandardJsonSchema } from "@valibot/to-json-schema";
import { createWorkflowContext, runWorkflow, task, Type, type WorkflowExecution } from "runling";
import { defineWebConfig, isWebConfig, startWorkflow, type WebhookContext, type WebhookRouter } from "runling/web";
import { describeWebhook, handleWebhook, prepareWebhook } from "./webhook.ts";

const request = (body = '"hello"') => new Request("http://localhost/api/webhooks/echo", {
  method: "POST",
  headers: { "content-type": "application/json" },
  body,
});
const echo = task({ name: "Echo", input: Type.String(), output: Type.String() }, (_ctx, input) => input);
const config = defineWebConfig({ webhooks: { echo: startWorkflow(echo) } });

function host() {
  const executions: Promise<WorkflowExecution>[] = [];
  const inputs: unknown[] = [];
  const start: WebhookContext["start"] = async (task, { input }) => {
    inputs.push(input);
    executions.push(runWorkflow(task, { input }));
    return { id: `run-${executions.length}` };
  };
  return { start, executions, inputs };
}

test("starts one workflow and acknowledges its ID without returning task output", async () => {
  const h = host();
  const response = await handleWebhook("echo", request(), { config, start: h.start });
  expect(response.status).toBe(202);
  expect(await response.json()).toEqual({ runs: [{ id: "run-1" }] });
  expect((await h.executions[0])?.output).toBe("hello");
});

test("accepts schema-free routing functions and preserves webhook names", async () => {
  const routed = vi.fn<WebhookRouter>(async () => {});
  const custom = defineWebConfig({ webhooks: { custom: routed } });
  expectTypeOf<keyof typeof custom.webhooks>().toEqualTypeOf<"custom">();
  expect(isWebConfig(custom)).toBe(true);
  const start = vi.fn(async () => ({ id: "unused" }));
  const response = await handleWebhook("custom", request('{"event":"reply"}'), { config: custom, start });

  expect(await response.json()).toEqual({ runs: [] });
  expect(routed).toHaveBeenCalledWith(expect.objectContaining({ start: expect.any(Function) }), { event: "reply" });
  expect(start).not.toHaveBeenCalled();
  expect(await describeWebhook("custom", { config: custom }).json()).toEqual({ input: {}, output: {} });
});

test("a router can select tasks, map input, and fan out sequentially or concurrently", async () => {
  const h = host();
  const number = task({ name: "Number", input: Type.Number(), output: Type.Number() }, (_ctx, input) => input * 2);
  const custom = defineWebConfig({ webhooks: {
    fanout: async (ctx, payload: { name: string; count: number }) => {
      await ctx.start(echo, { input: payload.name });
      await Promise.all([
        ctx.start(number, { input: payload.count }),
        ctx.start(echo, { input: "another" }),
      ]);
    },
  } });
  const response = await handleWebhook("fanout", request('{"name":"Ada","count":3}'), { config: custom, start: h.start });

  expect(await response.json()).toEqual({ runs: [{ id: "run-1" }, { id: "run-2" }, { id: "run-3" }] });
  expect((await Promise.all(h.executions)).map(run => run.output)).toEqual(["Ada", 6, "another"]);
});

test("the convenience router also supports ordinary tasks without schemas", async () => {
  const root = task((_ctx, value: { name: string }) => value.name);
  const h = host();
  const config = defineWebConfig({ webhooks: { plain: startWorkflow(root) } });
  const response = await handleWebhook("plain", request('{"name":"Ada"}'), { config, start: h.start });
  expect(response.status).toBe(202);
  expect((await h.executions[0])?.output).toBe("Ada");
});

test("keeps schema transforms in the task and publishes the correct schema sides", async () => {
  const root = task({
    name: "Transform",
    input: z.object({ count: z.string().default("3").transform(Number) }),
    output: z.object({ count: z.number().default(2) }),
  }, (_ctx, input) => input);
  const config = defineWebConfig({ webhooks: { parsed: startWorkflow(root) } });
  const h = host();
  const response = await handleWebhook("parsed", request("{}"), { config, start: h.start });
  expect(response.status).toBe(202);
  expect(h.inputs).toEqual([{}]);
  expect((await h.executions[0])?.output).toEqual({ count: 3 });
  const schemas = await describeWebhook("parsed", { config }).json();
  expect(schemas.input.required ?? []).not.toContain("count");
  expect(schemas.output.required).toContain("count");
});

test("accepts Valibot schema exports", async () => {
  const schema = toStandardJsonSchema(v.string());
  const root = task({ name: "Valibot", input: schema, output: schema }, (_ctx, input) => input);
  const config = defineWebConfig({ webhooks: { echo: startWorkflow(root) } });
  const h = host();
  await handleWebhook("echo", request(), { config, start: h.start });
  expect((await h.executions[0])?.output).toBe("hello");
  expect(await describeWebhook("echo", { config }).json()).toMatchObject({ input: { type: "string" }, output: { type: "string" } });
});

test.each([
  { schema: Type.Object({ count: Type.Integer({ minimum: 1 }) }), value: { count: 2 }, invalid: { count: 0 } },
  { schema: Type.Array(Type.String()), value: ["one"], invalid: [1] },
  { schema: Type.Boolean(), value: false, invalid: "false" },
  { schema: Type.Number(), value: 0, invalid: "0" },
  { schema: Type.Null(), value: null, invalid: {} },
])("validates $schema.type before starting a run", async ({ schema, value, invalid }) => {
  const root = task({ name: "Typed", input: schema, output: schema }, (_ctx, input) => input);
  const config = defineWebConfig({ webhooks: { typed: startWorkflow(root) } });
  const h = host();
  expect((await handleWebhook("typed", request(JSON.stringify(value)), { config, start: h.start })).status).toBe(202);
  const response = await handleWebhook("typed", request(JSON.stringify(invalid)), { config, start: h.start });
  expect(response.status).toBe(400);
  expect(h.executions).toHaveLength(1);
});

test("custom router metadata validates asynchronously but leaves payload parsing to the router", async () => {
  const route = Object.assign(vi.fn<WebhookRouter<string>>(async () => {}), {
    input: z.string().refine(async value => value === "hello", "Expected hello").transform(value => value.length),
  });
  const config = defineWebConfig({ webhooks: { custom: route } });
  const h = host();
  expect((await handleWebhook("custom", request('"bad"'), { config, start: h.start })).status).toBe(400);
  expect(route).not.toHaveBeenCalled();
  await handleWebhook("custom", request(), { config, start: h.start });
  expect(route.mock.calls[0]?.[1]).toBe("hello");
});

test.each(["not JSON", ""])("rejects malformed JSON: %s", async body => {
  const response = await prepareWebhook("echo", request(body), config);
  expect(response).toBeInstanceOf(Response);
  expect((response as Response).status).toBe(400);
});

test.each(["missing", "constructor", "__proto__", "toString"])("rejects unknown or inherited name %s", async name => {
  expect(describeWebhook(name, { config }).status).toBe(404);
  expect((await handleWebhook(name, request(), { config, start: host().start })).status).toBe(404);
});

test("allows an explicitly configured prototype-like name", () => {
  const config = defineWebConfig({ webhooks: { constructor: startWorkflow(echo) } });
  expect(describeWebhook("constructor", { config }).status).toBe(200);
});

test.each([null, [], {}, { task: echo }, { task: echo, route: async () => {} }, true])("rejects non-function webhook definitions", value => {
  expect(isWebConfig({ webhooks: { echo: value } })).toBe(false);
});

test("rejects invalid schema metadata and unexportable schema tasks", () => {
  const bad = Object.assign(async () => {}, { input: { type: "invalid" } });
  expect(isWebConfig({ webhooks: { bad } })).toBe(false);
  const local = task({ name: "Local", input: v.string(), output: v.string() }, (_ctx, input) => input);
  expect(isWebConfig({ webhooks: { local: startWorkflow(local) } })).toBe(false);
  expect(() => defineWebConfig({ webhooks: { local: startWorkflow(local) } })).toThrow("Webhook");
});

test("waits for registration but never waits for workflow completion", async () => {
  const register = Promise.withResolvers<{ id: string }>();
  const start = vi.fn(() => register.promise);
  const response = handleWebhook("echo", request(), { config, start });
  let settled = false;
  void response.then(() => { settled = true; });
  await vi.waitFor(() => expect(start).toHaveBeenCalledOnce());
  expect(settled).toBe(false);
  register.resolve({ id: "running" });
  expect(await (await response).json()).toEqual({ runs: [{ id: "running" }] });
});

test("observes starts that the router did not await", async () => {
  const config = defineWebConfig({ webhooks: {
    detached: (ctx) => { void ctx.start(echo, { input: "one" }); },
  } });
  const response = await handleWebhook("detached", request(), { config, start: host().start });
  expect(await response.json()).toEqual({ runs: [{ id: "run-1" }] });
});

test("reports partial registration failures without losing successful run IDs", async () => {
  let count = 0;
  const config = defineWebConfig({ webhooks: {
    fanout: async ctx => {
      await Promise.all([ctx.start(echo, { input: "one" }), ctx.start(echo, { input: "two" })]);
    },
  } });
  const response = await handleWebhook("fanout", request(), {
    config,
    start: async () => {
      if (++count === 2) throw new Error("disk full");
      return { id: "retained" };
    },
  });
  expect(response.status).toBe(500);
  expect(await response.json()).toEqual({ error: "disk full", runs: [{ id: "retained" }] });
});

test("router errors still await outstanding registrations and report their IDs", async () => {
  const pending = Promise.withResolvers<{ id: string }>();
  const config = defineWebConfig({ webhooks: {
    broken: ctx => {
      void ctx.start(echo, { input: "one" });
      throw new Error("route failed");
    },
  } });
  const response = handleWebhook("broken", request(), { config, start: () => pending.promise });
  pending.resolve({ id: "survives" });
  expect(await (await response).json()).toEqual({ error: "route failed", runs: [{ id: "survives" }] });
});

test("does not permit starts through a retained routing context after the router exits", async () => {
  let saved!: WebhookContext;
  const config = defineWebConfig({ webhooks: { capture: ctx => { saved = ctx; } } });
  const start = vi.fn(async () => ({ id: "unused" }));
  await handleWebhook("capture", request(), { config, start });
  await expect(saved.start(echo, { input: "late" })).rejects.toThrow("routing has finished");
  void saved.start(echo, { input: "detached late call" });
  await new Promise(resolve => setTimeout(resolve, 0));
  expect(start).not.toHaveBeenCalled();
});

test("separate deliveries do not share registered IDs", async () => {
  const h = host();
  const replies = await Promise.all([
    handleWebhook("echo", request(), { config, start: h.start }),
    handleWebhook("echo", request(), { config, start: h.start }),
  ]);
  expect(await replies[0]!.json()).toEqual({ runs: [{ id: "run-1" }] });
  expect(await replies[1]!.json()).toEqual({ runs: [{ id: "run-2" }] });
});

function typeChecks(ctx: WebhookContext) {
  // @ts-expect-error task input remains typed
  ctx.start(echo, { input: 1 });
  // @ts-expect-error context and payload are required by the convenience router
  startWorkflow(echo)("hello");
  // @ts-expect-error task input is required
  ctx.start(echo, {});
}


test("routers may return a start promise without changing the HTTP response", async () => {
  const config = defineWebConfig({ webhooks: {
    direct: ctx => ctx.start(echo, { input: "hello" }),
  } });

  const response = await handleWebhook("direct", request(), { config, start: host().start });
  expect(await response.json()).toEqual({ runs: [{ id: "run-1" }] });
});
