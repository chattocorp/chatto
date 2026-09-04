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
  conversationPrompt,
  messageReplyTarget,
  replyToMessage,
  THINKING_REPLY,
  type BotAPI,
} from "./bot.js";
import { loadTestBotState, type TestBotState } from "./state.js";

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

test("uses every supplied thread message with anonymous prompt labels", () => {
  assert.equal(
    conversationPrompt(
      [
        { eventId: "1", actorId: "user-secret-id", body: "Earlier context." },
        { eventId: "2", actorId: BOT_ID, body: "An earlier answer." },
        {
          eventId: "3",
          actorId: "another-secret-id",
          body: "A message that does not mention the bot.",
        },
        { eventId: "4", actorId: "user-secret-id", body: "@test_bot Help?" },
      ],
      BOT_ID,
    ),
    "Person 1: Earlier context.\n\nAssistant: An earlier answer.\n\nPerson 2: A message that does not mention the bot.\n\nPerson 1: @test_bot Help?",
  );
});

test("posts an italic placeholder immediately and reuses it after an AI failure", async (t) => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "chatto-test-bot-"));
  t.after(() => rm(directory, { force: true, recursive: true }));
  const stateFile = path.join(directory, "state.json");
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
    stateFile,
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
    stateFile,
  );

  assert.equal(placeholderPosts, 1);
  assert.equal(actions.at(-1), "updated:reply-1:Final answer");
});
