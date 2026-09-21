import { expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage } from "runling";
import type { AgentRunOptions } from "runling/agents";
import type { WebhookContext } from "runling/web";
import type { Delivery } from "../chatto/webhook.ts";
import config from "../runling.config.ts";
import { createChattoBot } from "./chat.ts";

const delivery: Delivery = {
  version: 1,
  id: "delivery",
  type: "message.created",
  triggers: ["direct_message"],
  occurred_at: "2026-09-19T12:00:00Z",
  bot_id: "chattobot",
  room_id: "dm",
  thread_root_id: null,
  message: { id: "root", author_id: "alice", body: "Hello!" },
};

test.each([
  { trigger: "direct_message", thread: null },
  { trigger: "direct_message", thread: "existing-thread" },
  { trigger: "mention", thread: null },
  { trigger: "mention", thread: "existing-thread" },
])("routes $trigger in $thread with full context", async ({ trigger, thread }) => {
  const dispose = vi.fn();
  const acknowledge = vi.fn(async () => {});
  const post = vi.fn(async () => {});
  const createAgent = vi.fn(async () => ({
    async runOutcome(_ctx: unknown, prompt: string, options?: AgentRunOptions) {
      expect(JSON.parse(prompt)).toEqual({
        thread: [{ id: "earlier", role: "human", body: "eins, zwei, drei" }],
        currentMessage: "Hello!",
      });
      expect(acknowledge).toHaveBeenCalledOnce();
      options?.onText?.("Hi! I'm ChattoBot.");

      return {
        outcome: "completed" as const,
        summary: "This internal summary must not be posted.",
        usage: emptyTokenUsage(),
      };
    },
    steer: async () => true,
    dispose,
  }));
  const bot = createChattoBot({
    acknowledge,
    createAgent,
    post,
    typing: async () => {},
    model: "test/model",
    timeout: 0,
    readThread: async () => [{ id: "earlier", role: "human", body: "eins, zwei, drei" }],
  });
  let running: Promise<unknown> | undefined;
  let starts = 0;
  const start: WebhookContext["start"] = async (root, { input }) => {
    starts++;
    running = Promise.resolve(root(createWorkflowContext(), input));
    return { id: "run" };
  };

  const incoming = { ...delivery, triggers: [trigger], thread_root_id: thread };
  await bot.route({ start }, incoming);
  await running;
  await bot.route({ start }, incoming);

  expect(starts).toBe(1);
  expect(acknowledge).toHaveBeenCalledExactlyOnceWith(incoming, expect.any(AbortSignal));
  expect(acknowledge.mock.invocationCallOrder[0]).toBeLessThan(createAgent.mock.invocationCallOrder[0]!);
  expect(post).toHaveBeenCalledExactlyOnceWith(
    { roomId: "dm", threadRootId: thread ?? "root" },
    "Hi! I'm ChattoBot.",
    expect.any(AbortSignal),
  );
  expect(createAgent).toHaveBeenCalledWith(expect.objectContaining({
    cwd: expect.stringMatching(/\/projects\/chattobot\/$/),
    model: "test/model",
    output: "text",
    tools: [],
    resources: {
      extensions: false,
      skills: false,
      promptTemplates: false,
      themes: false,
      contextFiles: false,
    },
  }));
  expect(dispose).toHaveBeenCalledOnce();
});

test("the project config registers the ChattoBot route", () => {
  expect(Object.keys(config.webhooks)).toEqual(["chatto"]);
  expect(config.webhooks.chatto.label).toBe("ChattoBot");
});

test("refreshes history for a later mention in the same conversation", async () => {
  const prompts: string[] = [];
  const acknowledged: string[] = [];
  const readThread = vi.fn()
    .mockResolvedValueOnce([{ id: "root", role: "human", body: "Hey" }])
    .mockResolvedValue([{ id: "count", role: "human", body: "eins, zwei, drei" }]);
  const post = vi.fn(async () => {
    if (post.mock.calls.length === 1) {
      await bot.route({ start: async () => { throw new Error("Unexpected run"); } }, {
        ...delivery,
        id: "follow-up",
        triggers: ["mention"],
        thread_root_id: "root",
        message: { ...delivery.message, id: "follow-up", body: "What's next?" },
      });
    }
  });
  const bot = createChattoBot({
    acknowledge: async delivery => { acknowledged.push(delivery.message.id); },
    readThread,
    post,
    typing: async () => {},
    timeout: 0.05,
    createAgent: async () => ({
      async runOutcome(_ctx, prompt, options) {
        expect(acknowledged).toEqual(["root"]);
        prompts.push(prompt);
        options?.onText?.("Reply");
        return { outcome: "completed", summary: "Reply", usage: emptyTokenUsage() };
      },
      steer: async () => false,
      dispose: () => {},
    }),
  });

  await bot(createWorkflowContext(), { ...delivery, triggers: ["mention"] });
  expect(prompts).toHaveLength(2);
  expect(acknowledged).toEqual(["root"]);
  expect(JSON.parse(prompts[1]!)).toEqual({
    thread: [{ id: "count", role: "human", body: "eins, zwei, drei" }],
    currentMessage: "What's next?",
  });
});
