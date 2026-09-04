import { RealtimeEvent } from "@chatto/api-types/realtime/v1/realtime_pb";
import {
  DirectUserMention,
  MessageMention,
  MessagePostedEvent,
  RoleMessageMention,
} from "@chatto/api-types/realtime/v1/events_pb";
import assert from "node:assert/strict";
import test from "node:test";
import { conversationPrompt, messageReplyTarget } from "./bot.js";

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
      new Set(),
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      threadRootEventId: "thread-root-1",
      directMention: true,
      sourceActorId: "user-1",
      sourceBody: "hello",
    },
  );
});

test("uses a directly mentioned root as the new thread root", () => {
  assert.deepEqual(
    messageReplyTarget(messageEvent({ direct: true }), BOT_ID, new Set()),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      threadRootEventId: "message-1",
      directMention: true,
      sourceActorId: "user-1",
      sourceBody: "hello",
    },
  );
});

test("targets later messages in a followed thread without another mention", () => {
  assert.deepEqual(
    messageReplyTarget(
      messageEvent({ inThread: "thread-root-1" }),
      BOT_ID,
      new Set(["room-1\u0000thread-root-1"]),
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      threadRootEventId: "thread-root-1",
      directMention: false,
      sourceActorId: "user-1",
      sourceBody: "hello",
    },
  );
});

test("ignores indirect, self-authored, and channel-echo events", () => {
  assert.equal(
    messageReplyTarget(messageEvent({ role: true }), BOT_ID, new Set()),
    undefined,
  );
  assert.equal(
    messageReplyTarget(
      messageEvent({ actorId: BOT_ID, direct: true }),
      BOT_ID,
      new Set(),
    ),
    undefined,
  );
  assert.equal(
    messageReplyTarget(
      messageEvent({ direct: true, echoOfEventId: "canonical-reply-1" }),
      BOT_ID,
      new Set(),
    ),
    undefined,
  );
});

test("uses anonymous prompt labels while preserving the thread transcript", () => {
  assert.equal(
    conversationPrompt(
      [
        { eventId: "1", actorId: "user-secret-id", body: "Can you help?" },
        { eventId: "2", actorId: BOT_ID, body: "Certainly." },
        {
          eventId: "3",
          actorId: "another-secret-id",
          body: "What about this?",
        },
      ],
      BOT_ID,
    ),
    "Person 1: Can you help?\n\nAssistant: Certainly.\n\nPerson 2: What about this?",
  );
});
