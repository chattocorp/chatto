import { RealtimeEvent } from "@chatto/api-types/realtime/v1/realtime_pb";
import {
  DirectUserMention,
  MessageMention,
  MessagePostedEvent,
  RoleMessageMention,
  AllMessageMention,
  HereMessageMention,
} from "@chatto/api-types/realtime/v1/events_pb";
import { RoomKind } from "@chatto/api-types/api/v1/rooms_pb";
import assert from "node:assert/strict";
import test from "node:test";
import type { AIResponder } from "./ai.js";
import {
  chatAIConversation,
  connectPublicAPI,
  ConversationReplyScheduler,
  messageReplyTarget,
  OrderedCommitProcessor,
  PROCESSING_FAILURE_CLOSE_CODE,
  refreshTypingIndicator,
  type BotAPI,
} from "./bot.js";

const BOT_ID = "bot-1";

test("uses an application WebSocket close code for local failures", () => {
  assert.ok(
    PROCESSING_FAILURE_CLOSE_CODE >= 3_000 &&
      PROCESSING_FAILURE_CLOSE_CODE <= 4_999,
  );
});

function messageEvent(options?: {
  actorId?: string;
  body?: string;
  direct?: boolean;
  echoOfEventId?: string;
  eventId?: string;
  threadRootEventId?: string;
  role?: boolean;
  roomId?: string;
  roomKind?: RoomKind;
  cursor?: string;
}): RealtimeEvent {
  const mentions = [];
  if (options?.direct) {
    mentions.push(
      new MessageMention({
        includesViewer: true,
        cause: { case: "direct", value: new DirectUserMention({ userId: BOT_ID }) },
      }),
    );
  }
  if (options?.role) {
    mentions.push(
      new MessageMention({
        includesViewer: true,
        cause: {
          case: "role",
          value: new RoleMessageMention({ roleName: "helpers" }),
        },
      }),
    );
  }
  return new RealtimeEvent({
    id: options?.eventId ?? "message-1",
    cursor: options?.cursor,
    actorId: options?.actorId ?? "user-1",
    event: {
      case: "messagePosted",
      value: new MessagePostedEvent({
        roomId: options?.roomId ?? "room-1",
        roomKind: options?.roomKind ?? RoomKind.CHANNEL,
        threadRootEventId: options?.threadRootEventId,
        echoOfEventId: options?.echoOfEventId,
        mentions,
        bodyPlaintext: options?.body ?? "hello",
      }),
    },
  });
}

test("targets the existing thread for a direct mention in a reply", () => {
  assert.deepEqual(
    messageReplyTarget(
      messageEvent({ direct: true, threadRootEventId: "thread-root-1" }),
      BOT_ID,
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      sourceActorId: "user-1",
      sourceBody: "hello",
      threadRootEventId: "thread-root-1",
      trigger: "direct_mention",
    },
  );
});

test("does not treat broadcast inclusion or another direct target as a direct mention", () => {
  for (const mention of [
    new MessageMention({ includesViewer: true, cause: { case: "all", value: new AllMessageMention() } }),
    new MessageMention({ includesViewer: true, cause: { case: "here", value: new HereMessageMention() } }),
    new MessageMention({ includesViewer: false, cause: { case: "direct", value: new DirectUserMention({ userId: "other-user" }) } }),
  ]) {
    const event = messageEvent();
    if (event.event.case !== "messagePosted") throw new Error("invalid fixture");
    event.event.value.mentions = [mention];
    assert.equal(messageReplyTarget(event, BOT_ID), undefined);
  }
});

test("causal RPCs carry the source cursor without loading the room directory", async (t) => {
  const calls: Array<{ method: string; cursor: string | null }> = [];
  t.mock.method(
    globalThis,
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const method = new URL(request.url).pathname.split("/").at(-1)!;
      calls.push({
        method,
        cursor: request.headers.get("Chatto-Realtime-Minimum-Cursor"),
      });
      assert.equal(request.headers.get("Authorization"), "Bearer test-key");
      const responses: Record<string, unknown> = {
        GetViewer: { user: { profile: { id: BOT_ID } } },
        GetUser: { user: { user: { id: "user-1", isBot: false } } },
        GetThreadEventsAround: { page: { events: [] } },
        UpdateTypingIndicator: { updated: true },
        CreateMessage: { message: { id: "bot-reply" } },
      };
      assert.ok(method in responses, `unexpected RPC ${method}`);
      return Response.json(responses[method]);
    },
  );
  const api = await connectPublicAPI(
    {
      serverUrl: "https://test.invalid",
      apiKeyFile: "unused",
      stateFile: "unused",
      ai: { provider: "faux" },
    },
    "test-key",
  );
  const target = messageReplyTarget(
    messageEvent({
      direct: true,
      threadRootEventId: "root",
      cursor: "source-boundary",
    }),
    BOT_ID,
  )!;
  assert.equal(target.minimumCursor, "source-boundary");
  assert.equal(
    await api.isBotActor(target.sourceActorId, target.minimumCursor),
    false,
  );
  await api.loadConversation(target);
  await api.updateTypingIndicator(target);
  assert.equal(await api.postReply(target, "reply"), "bot-reply");
  assert.deepEqual(calls, [
    { method: "GetViewer", cursor: null },
    ...[
      "GetUser",
      "GetThreadEventsAround",
      "UpdateTypingIndicator",
      "CreateMessage",
    ].map((method) => ({ method, cursor: "source-boundary" })),
  ]);
});

