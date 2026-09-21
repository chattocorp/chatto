import { createWorkflowContext } from "./context.ts";
import { stripVTControlCharacters } from "node:util";
import { expect, test, vi } from "vitest";
import {
  observeRunlingEvents,
  withRunlingActivity,
  type RunlingEvent,
} from "./events.ts";
import { createInput, input, InputUnavailableError } from "./input.ts";

test("delegates input to the host and reports its lifecycle", async () => {
  const events: RunlingEvent[] = [];
  const input = createInput(async (request) => {
    expect(request.message).toBe("Which environment?");
    expect(request.defaultValue).toBe("staging");
    return "production";
  });

  const answer = await observeRunlingEvents(
    (event) => events.push(event),
    () =>
      withRunlingActivity("deploy", () =>
        input("Which environment?", { defaultValue: "staging" }),
      ),
  );

  expect(answer).toBe("production");
  const requested = events.find((event) => event.type === "input.requested");
  const finished = events.find((event) => event.type === "input.finished");
  const logs = events.filter((event) => event.type === "log");

  expect(requested).toMatchObject({
    type: "input.requested",
    activityId: "deploy",
    message: "Which environment?",
    defaultValue: "staging",
  });
  expect(finished).toMatchObject({
    type: "input.finished",
    id: requested?.type === "input.requested" ? requested.id : undefined,
    status: "answered",
    value: "production",
  });
  expect(logs.map(({ message }) => stripVTControlCharacters(message))).toEqual([
    "Asking Which environment?",
    "Answered production",
  ]);
  expect(logs.every((event) => event.source === "input")).toBe(true);
});

test("fails clearly when the host has no input handler", async () => {
  await expect(createInput()("Choose wisely")).rejects.toBeInstanceOf(
    InputUnavailableError,
  );
});

test("rejects invalid host responses", async () => {
  const input = createInput(async () => 42 as never);

  await expect(input("What is the answer?")).rejects.toThrow(
    "An input handler must return a string",
  );
});


test("parallel spread contexts route questions independently and permit composition", async () => {
  const parent = createWorkflowContext();
  parent.onInput = async ({ message }) => "parent:" + message;
  const previous = parent.onInput;
  const child = {
    ...parent,
    onInput: async (request: Parameters<typeof previous>[0]) =>
      "child:" + await previous(request),
  };
  expect(await Promise.all([input(parent, "one"), input(child, "two")]))
    .toEqual(["parent:one", "child:parent:two"]);
});

test("captures the handler when a question starts", async () => {
  const ctx = createWorkflowContext();
  const answer = Promise.withResolvers<string>();
  ctx.onInput = () => answer.promise;
  const first = input(ctx, "First");
  ctx.onInput = async () => "new handler";
  answer.resolve("original handler");
  expect(await first).toBe("original handler");
  expect(await input(ctx, "Second")).toBe("new handler");
});

test("aborts pending input even if its handler ignores cancellation", async () => {
  const ctx = createWorkflowContext();
  const started = Promise.withResolvers<void>();
  const answer = Promise.withResolvers<string>();
  const events: RunlingEvent[] = [];
  ctx.onInput = ({ signal }) => {
    expect(signal).toBe(ctx.signal);
    started.resolve();
    return answer.promise;
  };
  await observeRunlingEvents(event => events.push(event), async () => {
    const pending = input(ctx, "Wait");
    await started.promise;
    try { ctx.abort("Stop waiting"); } catch {}
    await expect(pending).rejects.toBe(ctx.signal.reason);
    answer.resolve("too late");
    await Promise.resolve();
  });
  expect(events.filter(e => e.type === "input.finished").map(e => e.status)).toEqual(["failed"]);
});

test("cleans cancellation listeners and consumes late handler rejections", async () => {
  const controller = new AbortController();
  const remove = vi.spyOn(controller.signal, "removeEventListener");
  const started = Promise.withResolvers<void>();
  const answer = Promise.withResolvers<string>();
  const ask = createInput(() => { started.resolve(); return answer.promise; });
  const pending = ask("Wait", { signal: controller.signal });
  await started.promise;
  controller.abort(new Error("Cancelled"));
  await expect(pending).rejects.toThrow("Cancelled");
  expect(remove).toHaveBeenCalledWith("abort", expect.any(Function));
  answer.reject(new Error("late handler failure"));
  await Promise.resolve();
});

test("per-question cancellation does not cancel the workflow", async () => {
  const ctx = createWorkflowContext();
  const controller = new AbortController();
  const started = Promise.withResolvers<void>();
  ctx.onInput = () => { started.resolve(); return new Promise(() => {}); };
  const pending = input(ctx, "Wait", { signal: controller.signal });
  await started.promise;
  controller.abort(new Error("Question cancelled"));
  await expect(pending).rejects.toThrow("Question cancelled");
  expect(ctx.signal.aborted).toBe(false);
  ctx.onInput = async () => "next answer";
  expect(await input(ctx, "Next")).toBe("next answer");
});

test("does not invoke a handler on an already aborted workflow", async () => {
  const ctx = createWorkflowContext();
  ctx.onInput = vi.fn(async () => "unexpected");
  try { ctx.abort("Stopped"); } catch {}
  await expect(input(ctx, "Question")).rejects.toBe(ctx.signal.reason);
  expect(ctx.onInput).not.toHaveBeenCalled();
});
