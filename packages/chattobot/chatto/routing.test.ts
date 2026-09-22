import { expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage } from "runling";
import { createChattoBot } from "../workflows/chat.ts";
import { createChattoPoster, type Delivery, type ChattoPost } from "./routing.ts";
import { chattoConversation } from "./chat-conversation.ts";

test("tool announcements reach the conversation thread before work starts", async () => {
  let release!: () => void;
  const posted = new Promise<void>(resolve => { release = resolve; });
  const post = vi.fn(async () => posted);
  const work = vi.fn();
  const bot = chattoConversation({ name: "Announcement test", settings: {},
    acknowledge: async () => {}, typing: async () => {}, post,
    async task(ctx, _prompt, options) {
      await options.announce("I'll compare those pages in the source.", ctx.signal);
      work();
      return "Done";
    },
  });
  const running = bot(createWorkflowContext(), {
    version: 1, id: "reply", type: "message.created", triggers: ["direct_message"],
    occurred_at: "now", bot_id: "bot", room_id: "dm", thread_root_id: "root",
    message: { id: "reply", author_id: "human", body: "Investigate" },
  });
  try {
    await vi.waitFor(() => expect(post).toHaveBeenCalledOnce());
    expect(post).toHaveBeenCalledWith({ roomId: "dm", threadRootId: "root", inReplyTo: "reply" },
      "I'll compare those pages in the source.", expect.any(AbortSignal));
    expect(work).not.toHaveBeenCalled();
  } finally { release(); await running; }
  expect(work).toHaveBeenCalledOnce();
});

test("queued replies retain their prompting message and unmentioned replies reach the active inbox", async () => {
  const entered = Promise.withResolvers<void>();
  const queued = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const post = vi.fn<ChattoPost>(async () => { entered.resolve(); await release.promise; });
  const bot = chattoConversation({ name: "Reply context", settings: {}, post,
    acknowledge: async () => {}, typing: async () => {},
    async task(ctx, prompt, options) {
      options.setReplyContext(prompt, "user");
      const first = options.announce("First reply", ctx.signal);
      const next = await ctx.inbox[Symbol.asyncIterator]().next();
      options.setReplyContext(next.value!, "user");
      const second = options.announce("Follow-up reply", ctx.signal);
      options.setReplyContext("Background finished", "notification");
      const notification = options.announce("Background result", ctx.signal);
      queued.resolve();
      await Promise.all([first, second, notification]);
      return "Done";
    },
  });
  const delivery: Delivery = {
    version: 1, id: "first", type: "message.created", triggers: ["mention"], occurred_at: "now",
    bot_id: "bot", room_id: "room", thread_root_id: "root",
    message: { id: "first", author_id: "human", body: "Hello" },
  };
  const running = bot(createWorkflowContext(), delivery);
  try {
    await entered.promise;
    const start = vi.fn();
    const followup = { ...delivery, id: "second", triggers: ["reply"], message: { ...delivery.message, id: "second", body: "Again" } };
    await bot.route({ start }, followup);
    await bot.route({ start }, followup);
    expect(start).not.toHaveBeenCalled();
    await queued.promise;
  } finally { release.resolve(); await running; }
  expect(post.mock.calls.map(([destination]) => destination.inReplyTo)).toEqual(["first", "second", undefined]);
});

test("posts DM replies in the triggering thread", async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(Response.json({}));
  await createChattoPoster("https://chat.example", "key", request)(
    { roomId: "dm", threadRootId: "root" }, "Hi", new AbortController().signal,
  );
  expect(JSON.parse(request.mock.calls[0]![1]!.body as string)).toEqual({
    roomId: "dm", body: "Hi", threadRootEventId: "root",
  });
});