test("uses a directly mentioned root as the new thread root", () => {
  assert.deepEqual(messageReplyTarget(messageEvent({ direct: true }), BOT_ID), {
    roomId: "room-1",
    sourceEventId: "message-1",
    sourceActorId: "user-1",
    sourceBody: "hello",
    threadRootEventId: "message-1",
    trigger: "direct_mention",
  });
});

test("targets a direct message without a mention", () => {
  assert.deepEqual(
    messageReplyTarget(messageEvent({ roomKind: RoomKind.DM }), BOT_ID),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      sourceActorId: "user-1",
      sourceBody: "hello",
      threadRootEventId: "message-1",
      trigger: "direct_message",
    },
  );
});

test("targets the existing thread for a direct-message reply", () => {
  assert.deepEqual(
    messageReplyTarget(
      messageEvent({
        threadRootEventId: "dm-thread-root-1",
        roomKind: RoomKind.DM,
      }),
      BOT_ID,
    ),
    {
      roomId: "room-1",
      sourceEventId: "message-1",
      sourceActorId: "user-1",
      sourceBody: "hello",
      threadRootEventId: "dm-thread-root-1",
      trigger: "direct_message",
    },
  );
});

test("ignores later messages in a thread without another direct mention", () => {
  assert.equal(
    messageReplyTarget(
      messageEvent({ threadRootEventId: "thread-root-1" }),
      BOT_ID,
    ),
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
      threadRootEventId: "thread-1",
      trigger: "direct_mention",
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

test("uses one stable AI session for a direct-message thread", () => {
  const target = {
    roomId: "dm-1",
    sourceEventId: "2",
    sourceActorId: "user-1",
    sourceBody: "Can you help?",
    threadRootEventId: "1",
    trigger: "direct_message" as const,
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

  assert.match(first.sessionId, /^chatto-thread-[a-f0-9]{32}$/);
  assert.equal(second.sessionId, first.sessionId);
  assert.deepEqual(
    first.turns.map((turn) => turn.role),
    ["assistant", "user"],
  );
});

test("commits concurrent work in submission order", async () => {
  const processor = new OrderedCommitProcessor();
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

  assert.deepEqual(started, [1, 2, 3]);
  finishSecond();
  finishThird();
  await new Promise((resolve) => setImmediate(resolve));
  assert.deepEqual(committed, []);
  finishFirst();
  await processor.wait();
  assert.deepEqual(committed, [1, 2, 3]);
});

test("sends typing before the model and posts one final reply", async () => {
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
  const event = messageEvent({
    direct: true,
    threadRootEventId: "thread-root-1",
  });
  const scheduler = new ConversationReplyScheduler(
    api,
    ai,
    new AbortController().signal,
    { settleIntervalMs: 0 },
  );

  const accepted = await scheduler.accept(event);
  await accepted.completion;

  assert.deepEqual(actions, [
    "typing",
    "conversation-loaded",
    "ai-started",
    "posted:Final answer",
  ]);
});

test("supersedes an active reply with the latest message in its conversation", async () => {
  const started: string[] = [];
  const superseded: string[] = [];
  const completed: string[] = [];
  let firstStarted: () => void = () => {};
  let secondStarted: () => void = () => {};
  let finishSecond: () => void = () => {};
  const firstStartedGate = new Promise<void>((resolve) => {
    firstStarted = resolve;
  });
  const secondStartedGate = new Promise<void>((resolve) => {
    secondStarted = resolve;
  });
  const secondFinishGate = new Promise<void>((resolve) => {
    finishSecond = resolve;
  });
  const api: BotAPI = {
    viewerId: BOT_ID,
    isBotActor: async () => false,
    updateTypingIndicator: async () => undefined,
    loadConversation: async (target) => {
      started.push(target.sourceEventId);
      return [
        {
          eventId: target.sourceEventId,
          actorId: target.sourceActorId,
          body: target.sourceBody,
        },
      ];
    },
    postReply: async (target) => {
      completed.push(target.sourceEventId);
      return `reply-${target.sourceEventId}`;
    },
  };
  let responseCount = 0;
  const ai: AIResponder = {
    provider: "test",
    model: "test",
    respond: async (_conversation, signal) => {
      responseCount += 1;
      if (responseCount === 1) {
        firstStarted();
        return new Promise<string>((resolve) => {
          signal.addEventListener(
            "abort",
            () => {
              superseded.push("message-1");
              resolve("Stale answer");
            },
            { once: true },
          );
        });
      }
      secondStarted();
      await secondFinishGate;
      return "Combined answer";
    },
  };
  const scheduler = new ConversationReplyScheduler(
    api,
    ai,
    new AbortController().signal,
    {
      settleIntervalMs: 0,
    },
  );

  const first = await scheduler.accept(
    messageEvent({ direct: true, threadRootEventId: "thread-root-1" }),
  );
  await firstStartedGate;
  const continuation = await scheduler.accept(
    messageEvent({
      body: "and a joke",
      eventId: "message-2",
      threadRootEventId: "thread-root-1",
    }),
  );
  await secondStartedGate;
  finishSecond();
  await Promise.all([first.completion, continuation.completion]);

  assert.deepEqual(started, ["message-1", "message-2"]);
  assert.deepEqual(superseded, ["message-1"]);
  assert.deepEqual(completed, ["message-2"]);
});

test("runs separate conversations concurrently within the global limit", async () => {
  const started: string[] = [];
  const finishes = new Map<string, () => void>();
  let bothStarted: () => void = () => {};
  let thirdStarted: () => void = () => {};
  const bothStartedGate = new Promise<void>((resolve) => {
    bothStarted = resolve;
  });
  const thirdStartedGate = new Promise<void>((resolve) => {
    thirdStarted = resolve;
  });
  const api: BotAPI = {
    viewerId: BOT_ID,
    isBotActor: async () => false,
    updateTypingIndicator: async () => undefined,
    loadConversation: async () => [],
    postReply: async () => "unused",
  };
  const ai: AIResponder = {
    provider: "test",
    model: "test",
    respond: async () => "unused",
  };
  const scheduler = new ConversationReplyScheduler(
    api,
    ai,
    new AbortController().signal,
    {
      maximumConcurrency: 2,
      settleIntervalMs: 0,
      runReply: async (target) => {
        started.push(target.sourceEventId);
        if (started.length === 2) bothStarted();
        if (target.sourceEventId === "message-3") thirdStarted();
        await new Promise<void>((resolve) => {
          finishes.set(target.sourceEventId, resolve);
        });
      },
    },
  );

  const first = await scheduler.accept(
    messageEvent({ direct: true, eventId: "message-1", roomId: "room-1" }),
  );
  const second = await scheduler.accept(
    messageEvent({ direct: true, eventId: "message-2", roomId: "room-2" }),
  );
  const third = await scheduler.accept(
    messageEvent({ direct: true, eventId: "message-3", roomId: "room-3" }),
  );
  await bothStartedGate;
  assert.deepEqual(started, ["message-1", "message-2"]);
  finishes.get("message-1")?.();
  await thirdStartedGate;
  finishes.get("message-2")?.();
  finishes.get("message-3")?.();
  await Promise.all([first.completion, second.completion, third.completion]);

  assert.deepEqual(started, ["message-1", "message-2", "message-3"]);
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
    threadRootEventId: "thread-root-1",
    trigger: "direct_mention" as const,
  };

  await refreshTypingIndicator(api, target, controller.signal, 0);

  assert.equal(updates, 3);
});

test("does not post a message when generation fails", async () => {
  let posts = 0;
  const api: BotAPI = {
    viewerId: BOT_ID,
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
  const scheduler = new ConversationReplyScheduler(
    api,
    ai,
    new AbortController().signal,
    { settleIntervalMs: 0 },
  );
  const accepted = await scheduler.accept(
    messageEvent({ direct: true, threadRootEventId: "thread-root-1" }),
  );

  await assert.rejects(accepted.completion, /provider failed/);

  assert.equal(posts, 0);
});
