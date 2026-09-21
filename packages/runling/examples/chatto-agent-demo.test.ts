import webFetchExtension from "../extensions/web-fetch.ts";
import { expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage } from "runling";
import type { WebhookContext } from "runling/web";
import type { AgentResult, AgentRunOptions } from "runling/agents";
import { conversation } from "./chatto-agent-demo.ts";
import { chattoConversation } from "./chatto/chat-conversation.ts";
import type { createWebChatAgent } from "./chatto/web-chat-agent.ts";
import type { ChattoPost } from "./chatto/webhook.ts";
import type { ChattoTyping } from "./chatto/typing.ts";

// Test transports and agent setup stay outside the tutorial's task definition.
function createChattoAgentDemo({ post, typing = async () => {}, ...settings }:
  Parameters<typeof createWebChatAgent>[0] & {
    post: ChattoPost;
    typing?: ChattoTyping;
    timeout?: number;
  },
) {
  return chattoConversation({ name: "Test conversation", task: conversation, settings, post, typing });
}
import type { Delivery } from "./chatto/webhook.ts";

const delivery = (body: string, id = "root", thread: string | null = null): Delivery => ({
  version: 1,
  id,
  type: "message.created",
  triggers: ["direct_message"],
  occurred_at: "2026-09-10T12:00:00Z",
  bot_id: "bot",
  room_id: "dm",
  thread_root_id: thread,
  message: { id, author_id: "alice", body },
});

const report = (summary = "Hello!"): AgentResult => ({
  outcome: "completed", summary, usage: emptyTokenUsage(),
});

function fixture(timeout = 0.03) {
  const turn = Promise.withResolvers<AgentResult>();
  const bot = {
    runOutcome: vi.fn((_ctx, _prompt: string, _options?: AgentRunOptions) => turn.promise),
    steer: vi.fn(async (_text: string) => true),
    dispose: vi.fn(),
  };
  const post = vi.fn(async (_destination, _text: string, _signal: AbortSignal) => {});
  const typing = vi.fn(async () => {});
  const createAgent = vi.fn(async () => ({
    ...bot,
    async runOutcome(ctx: Parameters<typeof bot.runOutcome>[0], prompt: string, options?: AgentRunOptions) {
      const result = await bot.runOutcome(ctx, prompt, options);
      if (result.outcome === "completed") options?.onText?.(result.summary);
      return result;
    },
  }));
  const workflow = createChattoAgentDemo({
    directory: "/explicit", model: "test/model", post, typing, createAgent, timeout,
  });
  const start = vi.fn(async () => ({ id: "unexpected" }));
  const send = (body: string, id: string) => workflow.route({ start }, delivery(body, id, "root"));
  const run = () => workflow(createWorkflowContext(), delivery("Hi"));

  return { turn, bot, post, typing, createAgent, start, send, run };
}

test("runs one web-only agent, sends typing and posts one final thread reply", async () => {
  const f = fixture();
  const run = f.run();
  await vi.waitFor(() => expect(f.bot.runOutcome).toHaveBeenCalledOnce());
  expect(f.typing).toHaveBeenCalled();
  f.turn.resolve(report());

  expect(await run).toEqual({ reply: "Hello!" });
  expect(f.post.mock.calls.map(call => call.slice(0, 2))).toEqual([
    [{ roomId: "dm", threadRootId: "root" }, "Hello!"],
  ]);
  expect(f.createAgent).toHaveBeenCalledWith(expect.objectContaining({
    cwd: "/explicit", model: "test/model", thinkingLevel: "medium", output: "text", tools: ["web_fetch"],
    extensions: [{ name: "runling-web-fetch", factory: webFetchExtension }],
    resources: {
      extensions: false, skills: false, promptTemplates: false,
      themes: false, contextFiles: false,
    },
  }));
  expect(f.bot.dispose).toHaveBeenCalledOnce();
});

test("forwards thread replies while the agent runs without starting stub runs", async () => {
  const f = fixture();
  const run = f.run();
  await vi.waitFor(() => expect(f.bot.runOutcome).toHaveBeenCalledOnce());
  await f.send("Make it short", "steering");
  await vi.waitFor(() => expect(f.bot.steer).toHaveBeenCalledWith("Make it short"));
  f.turn.resolve(report("Short reply"));
  await run;

  expect(f.start).not.toHaveBeenCalled();
  expect(f.bot.runOutcome).toHaveBeenCalledOnce();
});

