import { afterEach, expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage, type AgentResult } from "runling";
import { createChattoPlanDemo } from "./chatto-plan-demo.ts";
import type { ChattoTyping } from "./chatto/typing.ts";
import type { ChattoPost } from "./chatto/webhook.ts";

const cancellation = (error: unknown) => {
  if (error instanceof Error && error.message === "Cancelled from Chatto") return { status: "cancelled" };
  throw error;
};
const report = (outcome: AgentResult["outcome"], summary: string, details?: string): AgentResult =>
  ({ outcome, summary, details, usage: emptyTokenUsage() });
const delivery = (body: string, id: string, thread: string | null = null, author = "alice") => ({
  version: 1 as const, id, type: "message.created" as const, triggers: ["direct_message"],
  occurred_at: "now", bot_id: "bot", room_id: "dm", thread_root_id: thread,
  message: { id, body, author_id: author },
});
function setup(reports: AgentResult[], options: { timeout?: number; typing?: ChattoTyping } = {}) {
  const post = vi.fn<ChattoPost>(async () => {});
  const planner = { runOutcome: vi.fn(async () => {
    const next = reports.shift();
    if (!next) throw new Error("No prepared report");
    return next;
  }), steer: vi.fn(async (_text: string) => false), dispose: vi.fn() };
  const refresh = vi.fn(async () => "abc123");
  const createPlanner = vi.fn(async () => planner);
  const bot = createChattoPlanDemo({ directory: process.cwd(), post, refresh, createPlanner, ...options });
  let sequence = 0;
  const answer = (body: string, root = "root", author = "alice") =>
    bot(createWorkflowContext(), delivery(body, `answer-${sequence++}`, root, author));
  const waitQuestion = (text: string) => vi.waitFor(() => expect(post.mock.calls.some(call => call[1].includes(text))).toBe(true));
  return { bot, post, planner, refresh, createPlanner, answer, waitQuestion };
}
afterEach(() => vi.useRealTimers());

test("interviews, revises a plan, and enters the stub only on an exact command", async () => {
  const f = setup([
    report("blocked", "Which users need this?"),
    report("completed", "First plan", "Plan version one"),
    report("completed", "Revised plan", "Plan version two"),
  ]);
  const run = f.bot(createWorkflowContext(), delivery("Add a useful feature", "root"));
  await f.waitQuestion("Which users");
  expect(f.refresh).toHaveBeenCalledTimes(1);
  expect(await f.answer("wrong user", "root", "bob")).toBe("ignored");
  await f.answer("/implement");
  await f.waitQuestion("no completed plan");
  await vi.waitFor(() => expect(f.post.mock.calls.filter(call => call[1].includes("Which users"))).toHaveLength(2));
  expect(f.planner.runOutcome).toHaveBeenCalledTimes(1);
  await f.answer("All users");
  await f.waitQuestion("Plan version one");
  await f.answer("Please /implement eventually, but first add tests");
  await f.waitQuestion("Plan version two");
  expect(f.planner.runOutcome).toHaveBeenCalledTimes(3);
  await f.answer("/implement");
  expect(await run).toEqual({ status: "implementation-stub", plan: "Plan version two", revision: "abc123" });
  expect(f.planner.dispose).toHaveBeenCalledOnce();
  expect(f.post.mock.calls.at(-1)![1]).toContain("demo stub");
  for (const [destination] of f.post.mock.calls) expect(destination).toEqual({ roomId: "dm", threadRootId: "root" });
});

test("holds the checkout across input waits and releases it on cancellation", async () => {
  const f = setup([report("blocked", "Question one?"), report("blocked", "Question two?")]);
  const run = f.bot(createWorkflowContext(), delivery("Feature", "root")).catch(cancellation);
  await f.waitQuestion("Question one");
  // Config reload must not let a new factory change an active planner's checkout.
  vi.resetModules();
  const { createChattoPlanDemo: reloaded } = await import("./chatto-plan-demo.ts");
  const other = reloaded({ directory: process.cwd(), post: f.post, refresh: f.refresh, createPlanner: f.createPlanner });
  expect(await other(createWorkflowContext(), delivery("Another feature", "other"))).toMatchObject({ status: "busy" });
  expect(f.refresh).toHaveBeenCalledTimes(1);
  await f.answer("/cancel");
  expect(await run).toMatchObject({ status: "cancelled" });
  const next = f.bot(createWorkflowContext(), delivery("Try again", "next")).catch(cancellation);
  await f.waitQuestion("Question two");
  await f.answer("/cancel", "next");
  await next;
  expect(f.refresh).toHaveBeenCalledTimes(2);
});

