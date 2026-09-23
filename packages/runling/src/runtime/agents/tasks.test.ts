import { expect, test, vi } from "vitest";
import { createWorkflowContext, type WorkflowContext } from "../context.ts";
import { agentTasksExtension, createAgentTasks, type AgentTaskUpdate } from "./tasks.ts";
import type { AgentExtensionAPI } from "../agent.ts";
import { observeRunlingEvents, type RunlingEvent } from "../events.ts";

test("agent supervision observes an ordinary run without replacing its identity or typed result", async () => {
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  const run = ctx.spawn(async (child: WorkflowContext<string, AgentTaskUpdate>) => {
    const input = await child.inbox[Symbol.asyncIterator]().next();
    await child.emit({ type: "state", value: { phase: "done" } });
    await child.emit({ type: "output", text: "Checked the source" });
    return { answer: input.value };
  });
  const snapshot = tasks.observe("Worker", run);
  expect(snapshot.id).toBe(run.id);
  expect(tasks.observe("Worker", run).id).toBe(run.id);
  const reader = tasks.notifications[Symbol.asyncIterator]();
  await tasks.send(run.id, "Steering");
  expect(await run.result).toEqual({ answer: "Steering" });
  const notice = JSON.parse((await reader.next()).value!);
  expect(notice).toMatchObject({ type: "task.completed", task: { id: run.id, state: { phase: "done" }, result: '{"answer":"Steering"}' } });
  expect(tasks.get(run.id).output[0]?.text).toBe("Checked the source");
  expect(tasks.get(run.id).status).toBe(run.status);
  await tasks.dispose();
});

test("explicit host activity reaches operational events without copying task state or waking the owner", async () => {
  const events: RunlingEvent[] = [];
  await observeRunlingEvents(event => events.push(event), async () => {
    const ctx = createWorkflowContext();
    const tasks = createAgentTasks(ctx, { notifyActivity: false });
    const finish = Promise.withResolvers<void>();
    const run = ctx.spawn(async (ctx: WorkflowContext<string, AgentTaskUpdate>) => {
      await ctx.emit({ type: "state", value: { phase: "validating", private: "secret" }, activity: "Validating change", activityLevel: "success" });
      await finish.promise;
    });
    tasks.observe("Worker", run);
    const next = vi.fn();
    const reader = tasks.notifications[Symbol.asyncIterator]();
    const notice = reader.next().then(next);
    try {
      await vi.waitFor(() => expect(events.some(event => event.type === "task.activity")).toBe(true));
      expect(events.filter(event => event.type === "task.activity")).toEqual([
        expect.objectContaining({ channelId: run.id, message: "Validating change", level: "success" }),
      ]);
      expect(JSON.stringify(events.filter(event => event.type === "task.activity"))).not.toContain("secret");
      expect(next).not.toHaveBeenCalled();
      finish.resolve();
      await notice;
      expect(JSON.parse(next.mock.calls[0]![0].value).type).toBe("task.completed");
    } finally { finish.resolve(); await tasks.dispose(); }
  });
});

test("legacy task starts keep public activity severity", async () => {
  const events: RunlingEvent[] = [];
  await observeRunlingEvents(event => events.push(event), async () => {
    const tasks = createAgentTasks(createWorkflowContext());
    try {
      tasks.start("Worker", async ctx => {
        await ctx.emit({ type: "state", value: { phase: "done" }, activity: "Check passed", activityLevel: "success" });
        return "done";
      }, undefined);
      await vi.waitFor(() => expect(events.some(event => event.type === "task.activity")).toBe(true));
      expect(events.find(event => event.type === "task.activity")).toMatchObject({ message: "Check passed", level: "success" });
    } finally { await tasks.dispose(); }
  });
});

test("cancelling an observed run directly reports cancellation and awaits its cleanup", async () => {
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  const cleanup = Promise.withResolvers<void>();
  const run = ctx.spawn(async (child: WorkflowContext<string, AgentTaskUpdate>) => {
    try { await child.inbox[Symbol.asyncIterator]().next(); }
    finally { await cleanup.promise; }
    return "done";
  });
  tasks.observe("Worker", run);
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const notified = vi.fn();
  const notice = reader.next().then(value => { notified(); return value; });
  run.cancel();
  await expect(run.result).rejects.toThrow("Task cancelled");
  expect(notified).not.toHaveBeenCalled();
  cleanup.resolve();
  expect(JSON.parse((await notice).value!).type).toBe("task.cancelled");
  await tasks.dispose();
});

