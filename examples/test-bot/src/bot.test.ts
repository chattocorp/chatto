import { RealtimeEvent } from "@chatto/api-types/realtime/v1/realtime_pb";
import {
  DirectUserMention,
  MessageMention,
  MessagePostedEvent,
  RoleMessageMention,
} from "@chatto/api-types/realtime/v1/events_pb";
import { RoomKind } from "@chatto/api-types/api/v1/rooms_pb";
import assert from "node:assert/strict";
import test from "node:test";
import type { AIResponder } from "./ai.js";
import {
  chatAIConversation,
  messageReplyTarget,
  OrderedConcurrentProcessor,
  refreshTypingIndicator,
  replyToMessage,
  type BotAPI,
} from "./bot.js";

const BOT_ID = "bot-1";

function messageEvent(options?: {
  actorId?: string;
  direct?: boolean;
  echoOfEventId?: string;
  inThread?: string;
  role?: boolean;
}): RealtimeEvent {
  const mentions = [];
  if (options?.direct) {
    mentions.push(
      new MessageMention({
        userId: BOT_ID,
        cause: { case: "direct", value: new DirectUserMention() },
      }),
    );
  }
  if (options?.role) {
    mentions.push(
      new MessageMention({
        userId: BOT_ID,
        cause: {
          case: "role",
          value: new RoleMessageMention({ roleName: "helpers" }),
        },
      }),
    );
  }
  return new RealtimeEvent({
    id: "message-1",
    actorId: options?.actorId ?? "user-1",
    event: {
      case: "messagePosted",
      value: new MessagePostedEvent({
        roomId: "room-1",
        inThread: options?.inThread,
        echoOfEventId: options?.echoOfEventId,
        mentions,
        bodyPlaintext: "hello",
      }),
    },
  });
}

test("targets the existing thread for a direct mention in a reply", () => {
  assert.deepEqual(
    messageReplyTarget(
      messageEvent({ direct: true, inThread: "thread-root-1" }),
      BOT_ID,
      RoomKind.CHANNEL,
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      sourceActorId: "user-1",
      sourceBody: "hello",
      scope: { case: "thread", threadRootEventId: "thread-root-1" },
    },
  );
});

test("uses a directly mentioned root as the new thread root", () => {
  assert.deepEqual(
    messageReplyTarget(
      messageEvent({ direct: true }),
      BOT_ID,
      RoomKind.CHANNEL,
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      sourceActorId: "user-1",
      sourceBody: "hello",
      scope: { case: "thread", threadRootEventId: "message-1" },
    },
  );
});

test("targets a direct message without a mention", () => {
  assert.deepEqual(
    messageReplyTarget(messageEvent(), BOT_ID, RoomKind.DM),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      sourceActorId: "user-1",
      sourceBody: "hello",
      scope: { case: "directMessage" },
    },
  );
});

test("ignores later messages in a thread without another direct mention", () => {
  assert.equal(
    messageReplyTarget(
      messageEvent({ inThread: "thread-root-1" }),
      BOT_ID,
      RoomKind.CHANNEL,
    ),
    undefined,
  );
});

test("ignores indirect, self-authored, and channel-echo events", () => {
  assert.equal(
    messageReplyTarget(messageEvent({ role: true }), BOT_ID, RoomKind.CHANNEL),
    undefined,
  );
  assert.equal(
    messageReplyTarget(
      messageEvent({ actorId: BOT_ID, direct: true }),
      BOT_ID,
      RoomKind.CHANNEL,
    ),
    undefined,
  );
  assert.equal(
    messageReplyTarget(
      messageEvent({ direct: true, echoOfEventId: "canonical-reply-1" }),
      BOT_ID,
      RoomKind.CHANNEL,
    ),
    undefined,
  );
});

test("builds a structured thread snapshot ending at its source message", () => {
  const conversation = chatAIConversation(
    [
      { eventId: "1", actorId: "user-secret-id", body: "Earlier context." },
      { eventId: "2", actorId: BOT_ID, body: "An earlier answer." },
      {
        eventId: "3",
        actorId: "another-secret-id",
        body: "A message that does not mention the bot.",
      },
      { eventId: "4", actorId: "user-secret-id", body: "@test_bot Help?" },
      { eventId: "5", actorId: "user-secret-id", body: "A later message." },
    ],
    BOT_ID,
    {
      roomId: "room-1",
      sourceEventId: "4",
      sourceActorId: "user-secret-id",
      sourceBody: "@test_bot Help?",
      scope: { case: "thread", threadRootEventId: "thread-1" },
    },
  );

  assert.match(conversation.sessionId, /^chatto-thread-[a-f0-9]{32}$/);
  assert.deepEqual(
    conversation.turns.map((turn) => turn.role),
    ["user", "assistant", "user", "user"],
  );
  assert.equal(conversation.turns[1]?.content, "An earlier answer.");
  assert.match(
    conversation.turns[0]?.content ?? "",
    /^Person [a-f0-9]{8}: Earlier context\.$/,
  );
  assert.match(
    conversation.turns[2]?.content ?? "",
    /^Person [a-f0-9]{8}: A message that does not mention the bot\.$/,
  );
  assert.equal(
    conversation.turns[0]?.content.split(":", 1)[0],
    conversation.turns[3]?.content.split(":", 1)[0],
  );
  assert.match(conversation.turns[3]?.content ?? "", /@test_bot Help\?$/);
  assert.equal(
    conversation.turns.some((turn) => turn.content.includes("later message")),
    false,
  );
});

