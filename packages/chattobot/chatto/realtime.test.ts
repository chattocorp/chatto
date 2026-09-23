import { expect, test, vi } from "vitest";
import { createWorkflowContext, Type } from "runling";
import { messageDelivery } from "./realtime.ts";
import { createChattoRouter, createConversationState, type ChattoInbox, type Delivery } from "./routing.ts";

test("maps a client message into the application's existing delivery shape", () => {
  expect(messageDelivery({ id: "message", authorId: "human", roomId: "room", body: "hello",
    threadRootId: "root", reasons: ["mention", "reply"],
  }, "bot")).toMatchObject({ triggers: ["mention", "reply"], thread_root_id: "root",
    message: { id: "message", author_id: "human", body: "hello" } });
});

test("reloaded routes feed the original inbox and use new code only for new conversations", async () => {
  const state = createConversationState();
  let inbox!: ChattoInbox;
  let finish!: () => void;
  const first = createChattoRouter({ name: "old", output: Type.String(), post: async () => {}, state,
    run: async (_ctx, _delivery, _destination, messages) => {
      inbox = messages;
      await new Promise<void>(resolve => { finish = resolve; });
      return "old";
    },
  });
  const delivery: Delivery = { version: 1, id: "first", type: "message.created", triggers: ["direct_message"],
    occurred_at: "now", bot_id: "bot", room_id: "room", thread_root_id: null,
    message: { id: "first", author_id: "human", body: "hello" } };
  const original = first(createWorkflowContext(), delivery);
  await vi.waitFor(() => expect(inbox).toBeDefined());
  const updated = createChattoRouter({ name: "new", output: Type.String(), post: async () => {}, state,
    run: async () => "new",
  });
  const start = vi.fn(async () => ({ id: "new-run" }));
  const followup = { ...delivery, id: "second", thread_root_id: "first", message: { ...delivery.message, id: "second", body: "again" } };
  await updated.route({ start }, followup);
  await updated.route({ start }, followup);
  expect(start).not.toHaveBeenCalled();
  expect(inbox.drain()).toEqual([followup]);
  await updated.route({ start }, { ...followup, id: "other", thread_root_id: null, message: { ...followup.message, id: "other" } });
  expect(start).toHaveBeenCalledWith(updated, expect.any(Object));
  finish();
  expect(await original).toBe("old");
});

test("failed registration can retry the same delivery", async () => {
  const state = createConversationState();
  const route = createChattoRouter({ name: "bot", state, output: Type.String(), post: async () => {}, run: async () => "ok" }).route;
  const delivery: Delivery = { version: 1, id: "first", type: "message.created", triggers: ["direct_message"],
    occurred_at: "now", bot_id: "bot", room_id: "room", thread_root_id: null,
    message: { id: "first", author_id: "human", body: "hello" } };
  const start = vi.fn().mockImplementationOnce(async () => {
    expect(state.seen.size).toBe(0);
    throw new Error("disk failure");
  }).mockImplementationOnce(async () => {
    expect(state.seen.size).toBe(0);
    return { id: "run" };
  });
  await expect(route({ start }, delivery)).rejects.toThrow("Conversation registration failed");
  expect(state.seen.size).toBe(0);
  expect(state.conversations.size).toBe(0);
  await route({ start }, delivery);
  expect(state.seen.size).toBe(1);
  expect(start).toHaveBeenCalledTimes(2);
});