test("task context keeps bounded output and fresh state without waking the owner", async () => {
  const tasks = createAgentTasks(createWorkflowContext(), { notifyActivity: false });
  const ready = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<string>();
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const notification = vi.fn();
  const notice = reader.next().then(value => { notification(); return value; });
  const handle = tasks.start("Worker", async ctx => {
    const value = { phase: "setup", pending: ["check", "test"] };
    await ctx.emit({ type: "state", value });
    value.pending.push("mutated");
    for (let i = 0; i < 20; i++) await ctx.emit({ type: "output", text: `${i}:` + "x".repeat(5000) });
    await ctx.emit({ type: "state", value: { phase: "validating", completed: ["check"], pending: ["test"] } });
    ready.resolve();
    return finish.promise;
  }, undefined);
  try {
    await ready.promise;
    await vi.waitFor(() => expect(tasks.get(handle.id).state?.phase).toBe("validating"));
    expect(notification).not.toHaveBeenCalled();
    const copy = tasks.get(handle.id);
    expect(copy.output).toHaveLength(16);
    expect(copy.droppedOutput).toBe(4);
    expect(copy.output[0]).toMatchObject({ sequence: 5, kind: "output", text: expect.stringMatching(/^4:/) });
    expect(copy.output.every(item => item.text.length === 4000)).toBe(true);
    expect(copy.output.every(item => item.truncated)).toBe(true);
    (copy.state!.pending as string[]).push("wrong");
    copy.output[0]!.text = "wrong";
    expect(tasks.get(handle.id).state?.pending).toEqual(["test"]);
    expect(tasks.get(handle.id).output[0]!.text).not.toBe("wrong");
    finish.resolve("Final result");
    const event = JSON.parse((await notice).value!);
    expect(event).toMatchObject({ type: "task.completed", task: { result: "Final result" } });
    expect(event.task.output).toBeUndefined();
    expect(tasks.get(handle.id).output).toHaveLength(16);
  } finally { finish.resolve("done"); await tasks.dispose(); }
});

test("an explicit reply wakes the owner without waiting for the progress interval", async () => {
  const tasks = createAgentTasks(createWorkflowContext(), { progressIntervalMs: 120_000, notifyActivity: false });
  const finish = Promise.withResolvers<string>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit({ type: "reply", text: "A diff viewer and a test runner would help." });
    return finish.promise;
  }, undefined);
  const reader = tasks.notifications[Symbol.asyncIterator]();
  try {
    const notice = JSON.parse((await reader.next()).value!);
    expect(notice).toMatchObject({ type: "task.reply", task: { id: handle.id, status: "running" } });
    expect(tasks.get(handle.id).output.at(-1)).toMatchObject({ kind: "reply", text: "A diff viewer and a test runner would help." });
  } finally { finish.resolve("done"); await tasks.dispose(); }
});

test("a pending reply is not replaced by later tool failure activity", async () => {
  const tasks = createAgentTasks(createWorkflowContext(), { toolFailureNoticeThreshold: 1 });
  const finish = Promise.withResolvers<string>();
  const emitted = Promise.withResolvers<void>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit({ type: "reply", text: "Use the test runner.", replyTo: "question-1" });
    await ctx.emit({ type: "tool", operation: "other", phase: "failed", failures: 1, toolName: "runCheck", error: "unknown" });
    emitted.resolve();
    return finish.promise;
  }, undefined);
  try {
    await emitted.promise;
    await vi.waitFor(() => expect(tasks.get(handle.id).lastToolFailure).toBeDefined());
    const notice = JSON.parse((await tasks.notifications[Symbol.asyncIterator]().next()).value!);
    expect(notice.type).toBe("task.reply");
  } finally { finish.resolve("done"); await tasks.dispose(); }
});

test("oversized state fails the producer safely and task contexts stay isolated", async () => {
  const tasks = createAgentTasks(createWorkflowContext());
  const reader = tasks.notifications[Symbol.asyncIterator]();
  tasks.start("Invalid", async ctx => {
    await ctx.emit({ type: "state", value: { text: "x".repeat(16001) } });
    return "unexpected";
  }, undefined);
  expect(JSON.parse((await reader.next()).value!)).toMatchObject({ type: "task.failed" });
  const other = tasks.start("Other", async ctx => {
    await ctx.emit({ type: "output", text: "Own output" });
    return "done";
  }, undefined);
  await reader.next();
  expect(tasks.get(other.id).output.map(item => item.text)).toEqual(["Own output"]);
  expect(tasks.list()[0]?.output).toEqual([]);
  await tasks.dispose();
});