test("reuses the same agent for messages that steering did not consume", async () => {
  const f = fixture();
  f.bot.steer.mockResolvedValue(false);
  f.bot.runOutcome.mockImplementationOnce(() => f.turn.promise)
    .mockResolvedValueOnce(report("Follow-up reply"));
  const run = f.run();
  await vi.waitFor(() => expect(f.bot.runOutcome).toHaveBeenCalledOnce());
  await f.send("One more thing", "missed");
  await vi.waitFor(() => expect(f.bot.steer).toHaveBeenCalledOnce());
  f.turn.resolve(report());

  expect(await run).toEqual({ reply: "Follow-up reply" });
  expect(f.bot.runOutcome.mock.calls[1]?.[1]).toBe("One more thing");
  expect(f.createAgent).toHaveBeenCalledOnce();
  expect(f.bot.dispose).toHaveBeenCalledOnce();
});

test("processes messages received while posting the final reply", async () => {
  const f = fixture();
  f.bot.steer.mockResolvedValue(false);
  f.bot.runOutcome.mockResolvedValueOnce(report()).mockResolvedValueOnce(report("Second reply"));
  f.post.mockImplementationOnce(async () => {
    await f.send("Another question", "during-post");
  });

  expect(await f.run()).toEqual({ reply: "Second reply" });
  expect(f.bot.runOutcome.mock.calls[1]?.[1]).toBe("Another question");
  expect(f.post).toHaveBeenCalledTimes(2);
});

test("delivers progress before the final reply", async () => {
  const f = fixture();
  f.bot.runOutcome.mockImplementationOnce(async (_ctx, _prompt, options) => {
    options?.onText?.("Working on it.");
    return report("Done.");
  });
  await f.run();

  expect(f.post.mock.calls.map(call => call[1])).toEqual(["Working on it.", "Done."]);
});

test("cancellation stops the run, disposes the agent and posts the adapter notice", async () => {
  const f = fixture();
  const run = f.run();
  const rejected = expect(run).rejects.toThrow("Cancelled from Chatto");
  await vi.waitFor(() => expect(f.bot.runOutcome).toHaveBeenCalledOnce());
  await f.send("/cancel", "cancel");
  await rejected;

  expect(f.bot.runOutcome.mock.calls[0]?.[2]?.signal?.aborted).toBe(true);
  expect(f.bot.dispose).toHaveBeenCalledOnce();
  expect(f.post.mock.calls.map(call => call[1])).toEqual(["Conversation cancelled."]);
  expect(f.bot.steer).not.toHaveBeenCalled();
});

test.each(["agent", "post", "outcome"])("disposes the agent after %s failure", async failure => {
  const f = fixture();
  if (failure === "agent") {
    f.bot.runOutcome.mockRejectedValue(new Error("Agent unavailable"));
  } else if (failure === "post") {
    f.bot.runOutcome.mockResolvedValue(report());
    f.post.mockRejectedValue(new Error("Post unavailable"));
  } else {
    f.bot.runOutcome.mockResolvedValue({ ...report("Model failed"), outcome: "failed" });
  }

  await expect(f.run()).rejects.toThrow(/unavailable|Model failed/);
  // Child cancellation settles the parent before the task finishes cleanup.
  await vi.waitFor(() => expect(f.bot.dispose).toHaveBeenCalledOnce());
});

test("the router starts root DMs once and ignores replies after completion", async () => {
  const f = fixture();
  f.bot.runOutcome.mockResolvedValue(report());
  const workflow = createChattoAgentDemo({
    directory: "/explicit", post: f.post, createAgent: f.createAgent, timeout: 0,
  });
  let running: Promise<unknown> | undefined;
  let starts = 0;
  const start: WebhookContext["start"] = async (root, { input }) => {
    starts++;
    running = Promise.resolve(root(createWorkflowContext(), input));
    return { id: "registered" };
  };
  const root = delivery("Hello");

  await workflow.route({ start }, root);
  await running;
  await workflow.route({ start }, root);
  await workflow.route({ start }, delivery("Too late", "late", "root"));
  expect(starts).toBe(1);
  expect(f.createAgent).toHaveBeenCalledWith(expect.objectContaining({
    model: "openai-codex/gpt-5.6-luna", thinkingLevel: "medium",
  }));
});

