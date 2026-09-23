import { expect, test, vi } from "vitest";
import { createChattoClient, RealtimeEvent, RoomKind } from "@chatto/client";
import { conversationKey, createBotClient, createDeliveryTracker, replyDestination } from "./index.js";

test("composes a client, resolves identity once, and replies to the prompting message", async () => {
  const request = vi.fn<typeof fetch>().mockImplementation(async () => Response.json({ user: { profile: { id: "bot" } } }));
  const client = createChattoClient({ serverUrl: "https://chat.example", apiKey: "secret", fetch: request });
  const bot = await createBotClient(client);
  expect(bot.client).toBe(client);
  expect(bot.viewerId).toBe("bot");
  const event = new RealtimeEvent({ id: "incoming", actorId: "human", event: { case: "messagePosted", value: {
    roomId: "room", roomKind: RoomKind.DM, bodyPlaintext: "hello", threadRootEventId: "root",
  } } });
  const message = (await bot.addressedMessage(event))!;
  expect(message.reasons).toEqual(["direct_message"]);
  expect(bot.conversationKey(message)).toBe(JSON.stringify(["bot", "room", "root", "human"]));
  await bot.reply(message, "hi");
  expect(request).toHaveBeenCalledTimes(2);
  expect(JSON.parse(request.mock.calls[1]![1]!.body as string)).toEqual({
    roomId: "room", threadRootEventId: "root", inReplyTo: "incoming", body: "hi",
  });
  const controller = new AbortController();
  controller.abort(new Error("stop"));
  await expect(bot.reply(message, "no", controller.signal)).rejects.toThrow("stop");
  expect(request).toHaveBeenCalledTimes(2);
});

test("identity failures and cancellation do not create a bot adapter", async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(Response.json({}));
  const client = createChattoClient({ serverUrl: "https://chat.example", apiKey: "secret", fetch: request });
  await expect(createBotClient(client)).rejects.toThrow("viewer identity");
  const signal = AbortSignal.abort(new Error("cancelled"));
  await expect(createBotClient(client, { signal })).rejects.toThrow("cancelled");
  expect(request).toHaveBeenCalledOnce();
});

test("bot thread roles are separate from the API client's message data", async () => {
  const client = createChattoClient({ serverUrl: "https://chat.example", apiKey: "secret" });
  vi.spyOn(client, "getViewer").mockResolvedValue({ id: "bot" });
  const messages = [{ id: "one", authorId: "bot", body: "hi" }, { id: "two", authorId: "other", body: "hello" }];
  vi.spyOn(client, "readThread").mockResolvedValue(messages);
  const bot = await createBotClient(client);
  expect(await bot.readThread({ roomId: "room", threadRootId: "one" })).toEqual([
    { ...messages[0], role: "bot" }, { ...messages[1], role: "human" },
  ]);
  expect(messages[0]).not.toHaveProperty("role");
});

test("default keys isolate bot, room, thread, and sender; root messages reply in their own thread", () => {
  const message = { id: "root", roomId: "room", authorId: "human" };
  const key = conversationKey("bot", message);
  expect(replyDestination(message)).toEqual({ roomId: "room", threadRootId: "root", inReplyTo: "root" });
  expect(conversationKey("bot", { ...message, threadRootId: "root" })).toBe(key);
  expect(conversationKey("other", message)).not.toBe(key);
  for (const override of [{ roomId: "other" }, { threadRootId: "other" }, { authorId: "other" }]) {
    expect(conversationKey("bot", { ...message, ...override })).not.toBe(key);
  }
});

test("deliveries remain retryable until explicitly accepted, then expire", async () => {
  let now = 0;
  const tracker = createDeliveryTracker({ retentionMs: 10, now: () => now });
  const accept = async (register: () => Promise<void>) => {
    if (tracker.has("message")) return;
    await register();
    tracker.accept("message");
  };
  await expect(accept(async () => { throw new Error("disk"); })).rejects.toThrow("disk");
  expect(tracker.has("message")).toBe(false);
  const register = vi.fn().mockResolvedValue(undefined);
  await accept(register);
  await accept(register);
  expect(register).toHaveBeenCalledOnce();
  expect(createDeliveryTracker().has("message")).toBe(false);
  now = 10;
  await accept(register);
  expect(register).toHaveBeenCalledTimes(2);
});

test.each([0, -1, Infinity, NaN])("rejects invalid delivery retention: %s", retentionMs => {
  expect(() => createDeliveryTracker({ retentionMs })).toThrow("positive and finite");
});