test("task tools cannot poll status", async () => {
  const tasks = createAgentTasks(createWorkflowContext());
  const names: string[] = [];
  const extension = agentTasksExtension(tasks);
  const factory = typeof extension === "function" ? extension : extension.factory;
  await factory({ registerTool: tool => { names.push(tool.name); } } as AgentExtensionAPI);
  expect(names).toEqual(["task_send", "task_cancel"]);
  await tasks.dispose();
});

test("provider trouble bypasses progress throttle and snapshots retain the age of findings", async () => {
  vi.useFakeTimers();
  const tasks = createAgentTasks(createWorkflowContext());
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const gate = Promise.withResolvers<string>();
  const started = Promise.withResolvers<void>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit("Found the relevant handler");
    await ctx.emit({ type: "retrying", attempt: 1, maxAttempts: 3, delayMs: 2000 });
    started.resolve();
    return gate.promise;
  }, undefined);
  try {
    await started.promise;
    expect(JSON.parse((await reader.next()).value!)).toMatchObject({ type: "task.retrying", task: {
      progress: "Found the relevant handler", provider: { type: "retrying", attempt: 1 },
    } });
    await vi.advanceTimersByTimeAsync(5000);
    expect(tasks.get(handle.id).progressAgeMs).toBe(5000);
    expect(tasks.list()).toEqual([tasks.get(handle.id)]);
    const copy = tasks.get(handle.id);
    copy.provider = { type: "working" };
    expect(tasks.get(handle.id).provider?.type).toBe("retrying");
  } finally { gate.resolve("done"); await tasks.dispose(); vi.useRealTimers(); }
});

test("returns handles immediately, forwards input, and reports completion", async () => {
  const tasks = createAgentTasks(createWorkflowContext());
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const handle = tasks.start("Worker", async ctx => {
    const message = await ctx.inbox[Symbol.asyncIterator]().next();
    return `Received: ${message.value}`;
  }, undefined);
  expect(handle.status).toBe("running");
  await tasks.send(handle.id, "A clarification");
  const event = JSON.parse((await reader.next()).value!);
  expect(event).toMatchObject({ type: "task.completed", task: { id: handle.id, result: "Received: A clarification" } });
  expect(tasks.active).toBe(false);
  await expect(tasks.send(handle.id, "late")).rejects.toThrow("not running");
  await tasks.dispose();
});

test("tool failures supersede stale prose and stop the child after the configured limit", async () => {
  const tasks = createAgentTasks(createWorkflowContext(), { maxToolFailures: 3, toolFailureNoticeThreshold: 1 });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const cleanup = vi.fn();
  const nextFailure = Promise.withResolvers<void>();
  const handle = tasks.start("Worker", async ctx => {
    try {
      await ctx.emit("I will start finding files");
      await ctx.emit({ type: "tool", operation: "edit", phase: "failed", failures: 1 });
      await nextFailure.promise;
      await ctx.emit({ type: "tool", operation: "edit", phase: "failed", failures: 3 });
      await new Promise<void>(resolve => {
        if (ctx.signal.aborted) resolve();
        else ctx.signal.addEventListener("abort", () => resolve(), { once: true });
      });
      ctx.signal.throwIfAborted();
      return "unexpected";
    } finally { cleanup(); }
  }, undefined);
  try {
    const first = JSON.parse((await reader.next()).value!);
    expect(first).toMatchObject({ type: "task.tool_failed", task: { activity: { operation: "edit", phase: "failed" } } });
    expect(first.task.progress).toBeUndefined();
    expect(tasks.get(handle.id).activityAgeMs).toBeGreaterThanOrEqual(0);
    nextFailure.resolve();
    expect(JSON.parse((await reader.next()).value!)).toMatchObject({ type: "task.failed", task: { failureReason: "tool_failure_limit" } });
    expect(cleanup).toHaveBeenCalledOnce();
  } finally { nextFailure.resolve(); await tasks.dispose(); }
});

