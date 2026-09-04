import { RealtimeEvent } from "@chatto/api-types/realtime/v1/realtime_pb";
import {
  DirectUserMention,
  MessageMention,
  MessagePostedEvent,
  RoleMessageMention,
} from "@chatto/api-types/realtime/v1/events_pb";
import assert from "node:assert/strict";
import test from "node:test";
import { mentionReplyTarget } from "./bot.js";

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
      }),
    },
  });
}

test("targets the existing thread for a direct mention in a reply", () => {
  assert.deepEqual(
    mentionReplyTarget(
      messageEvent({ direct: true, inThread: "thread-root-1" }),
      BOT_ID,
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      threadRootEventId: "thread-root-1",
    },
  );
});

test("uses a directly mentioned root as the new thread root", () => {
  assert.deepEqual(mentionReplyTarget(messageEvent({ direct: true }), BOT_ID), {
    roomId: "room-1",
    sourceEventId: "message-1",
    threadRootEventId: "message-1",
  });
});

test("ignores indirect, self-authored, and channel-echo events", () => {
  assert.equal(mentionReplyTarget(messageEvent({ role: true }), BOT_ID), undefined);
  assert.equal(
    mentionReplyTarget(messageEvent({ actorId: BOT_ID, direct: true }), BOT_ID),
    undefined,
  );
  assert.equal(
    mentionReplyTarget(
      messageEvent({ direct: true, echoOfEventId: "canonical-reply-1" }),
      BOT_ID,
    ),
    undefined,
  );
});