test.each(["assistant-first", "announcement-first", "paraphrase"])("deduplicates concurrent announcements: %s", async order => {
  const entered = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const post = vi.fn(async () => { entered.resolve(); await release.promise; });
  const work = vi.fn();
  const bot = chattoConversation({ name: "Deduplication", settings: {},
    acknowledge: async () => {}, typing: async () => {}, post,
    async task(ctx, _prompt, options) {
      if (order !== "announcement-first") {
        await ctx.emit("I'll investigate.\n\n");
        await entered.promise;
        await options.announce(order === "paraphrase" ? "I'm going to check the source now." : "I'll investigate.", ctx.signal);
      } else {
        const announced = options.announce("I'll investigate.", ctx.signal);
        await entered.promise;
        await ctx.emit("I'll investigate.\n\n");
        await announced;
      }
      work();
      await ctx.emit("A finding");
      await ctx.emit("A finding");
      options.onBusy(false);
      options.onBusy(true);
      await options.announce("I'll investigate the next request.", ctx.signal);
      return "Done";
    },
  });
  const running = bot(createWorkflowContext(), {
    version: 1, id: "reply", type: "message.created", triggers: ["direct_message"],
    occurred_at: "now", bot_id: "bot", room_id: "dm", thread_root_id: "root",
    message: { id: "reply", author_id: "human", body: "Investigate" },
  });
  try {
    await entered.promise;
    expect(work).not.toHaveBeenCalled();
  } finally { release.resolve(); await running; }
  expect(post.mock.calls).toHaveLength(4);
  expect(work).toHaveBeenCalledOnce();
});

test("host shutdown cancels the conversation without sending a failure reply", async () => {
  const controller = new AbortController();
  const entered = Promise.withResolvers<void>();
  const cleaning = Promise.withResolvers<void>();
  const releaseCleanup = Promise.withResolvers<void>();
  const post = vi.fn(async () => {});
  const bot = chattoConversation({ name: "Shutdown", settings: {},
    acknowledge: async () => {}, typing: async () => {}, post,
    async task(ctx) {
      entered.resolve();
      await new Promise<void>(resolve => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
      cleaning.resolve();
      await releaseCleanup.promise;
      ctx.signal.throwIfAborted();
      return "";
    },
  });
  const running = bot({ ...createWorkflowContext(), signal: controller.signal }, {
    version: 1, id: "reply", type: "message.created", triggers: ["direct_message"],
    occurred_at: "now", bot_id: "bot", room_id: "dm", thread_root_id: "root",
    message: { id: "reply", author_id: "human", body: "Investigate" },
  });
  const rejected = expect(running).rejects.toThrow("Host stopped");
  let finished = false;
  void running.then(() => { finished = true; }, () => { finished = true; });
  await entered.promise;
  controller.abort(new Error("Host stopped"));
  await cleaning.promise;
  // Allow the cancelled spawn handle and its owner to settle if cleanup is not awaited.
  await new Promise(resolve => setTimeout(resolve, 10));
  expect(finished).toBe(false);
  releaseCleanup.resolve();
  await rejected;
  expect(post).not.toHaveBeenCalled();
});

test("DM thread follow-ups share a conversation but separate roots start new runs", async () => {
  const delivery: Delivery = {
    version: 1, id: "first", type: "message.created", triggers: ["direct_message"],
    occurred_at: "now", bot_id: "bot", room_id: "dm", thread_root_id: null,
    message: { id: "first", author_id: "alice", body: "Hi" },
  };
  const prompts: string[] = [];
  const acknowledged: string[] = [];
  const post = vi.fn(async () => {});
  const bot = createChattoBot({
    acknowledge: async delivery => { acknowledged.push(delivery.message.id); },
    post, typing: async () => {}, readThread: async () => [], timeout: 0.2,
    createAgent: async () => ({
      async runOutcome(_ctx, prompt, options) {
        prompts.push(JSON.parse(prompt).currentMessage);
        options?.onText?.("Reply");
        return { outcome: "completed", summary: "Reply", usage: emptyTokenUsage() };
      },
      steer: async () => false,
      dispose: () => {},
    }),
  });
  const running = bot(createWorkflowContext(), delivery);
  await vi.waitFor(() => expect(post).toHaveBeenCalledOnce());
  const start = vi.fn(async () => ({ id: "unexpected" }));
  await bot.route({ start }, {
    ...delivery, id: "second", thread_root_id: "first",
    message: { ...delivery.message, id: "second", body: "Again" },
  });
  expect(start).not.toHaveBeenCalled();
  await bot.route({ start }, {
    ...delivery, id: "other-root",
    message: { ...delivery.message, id: "other-root", body: "New topic" },
  });
  await running;
  expect(start).toHaveBeenCalledOnce();
  expect(prompts).toEqual(["Hi", "Again"]);
  expect(acknowledged).toEqual(["first"]);
  expect(post.mock.calls).toHaveLength(2);
});