test("an isolated recovered lookup failure stays in snapshots without notifying the owner", async () => {
  const tasks = createAgentTasks(createWorkflowContext(), { notifyActivity: false });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const failed = Promise.withResolvers<void>();
  const recover = Promise.withResolvers<void>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit({ type: "tool", operation: "read", phase: "failed", failures: 1, error: "not_found" });
    failed.resolve();
    await recover.promise;
    await ctx.emit({ type: "tool", operation: "read", phase: "succeeded", failures: 0 });
    return "Found the correct module";
  }, undefined);
  try {
    await failed.promise;
    await vi.waitFor(() => expect(tasks.get(handle.id).lastToolFailure?.error).toBe("not_found"));
    recover.resolve();
    expect(JSON.parse((await reader.next()).value!).type).toBe("task.completed");
    expect(tasks.get(handle.id).lastToolFailure).toBeUndefined();
  } finally { recover.resolve(); await tasks.dispose(); }
});

test("provider recovery and repeated retries do not wake the owner with stale findings", async () => {
  const tasks = createAgentTasks(createWorkflowContext());
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const retry = Promise.withResolvers<void>();
  const settled = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<string>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit({ type: "retrying", attempt: 1, maxAttempts: 3, delayMs: 2000 });
    await retry.promise;
    await ctx.emit({ type: "retrying", attempt: 2, maxAttempts: 3, delayMs: 4000 });
    await ctx.emit({ type: "working" });
    settled.resolve();
    return finish.promise;
  }, undefined);
  try {
    expect(JSON.parse((await reader.next()).value!).type).toBe("task.retrying");
    retry.resolve();
    await settled.promise;
    await vi.waitFor(() => expect(tasks.get(handle.id).provider).toEqual({ type: "working" }), { interval: 1 });
    const next = reader.next();
    finish.resolve("Done");
    expect(JSON.parse((await next).value!).type).toBe("task.completed");
  } finally { retry.resolve(); finish.resolve("Done"); await tasks.dispose(); }
});

test("routine tool activity is coalesced and cannot replay an old intention", async () => {
  vi.useFakeTimers();
  const tasks = createAgentTasks(createWorkflowContext(), { progressIntervalMs: 100 });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const finish = Promise.withResolvers<string>();
  const started = Promise.withResolvers<void>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit("I will start looking");
    for (let i = 0; i < 20; i++) {
      await ctx.emit({ type: "tool", operation: "read", phase: "started", failures: 0 });
      await ctx.emit({ type: "tool", operation: "read", phase: "succeeded", failures: 0 });
    }
    started.resolve();
    return finish.promise;
  }, undefined);
  try {
    await started.promise;
    await vi.advanceTimersByTimeAsync(100);
    const event = JSON.parse((await reader.next()).value!);
    expect(event.type).toBe("task.activity");
    expect(event.task.activity).toMatchObject({ operation: "read", phase: "succeeded" });
    expect(JSON.stringify(event)).not.toContain("I will start");
    expect(tasks.get(handle.id).progress).toBeUndefined();
    finish.resolve("Done");
    expect(JSON.parse((await reader.next()).value!).type).toBe("task.completed");
  } finally { finish.resolve("Done"); await tasks.dispose(); vi.useRealTimers(); }
});

test("quiet activity updates snapshots without waking the owner, while failures still notify", async () => {
  vi.useFakeTimers();
  const tasks = createAgentTasks(createWorkflowContext(), { progressIntervalMs: 100, notifyActivity: false });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const fail = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<string>();
  const started = Promise.withResolvers<void>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit({ type: "tool", operation: "read", phase: "succeeded", failures: 0 });
    started.resolve();
    await fail.promise;
    await ctx.emit({ type: "tool", operation: "read", phase: "failed", failures: 2 });
    return finish.promise;
  }, undefined);
  try {
    await started.promise;
    await vi.advanceTimersByTimeAsync(100);
    expect(tasks.get(handle.id).activity?.phase).toBe("succeeded");
    const next = reader.next();
    fail.resolve();
    expect(JSON.parse((await next).value!).type).toBe("task.tool_failed");
  } finally { fail.resolve(); finish.resolve("Done"); await tasks.dispose(); vi.useRealTimers(); }
});

