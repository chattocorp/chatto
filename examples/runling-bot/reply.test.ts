import assert from "node:assert/strict";
import { test } from "node:test";
import { runWorkflow } from "runling";
import { createReplyWorkflow } from "./reply.ts";

const input = {
  version: 1 as const,
  type: "message.created" as const,
  id: "delivery",
  triggers: ["mention" as const],
  occurred_at: "2026-09-06T12:00:00Z",
  bot_id: "bot",
  room_id: "room",
  thread_root_id: null,
  message: { id: "source", author_id: "human", body: "Hello" },
};

function fixture(
  options: {
    wrongBot?: boolean;
    botAuthor?: boolean;
    postStatus?: number;
    modelFailure?: boolean;
    typingFailure?: boolean;
    duplicateFinal?: boolean;
    noSend?: boolean;
    failAfterSend?: boolean;
    duringComposition?: () => Promise<void>;
  } = {},
) {
  const posts: object[] = [];
  const contexts: unknown[] = [];
  const typing: unknown[] = [];
  const workflow = createReplyWorkflow(
    async () => ({ serverUrl: "http://chatto.test", apiKey: "test-secret" }),
    async (url, init) => {
      assert.equal(
        new Headers(init?.headers).get("Authorization"),
        "Bearer test-secret",
      );
      assert.equal(init?.redirect, "error");
      const path = new URL(String(url)).pathname;
      if (path.endsWith("GetViewer"))
        return Response.json({
          user: { profile: { id: options.wrongBot ? "other" : "bot" } },
        });
      if (path.endsWith("GetUser"))
        return Response.json({
          user: { user: { isBot: !!options.botAuthor } },
        });
      if (path.endsWith("RefreshTypingIndicator")) {
        typing.push(JSON.parse(String(init?.body)));
        return Response.json(
          {},
          { status: options.typingFailure ? 503 : 200 },
        );
      }
      if (path.endsWith("GetThreadEvents")) {
        const body = JSON.parse(String(init?.body));
        assert.equal(body.roomId, "room");
        const event = (id: string, body: string, actorId = "human") => ({
          id,
          messagePosted: { message: { body, actorId } },
        });
        return Response.json({
          page: body.before
            ? {
                events: [event("earlier", "An earlier request")],
              }
            : {
                events: [
                  event(body.threadRootEventId, "Thread root"),
                  event("bot-reply", "Previous joke", "bot"),
                  event("source-ping", "@test_bot"),
                ],
                hasOlder: true,
                startCursor: "older-token",
              },
        });
      }
      assert.ok(path.endsWith("CreateMessage"));
      posts.push(JSON.parse(String(init?.body)));
      return Response.json(
        { message: { id: "reply" } },
        { status: options.postStatus ?? 200 },
      );
    },
    async (_r, context) => {
      assert.equal(typing.length, 1);
      contexts.push(context.thread);
      if (options.noSend) return;
      if (options.modelFailure) throw new Error("Model unavailable");
      await options.duringComposition?.();
      if (options.duplicateFinal) {
        await Promise.all([
          context.sender.sendFinal("First reply"),
          context.sender.sendFinal("Second reply"),
        ]);
        return;
      }
      if (options.failAfterSend) {
        await context.sender.sendFinal("Final answer");
        throw new Error("Late model failure");
      }
      await context.sender.sendFinal(
        "Hello from Runling! I received your webhook and replied through the Chatto API.",
      );
    },
  );
  return { workflow, posts, contexts, typing };
}

test("posts a thread reply through authenticated Connect JSON", async () => {
  const { workflow, posts } = fixture();
  const result = await runWorkflow(workflow, { input });
  assert.equal(result.ok, true);
  assert.deepEqual(result.output, {
    deliveryId: "delivery",
    status: "replied",
    replyId: "reply",
  });
  assert.deepEqual(posts, [
    {
      roomId: "room",
      body: "Hello from Runling! I received your webhook and replied through the Chatto API.",
      threadRootEventId: "source",
      inReplyTo: "source",
    },
  ]);
});

test("continues an existing DM thread", async () => {
  const { workflow, posts } = fixture();
  const result = await runWorkflow(workflow, {
    input: { ...input, triggers: ["direct_message"], thread_root_id: "root" },
  });
  assert.equal(result.ok, true);
  assert.equal(
    (posts[0] as { threadRootEventId: string }).threadRootEventId,
    "root",
  );
});

test("rejects another bot's payload before posting", async () => {
  const { workflow, posts } = fixture({ wrongBot: true });
  assert.equal((await runWorkflow(workflow, { input })).ok, false);
  assert.equal(posts.length, 0);
});

test("skips bot authors to prevent reply loops", async () => {
  const { workflow, posts } = fixture({ botAuthor: true });
  assert.equal(
    (await runWorkflow(workflow, { input })).output?.status,
    "skipped",
  );
  assert.equal(posts.length, 0);
});

test("reports API failures instead of claiming a reply", async () => {
  const { workflow } = fixture({ postStatus: 403 });
  const result = await runWorkflow(workflow, { input });
  assert.equal(result.ok, false);
  assert.equal(result.output, null);
});