test("fails visibly and releases the checkout when git or planning fails", async () => {
  const f = setup([report("failed", "Cannot plan"), report("blocked", "Try again?")]);
  f.refresh.mockRejectedValueOnce(new Error("dirty checkout"));
  await expect(f.bot(createWorkflowContext(), delivery("Feature", "git-fail"))).rejects.toThrow("dirty checkout");
  expect(f.createPlanner).not.toHaveBeenCalled();
  await expect(f.bot(createWorkflowContext(), delivery("Feature", "agent-fail"))).rejects.toThrow("Cannot plan");
  expect(f.planner.dispose).toHaveBeenCalledOnce();
  const next = f.bot(createWorkflowContext(), delivery("Feature", "root")).catch(cancellation);
  await f.waitQuestion("Try again");
  await f.answer("/cancel");
  await next;
});

test("times out an interview, disposes the planner, and permits a new conversation", async () => {
  vi.useFakeTimers();
  const f = setup([report("blocked", "Question?"), report("blocked", "Again?")], { timeout: 1 });
  const run = f.bot(createWorkflowContext(), delivery("Feature", "root")).catch(cancellation);
  const rejected = expect(run).rejects.toThrow("Input timed out");
  await vi.waitFor(() => expect(f.post.mock.calls.some(call => call[1].includes("Question?"))).toBe(true));
  await vi.advanceTimersByTimeAsync(1000);
  await rejected;
  expect(f.planner.dispose).toHaveBeenCalledOnce();
  expect(f.post.mock.calls.at(-1)![1]).toContain("timed out");
  vi.useRealTimers();
  const next = f.bot(createWorkflowContext(), delivery("Feature", "next")).catch(cancellation);
  await f.waitQuestion("Again?");
  await f.answer("/cancel", "next");
  await next;
});


test("keeps typing through an agent turn and stops at the input prompt", async () => {
  vi.useFakeTimers();
  const typing = vi.fn(async () => {});
  const f = setup([], { typing });
  const turn = Promise.withResolvers<AgentResult>();
  f.planner.runOutcome.mockImplementationOnce(() => turn.promise);
  const run = f.bot(createWorkflowContext(), delivery("Feature", "root")).catch(cancellation);
  await vi.waitFor(() => expect(f.planner.runOutcome).toHaveBeenCalledOnce());
  const before = typing.mock.calls.length;
  await vi.advanceTimersByTimeAsync(3000);
  expect(typing.mock.calls.length).toBeGreaterThan(before);
  turn.resolve(report("blocked", "Who needs this?"));
  await f.waitQuestion("Who needs this?");
  const atQuestion = typing.mock.calls.length;
  await vi.advanceTimersByTimeAsync(6000);
  expect(typing.mock.calls).toHaveLength(atQuestion);
  await f.answer("/cancel");
  await run;
});


test("incorporates busy messages before publishing a plan and never queues approval", async () => {
  const f = setup([report("completed", "Updated", "Updated plan")]);
  const turn = Promise.withResolvers<AgentResult>();
  f.planner.runOutcome.mockImplementationOnce(() => turn.promise);
  const run = f.bot(createWorkflowContext(), delivery("Feature", "root"));
  await vi.waitFor(() => expect(f.planner.runOutcome).toHaveBeenCalledOnce());
  expect(await f.answer("Include tests")).toBe("queued");
  expect(await f.answer("/implement")).toBe("queued");
  turn.resolve(report("completed", "Stale", "Stale plan"));
  await f.waitQuestion("Updated plan");
  expect(f.post.mock.calls.some(call => call[1].includes("Stale plan"))).toBe(false);
  expect(f.planner.runOutcome.mock.calls[1]).toEqual(expect.arrayContaining([expect.stringContaining("Include tests")]));
  expect(f.post.mock.calls.some(call => call[1].includes("not saved as approval"))).toBe(true);
  await f.answer("/implement");
  expect(await run).toMatchObject({ status: "implementation-stub", plan: "Updated plan" });
});

test("cancel interrupts an active agent turn and releases the checkout", async () => {
  const f = setup([]);
  f.planner.runOutcome.mockImplementationOnce(async (...args: unknown[]) => {
    const ctx = args[0] as ReturnType<typeof createWorkflowContext>;
    return new Promise<AgentResult>((_resolve, reject) => {
      ctx.signal.addEventListener("abort", () => reject(ctx.signal.reason), { once: true });
    });
  });
  const ctx = createWorkflowContext();
  const run = f.bot(ctx, delivery("Feature", "root"));
  const rejected = expect(run).rejects.toThrow("Cancelled from Chatto");
  await vi.waitFor(() => expect(f.planner.runOutcome).toHaveBeenCalledOnce());
  await f.answer("/cancel");
  await rejected;
  expect(ctx.signal.aborted).toBe(true);
  expect(f.planner.dispose).toHaveBeenCalledOnce();
});