test("explicit findings survive tool activity and are announced only once", async () => {
  vi.useFakeTimers();
  const tasks = createAgentTasks(createWorkflowContext(), { progressIntervalMs: 100, notifyActivity: false });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const nextTool = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<string>();
  const handle = tasks.start("Worker", async ctx => {
    await ctx.emit({ type: "finding", text: "Checked source evidence" });
    await ctx.emit({ type: "tool", operation: "other", phase: "succeeded", failures: 0 });
    await nextTool.promise;
    await ctx.emit({ type: "tool", operation: "read", phase: "succeeded", failures: 0 });
    return finish.promise;
  }, undefined);
  try {
    await vi.advanceTimersByTimeAsync(100);
    expect(JSON.parse((await reader.next()).value!)).toMatchObject({ type: "task.progress", progress: "Checked source evidence" });
    nextTool.resolve();
    await vi.advanceTimersByTimeAsync(100);
    expect(tasks.get(handle.id).progress).toBe("Checked source evidence");
    const next = reader.next();
    finish.resolve("Done");
    expect(JSON.parse((await next).value!).type).toBe("task.completed");
  } finally { nextTool.resolve(); finish.resolve("Done"); await tasks.dispose(); vi.useRealTimers(); }
});

test("coalesces progress and completion replaces stale progress", async () => {
  vi.useFakeTimers();
  const tasks = createAgentTasks(createWorkflowContext(), { progressIntervalMs: 100 });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const gate = Promise.withResolvers<string>();
  const started = Promise.withResolvers<void>();
  tasks.start("Worker", async ctx => {
    await ctx.emit("first");
    await ctx.emit("latest");
    started.resolve();
    return gate.promise;
  }, undefined);
  try {
    await started.promise;
    await vi.advanceTimersByTimeAsync(100);
    expect(JSON.parse((await reader.next()).value!).progress).toBe("latest");
    gate.resolve("Final evidence");
    expect(JSON.parse((await reader.next()).value!)).toMatchObject({ type: "task.completed", task: { result: "Final evidence" } });
  } finally { gate.resolve("done"); await tasks.dispose(); vi.useRealTimers(); }
});

test("disposal waits for cooperative cleanup and cancels all tasks", async () => {
  const tasks = createAgentTasks(createWorkflowContext());
  const entered = Promise.withResolvers<void>();
  const cleanup = Promise.withResolvers<void>();
  const aborted = Promise.withResolvers<void>();
  tasks.start("Worker", async ctx => {
    entered.resolve();
    await new Promise<void>(resolve => ctx.signal.addEventListener("abort", () => { aborted.resolve(); resolve(); }, { once: true }));
    await cleanup.promise;
    return "done";
  }, undefined);
  await entered.promise;
  let closed = false;
  const closing = tasks.dispose().then(() => { closed = true; });
  await aborted.promise;
  expect(closed).toBe(false);
  cleanup.resolve();
  await closing;
  expect(closed).toBe(true);
  expect(() => tasks.start("late", async () => "", undefined)).toThrow("closed");
});

test("isolates failures and cancellation, hides raw errors, and bounds handles", async () => {
  const tasks = createAgentTasks(createWorkflowContext(), { maxTasks: 2 });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const failed = tasks.start("Broken", async () => { throw new Error("secret remote error"); }, undefined);
  const event = (await reader.next()).value!;
  expect(JSON.parse(event).type).toBe("task.failed");
  expect(event).not.toContain("secret");
  expect(tasks.get(failed.id).status).toBe("failed");
  const running = tasks.start("Other", async ctx => {
    await ctx.inbox[Symbol.asyncIterator]().next();
    return "done";
  }, undefined);
  tasks.cancel(running.id);
  expect(JSON.parse((await reader.next()).value!).type).toBe("task.cancelled");
  expect(() => tasks.start("overflow", async () => "", undefined)).toThrow("limit");
  expect(() => tasks.get("foreign-handle")).toThrow("Unknown");
  await tasks.dispose();
});

test("parent cancellation releases a waiting notification reader and child resources", async () => {
  const controller = new AbortController();
  const tasks = createAgentTasks({ ...createWorkflowContext(), signal: controller.signal });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const started = Promise.withResolvers<void>();
  const cleanup = vi.fn();
  tasks.start("Worker", async ctx => {
    started.resolve();
    try { await ctx.inbox[Symbol.asyncIterator]().next(); }
    finally { cleanup(); }
    return "done";
  }, undefined);
  await started.promise;
  const next = reader.next();
  controller.abort();
  await tasks.dispose();
  expect(await next).toEqual({ done: true, value: undefined });
  expect(cleanup).toHaveBeenCalledOnce();
});