test("uses one stable AI session for a direct-message conversation", () => {
  const target = {
    roomId: "dm-1",
    sourceEventId: "2",
    sourceActorId: "user-1",
    sourceBody: "Can you help?",
    scope: { case: "directMessage" as const },
  };
  const first = chatAIConversation(
    [
      { eventId: "1", actorId: BOT_ID, body: "Hello." },
      { eventId: "2", actorId: "user-1", body: "Can you help?" },
    ],
    BOT_ID,
    target,
  );
  const second = chatAIConversation(
    [{ eventId: "2", actorId: "user-1", body: "Can you help?" }],
    BOT_ID,
    target,
  );

  assert.match(first.sessionId, /^chatto-dm-[a-f0-9]{32}$/);
  assert.equal(second.sessionId, first.sessionId);
  assert.deepEqual(
    first.turns.map((turn) => turn.role),
    ["assistant", "user"],
  );
});

test("runs work concurrently but commits it in submission order", async () => {
  const processor = new OrderedConcurrentProcessor(2);
  const started: number[] = [];
  const committed: number[] = [];
  let finishFirst: () => void = () => {};
  let finishSecond: () => void = () => {};
  let finishThird: () => void = () => {};
  const firstGate = new Promise<void>((resolve) => {
    finishFirst = resolve;
  });
  const secondGate = new Promise<void>((resolve) => {
    finishSecond = resolve;
  });
  const thirdGate = new Promise<void>((resolve) => {
    finishThird = resolve;
  });

  void processor.enqueue(
    async () => {
      started.push(1);
      await firstGate;
    },
    async () => {
      committed.push(1);
    },
  );
  void processor.enqueue(
    async () => {
      started.push(2);
      await secondGate;
    },
    async () => {
      committed.push(2);
    },
  );
  void processor.enqueue(
    async () => {
      started.push(3);
      await thirdGate;
    },
    async () => {
      committed.push(3);
    },
  );
  await new Promise((resolve) => setImmediate(resolve));

  assert.deepEqual(started, [1, 2]);
  finishSecond();
  await new Promise((resolve) => setImmediate(resolve));
  assert.deepEqual(started, [1, 2, 3]);
  assert.deepEqual(committed, []);
  finishFirst();
  finishThird();
  await processor.wait();
  assert.deepEqual(committed, [1, 2, 3]);
});

test("sends typing before the model and posts only the final reply", async () => {
  const actions: string[] = [];
  const ai: AIResponder = {
    provider: "test",
    model: "test",
    respond: async () => {
      actions.push("ai-started");
      return "Final answer";
    },
  };
  const api: BotAPI = {
    viewerId: BOT_ID,
    roomKind: async () => RoomKind.CHANNEL,
    isBotActor: async () => false,
    updateTypingIndicator: async () => {
      actions.push("typing");
    },
    loadConversation: async (target) => {
      actions.push("conversation-loaded");
      return [
        {
          eventId: target.sourceEventId,
          actorId: target.sourceActorId,
          body: target.sourceBody,
        },
      ];
    },
    postReply: async (_target, body) => {
      actions.push(`posted:${body}`);
      return "reply-1";
    },
  };
  const event = messageEvent({ direct: true, inThread: "thread-root-1" });

  await replyToMessage(api, ai, event, new AbortController().signal);

  assert.deepEqual(actions, [
    "typing",
    "conversation-loaded",
    "ai-started",
    "posted:Final answer",
  ]);
});

test("refreshes typing until the reply operation stops", async () => {
  const controller = new AbortController();
  let updates = 0;
  const api: Pick<BotAPI, "updateTypingIndicator"> = {
    updateTypingIndicator: async () => {
      updates += 1;
      if (updates === 3) controller.abort();
    },
  };
  const target = {
    roomId: "room-1",
    sourceEventId: "message-1",
    sourceActorId: "user-1",
    sourceBody: "hello",
    scope: { case: "thread" as const, threadRootEventId: "thread-root-1" },
  };

  await refreshTypingIndicator(api, target, controller.signal, 0);

  assert.equal(updates, 3);
});

test("does not post a message when generation fails", async () => {
  let posts = 0;
  const api: BotAPI = {
    viewerId: BOT_ID,
    roomKind: async () => RoomKind.CHANNEL,
    isBotActor: async () => false,
    updateTypingIndicator: async () => undefined,
    loadConversation: async (target) => [
      {
        eventId: target.sourceEventId,
        actorId: target.sourceActorId,
        body: target.sourceBody,
      },
    ],
    postReply: async () => {
      posts += 1;
      return "reply-1";
    },
  };
  const ai: AIResponder = {
    provider: "test",
    model: "test",
    respond: async () => {
      throw new Error("provider failed");
    },
  };

  await assert.rejects(
    replyToMessage(
      api,
      ai,
      messageEvent({ direct: true, inThread: "thread-root-1" }),
      new AbortController().signal,
    ),
    /provider failed/,
  );

  assert.equal(posts, 0);
});
