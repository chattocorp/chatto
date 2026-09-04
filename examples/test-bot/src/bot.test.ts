import { RealtimeEvent } from "@chatto/api-types/realtime/v1/realtime_pb";
import {
  DirectUserMention,
  MessageMention,
  MessagePostedEvent,
  RoleMessageMention,
} from "@chatto/api-types/realtime/v1/events_pb";
import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import type { AIResponder } from "./ai.js";
import {
  messageReplyTarget,
  OrderedConcurrentProcessor,
  replyToMessage,
  threadAIConversation,
  THINKING_REPLY,
  type BotAPI,
} from "./bot.js";
import {
  loadTestBotState,
  serialTestBotStateSaver,
  type TestBotState,
} from "./state.js";

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
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      threadRootEventId: "thread-root-1",
      sourceActorId: "user-1",
      sourceBody: "hello",
    },
  );
});

test("uses a directly mentioned root as the new thread root", () => {
  assert.deepEqual(messageReplyTarget(messageEvent({ direct: true }), BOT_ID), {
    roomId: "room-1",
    sourceEventId: "message-1",
    threadRootEventId: "message-1",
    sourceActorId: "user-1",
    sourceBody: "hello",
  });
});

test("ignores later messages in a thread without another direct mention", () => {
  assert.equal(
    messageReplyTarget(messageEvent({ inThread: "thread-root-1" }), BOT_ID),
    undefined,
  );
});

test("ignores indirect, self-authored, and channel-echo events", () => {
  assert.equal(
    messageReplyTarget(messageEvent({ role: true }), BOT_ID),
    undefined,
  );
  assert.equal(
    messageReplyTarget(messageEvent({ actorId: BOT_ID, direct: true }), BOT_ID),
    undefined,
  );
  assert.equal(
    messageReplyTarget(
      messageEvent({ direct: true, echoOfEventId: "canonical-reply-1" }),
      BOT_ID,
    ),
    undefined,
  );
});

test("builds a structured thread snapshot ending at its source message", () => {
  const conversation = threadAIConversation(
    [
      { eventId: "1", actorId: "user-secret-id", body: "Earlier context." },
      { eventId: "2", actorId: BOT_ID, body: "An earlier answer." },
      { eventId: "placeholder", actorId: BOT_ID, body: THINKING_REPLY },
      {
        eventId: "3",
        actorId: "another-secret-id",
        body: "A message that does not mention the bot.",
      },
      { eventId: "4", actorId: "user-secret-id", body: "@test_bot Help?" },
      { eventId: "5", actorId: "user-secret-id", body: "A later message." },
    ],
    BOT_ID,
    "room-1",
    "thread-1",
    "4",
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
  assert.equal(
    conversation.turns.some((turn) => turn.content === THINKING_REPLY),
    false,
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

test("posts an italic placeholder immediately and reuses it after an AI failure", async (t) => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "chatto-test-bot-"));
  t.after(() => rm(directory, { force: true, recursive: true }));
  const stateFile = path.join(directory, "state.json");
  const saveState = serialTestBotStateSaver(stateFile);
  const state: TestBotState = { processedEventIds: [], pendingReplies: [] };
  const actions: string[] = [];
  let rejectFirstAI: (reason: Error) => void = () => {};
  let markAIStarted: () => void = () => {};
  const aiStarted = new Promise<void>((resolve) => {
    markAIStarted = resolve;
  });
  const firstAI: AIResponder = {
    provider: "test",
    model: "test",
    respond: async () =>
      new Promise<string>((_resolve, reject) => {
        rejectFirstAI = reject;
        actions.push("ai-started");
        markAIStarted();
      }),
  };
  let placeholderPosts = 0;
  const api: BotAPI = {
    viewerId: BOT_ID,
    isBotActor: async () => false,
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
      placeholderPosts += 1;
      actions.push(`posted:${body}`);
      return "reply-1";
    },
    updateReply: async (_roomId, replyEventId, body) => {
      actions.push(`updated:${replyEventId}:${body}`);
    },
  };
  const event = messageEvent({ direct: true, inThread: "thread-root-1" });

  const firstAttempt = replyToMessage(
    api,
    firstAI,
    event,
    new AbortController().signal,
    state,
    saveState,
  );
  await aiStarted;

  assert.deepEqual(actions, [
    `posted:${THINKING_REPLY}`,
    "conversation-loaded",
    "ai-started",
  ]);
  assert.equal(THINKING_REPLY, "_Thinking…_");
  assert.deepEqual((await loadTestBotState(stateFile)).pendingReplies, [
    { sourceEventId: "message-1", replyEventId: "reply-1" },
  ]);

  rejectFirstAI(new Error("provider failed"));
  await assert.rejects(firstAttempt, /provider failed/);

  const secondAI: AIResponder = {
    provider: "test",
    model: "test",
    respond: async () => "Final answer",
  };
  await replyToMessage(
    api,
    secondAI,
    event,
    new AbortController().signal,
    state,
    saveState,
  );

  assert.equal(placeholderPosts, 1);
  assert.equal(actions.at(-1), "updated:reply-1:Final answer");
});