test("concurrent conversations use separate agents and retain their thread destinations", async () => {
  const bots = [fixture(), fixture()];
  const createAgent = vi.fn()
    .mockImplementationOnce(() => bots[0]!.createAgent())
    .mockImplementationOnce(() => bots[1]!.createAgent());
  const post = vi.fn(async () => {});
  const workflow = createChattoAgentDemo({ directory: "/explicit", post, createAgent, timeout: 0 });
  const first = workflow(createWorkflowContext(), delivery("First", "first"));
  const second = workflow(createWorkflowContext(), delivery("Second", "second"));
  await vi.waitFor(() => expect(bots[1]!.bot.runOutcome).toHaveBeenCalledOnce());

  await workflow.route({ start: async () => ({ id: "unused" }) }, delivery("For first", "reply", "first"));
  await vi.waitFor(() => expect(bots[0]!.bot.steer).toHaveBeenCalledWith("For first"));
  expect(bots[1]!.bot.steer).not.toHaveBeenCalled();
  bots[0]!.turn.resolve(report("First answer"));
  bots[1]!.turn.resolve(report("Second answer"));
  await Promise.all([first, second]);

  expect(post).toHaveBeenCalledWith({ roomId: "dm", threadRootId: "first" }, "First answer", expect.any(AbortSignal));
  expect(post).toHaveBeenCalledWith({ roomId: "dm", threadRootId: "second" }, "Second answer", expect.any(AbortSignal));
  for (const f of bots) expect(f.bot.dispose).toHaveBeenCalledOnce();
});


test("keeps an idle conversation open for the next reply without another question", async () => {
  const f = fixture(900);
  f.bot.runOutcome.mockResolvedValueOnce(report("Hey! How can I help?"))
    .mockResolvedValueOnce(report("I am TestBot."));
  const run = f.run();
  const cancelled = expect(run).rejects.toThrow("Cancelled from Chatto");
  await vi.waitFor(() => expect(f.post).toHaveBeenCalledOnce());
  await new Promise(resolve => setTimeout(resolve, 20));
  expect(f.bot.dispose).not.toHaveBeenCalled();

  await f.send("What's your name?", "follow-up");
  await vi.waitFor(() => expect(f.post).toHaveBeenCalledTimes(2));
  expect(f.bot.runOutcome.mock.calls[1]?.[1]).toBe("What's your name?");
  expect(f.createAgent).toHaveBeenCalledOnce();
  expect(f.start).not.toHaveBeenCalled();
  expect(f.post.mock.calls.map(call => call[1])).toEqual(["Hey! How can I help?", "I am TestBot."]);

  await f.send("/cancel", "stop-idle");
  await cancelled;
  expect(f.bot.dispose).toHaveBeenCalledOnce();
});


test("posts assistant text without exposing report details", async () => {
  const f = fixture();
  f.bot.runOutcome.mockResolvedValueOnce({
    ...report("A cheesecake joke."),
    details: "Report 1 (earlier findings): An unrelated joke.",
  });

  await f.run();
  expect(f.post.mock.calls.map(call => call[1])).toEqual(["A cheesecake joke."]);
});

test("identical replies in different interactions are still delivered", async () => {
  const f = fixture();
  f.bot.steer.mockResolvedValue(false);
  f.bot.runOutcome.mockResolvedValueOnce(report("Yes."))
    .mockResolvedValueOnce(report("Yes."));
  f.post.mockImplementationOnce(async () => {
    await f.send("Still yes?", "again");
  });

  await f.run();
  expect(f.post.mock.calls.map(call => call[1])).toEqual(["Yes.", "Yes."]);
});

test.each([-1, Infinity, NaN, 2147484])("rejects timeout %s before creating an agent", async timeout => {
  const createAgent = vi.fn();
  const workflow = createChattoAgentDemo({
    directory: "/explicit", post: async () => {}, createAgent, timeout,
  });

  await expect(workflow(createWorkflowContext(), delivery("Hi"))).rejects.toThrow("timeout");
  expect(createAgent).not.toHaveBeenCalled();
});