test("notifies the user when the model fails and keeps the run failed", async () => {
  const { workflow, posts } = fixture({ modelFailure: true });
  assert.equal((await runWorkflow(workflow, { input })).ok, false);
  assert.deepEqual(posts, [
    {
      roomId: "room",
      body: "Sorry, I couldn't generate a reply. Please try again.",
      threadRootEventId: "source",
      inReplyTo: "source",
    },
  ]);
});

test("channel pings preload every page in order, including the root and bot replies", async () => {
  const { workflow, contexts } = fixture();
  const result = await runWorkflow(workflow, {
    input: {
      ...input,
      thread_root_id: "root",
      message: { ...input.message, body: "@test_bot" },
    },
  });
  assert.equal(result.ok, true);
  assert.deepEqual(contexts, [
    [
      { role: "human", body: "Thread root" },
      { role: "human", body: "An earlier request" },
      { role: "bot", body: "Previous joke" },
      { role: "human", body: "@test_bot" },
    ],
  ]);
});

test("DMs preload their thread even when also mentioned", async () => {
  const { workflow, contexts } = fixture();
  assert.equal(
    (
      await runWorkflow(workflow, {
        input: { ...input, triggers: ["direct_message", "mention"] },
      })
    ).ok,
    true,
  );
  assert.equal((contexts[0] as unknown[]).length, 4);
  assert.deepEqual((contexts[0] as unknown[])[2], {
    role: "bot",
    body: "Previous joke",
  });
});

test("starts typing in the reply thread before composing", async () => {
  const { workflow, typing } = fixture();
  assert.equal(
    (
      await runWorkflow(workflow, {
        input: { ...input, thread_root_id: "root" },
      })
    ).ok,
    true,
  );
  assert.deepEqual(typing, [{ roomId: "room", threadRootEventId: "root" }]);
});

test("typing failures do not prevent a reply", async () => {
  const { workflow, posts } = fixture({ typingFailure: true });
  assert.equal((await runWorkflow(workflow, { input })).ok, true);
  assert.equal(posts.length, 1);
});

test("repeated final delivery shares one POST even when it fails", async () => {
  for (const postStatus of [200, 503]) {
    const { workflow, posts } = fixture({ duplicateFinal: true, postStatus });
    assert.equal(
      (await runWorkflow(workflow, { input })).ok,
      postStatus === 200,
    );
    assert.equal(posts.length, 1);
    assert.equal((posts[0] as { body: string }).body, "First reply");
  }
});

test("a DM thread summary receives earlier messages and bot replies before the model runs", async () => {
  const { workflow, contexts } = fixture();
  const result = await runWorkflow(workflow, {
    input: {
      ...input,
      triggers: ["direct_message"],
      thread_root_id: "root",
      message: { ...input.message, body: "Summarize our thread please" },
    },
  });
  assert.equal(result.ok, true);
  assert.deepEqual(contexts, [
    [
      { role: "human", body: "Thread root" },
      { role: "human", body: "An earlier request" },
      { role: "bot", body: "Previous joke" },
      { role: "human", body: "@test_bot" },
    ],
  ]);
});

test("notifies an existing DM thread when the agent finishes without sending", async () => {
  const { workflow, posts } = fixture({ noSend: true });
  assert.equal(
    (
      await runWorkflow(workflow, {
        input: {
          ...input,
          triggers: ["direct_message"],
          thread_root_id: "root",
        },
      })
    ).ok,
    false,
  );
  assert.deepEqual(posts, [
    {
      roomId: "room",
      body: "Sorry, I couldn't generate a reply. Please try again.",
      threadRootEventId: "root",
      inReplyTo: "source",
    },
  ]);
});

test("does not notify after a final answer or failed POST", async () => {
  const partial = fixture({ failAfterSend: true });
  assert.equal((await runWorkflow(partial.workflow, { input })).ok, false);
  assert.deepEqual(
    partial.posts.map((post) => (post as { body: string }).body),
    ["Final answer"],
  );
  const failed = fixture({ postStatus: 503 });
  assert.equal((await runWorkflow(failed.workflow, { input })).ok, false);
  assert.equal(failed.posts.length, 1);
});

test("does not retry a failed error notification", async () => {
  const { workflow, posts } = fixture({ modelFailure: true, postStatus: 503 });
  assert.equal((await runWorkflow(workflow, { input })).ok, false);
  assert.equal(posts.length, 1);
});

for (const fails of [false, true]) {
  test(`typing continues during composition and stops on failure=${fails}`, async (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const { workflow, posts, typing } = fixture({
      duringComposition: async () => {
        assert.equal(posts.length, 0);
        t.mock.timers.tick(3000);
        assert.equal(typing.length, 2);
        if (fails) throw new Error("Model disconnected");
      },
    });
    assert.equal((await runWorkflow(workflow, { input })).ok, !fails);
    const stoppedAt = typing.length;
    t.mock.timers.tick(30_000);
    assert.equal(typing.length, stoppedAt);
    assert.equal(posts.length, 1);
  });
}