test("includes feedback received while sending a rejected-approval notice", async () => {
  const f = setup([report("completed", "Updated", "Updated after notice")]);
  const turn = Promise.withResolvers<AgentResult>();
  const notice = Promise.withResolvers<void>();
  f.planner.runOutcome.mockImplementationOnce(() => turn.promise);
  f.post.mockImplementation(async (_destination, body) => {
    if (body.includes("not saved as approval")) await notice.promise;
  });
  const run = f.bot(createWorkflowContext(), delivery("Feature", "root"));
  await vi.waitFor(() => expect(f.planner.runOutcome).toHaveBeenCalledOnce());
  await f.answer("/implement");
  turn.resolve(report("completed", "Stale", "Stale before notice"));
  await f.waitQuestion("not saved as approval");
  await f.answer("Use the existing API");
  notice.resolve();
  await f.waitQuestion("Updated after notice");
  expect(f.post.mock.calls.some(call => call[1].includes("Stale before notice"))).toBe(false);
  expect(f.planner.runOutcome.mock.calls[1]).toEqual(expect.arrayContaining([expect.stringContaining("Use the existing API")]));
  await f.answer("/implement");
  await run;
});


test("forwards busy feedback before the active interaction completes", async () => {
  const f = setup([]);
  const turn = Promise.withResolvers<AgentResult>();
  f.planner.runOutcome.mockImplementationOnce(() => turn.promise);
  f.planner.steer.mockResolvedValue(true);
  const run = f.bot(createWorkflowContext(), delivery("Feature", "root"));
  await vi.waitFor(() => expect(f.planner.runOutcome).toHaveBeenCalledOnce());
  await f.answer("Include accessibility tests");
  await vi.waitFor(() => expect(f.planner.steer).toHaveBeenCalledWith("Include accessibility tests"));
  expect(f.planner.runOutcome).toHaveBeenCalledOnce();
  await f.answer("/implement");
  expect(f.planner.steer).toHaveBeenCalledOnce();
  turn.resolve(report("completed", "Ready", "Plan with accessibility tests"));
  await f.waitQuestion("Plan with accessibility tests");
  expect(f.planner.runOutcome).toHaveBeenCalledOnce();
  await f.answer("/implement");
  expect(await run).toMatchObject({ status: "implementation-stub" });
});

test("posts mid-interaction text immediately and finishes posts before asking a question", async () => {
  const f = setup([]);
  const turn = Promise.withResolvers<AgentResult>();
  const sent = Promise.withResolvers<void>();
  let onText!: (text: string) => void;
  f.planner.runOutcome.mockImplementationOnce(async (...args: unknown[]) => {
    onText = (args[2] as { onText: (text: string) => void }).onText;
    return turn.promise;
  });
  f.post.mockImplementation(async (_destination, body) => {
    if (body === "Here is the joke.") await sent.promise;
  });
  const run = f.bot(createWorkflowContext(), delivery("Feature", "root"));
  await vi.waitFor(() => expect(f.planner.runOutcome).toHaveBeenCalledOnce());
  onText("Here is the joke.");
  onText("I am still investigating.");
  await f.waitQuestion("Here is the joke.");
  expect(f.post.mock.calls.some(call => call[1] === "I am still investigating.")).toBe(false);
  turn.resolve(report("completed", "Ready", "The final plan"));
  await new Promise(resolve => setTimeout(resolve, 10));
  expect(f.post.mock.calls.some(call => call[1].includes("The final plan"))).toBe(false);
  sent.resolve();
  await f.waitQuestion("The final plan");
  const bodies = f.post.mock.calls.map(call => call[1]);
  expect(bodies.indexOf("I am still investigating.")).toBeGreaterThan(bodies.indexOf("Here is the joke."));
  await f.answer("/implement");
  await run;
});

test("reports a failed intermediate post without presenting the final question", async () => {
  const f = setup([]);
  f.planner.runOutcome.mockImplementationOnce(async (...args: unknown[]) => {
    (args[2] as { onText: (text: string) => void }).onText("Intermediate reply");
    return report("completed", "Ready", "Unsent final plan");
  });
  f.post.mockImplementation(async (_destination, body) => {
    if (body === "Intermediate reply") throw new Error("Chatto offline");
  });
  await expect(f.bot(createWorkflowContext(), delivery("Feature", "root"))).rejects.toThrow("Chatto offline");
  expect(f.post.mock.calls.some(call => call[1].includes("Unsent final plan"))).toBe(false);
  expect(f.planner.dispose).toHaveBeenCalledOnce();
});
