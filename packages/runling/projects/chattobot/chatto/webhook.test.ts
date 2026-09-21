import { expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage } from "runling";
import { createChattoBot } from "../workflows/chat.ts";
import { createChattoPoster, type Delivery } from "./webhook.ts";

test("posts DM replies in the triggering thread", async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(Response.json({}));
  await createChattoPoster("https://chat.example", "key", request)(
    { roomId: "dm", threadRootId: "root" }, "Hi", new AbortController().signal,
  );
  expect(JSON.parse(request.mock.calls[0]![1]!.body as string)).toEqual({
    roomId: "dm", body: "Hi", threadRootEventId: "root",
  });
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
