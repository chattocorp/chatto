import assert from "node:assert/strict";
import { test } from "node:test";
import { createReplySender } from "./sender.ts";

test("concurrent sends and fallback share one attempt, including failure", async () => {
  for (const fails of [false, true]) {
    const posts: string[] = [];
    const sender = createReplySender(async (text) => {
      posts.push(text);
      if (fails) throw new Error("Response lost");
      return "reply";
    });
    const first = sender.send("First");
    assert.equal(sender.send("Second"), first);
    await Promise.allSettled([first]);
    await sender.notifyFailure();
    assert.deepEqual(posts, ["First"]);
    assert.equal(sender.id, fails ? undefined : "reply");
  }
});

test("fallback sends once and contains no original error details", async () => {
  const posts: object[] = [];
  const sender = createReplySender(async (text, stepName) => {
    posts.push({ text, stepName });
    return "notification";
  });
  await sender.notifyFailure();
  await sender.notifyFailure();
  assert.deepEqual(posts, [
    {
      text: "Sorry, I couldn't generate a reply. Please try again.",
      stepName: "Send error reply",
    },
  ]);
});

test("empty text does not consume the reply attempt", async () => {
  const sender = createReplySender(async () => "reply");
  await assert.rejects(sender.send("  "), /empty/);
  assert.equal(await sender.send("Hello"), "reply");
});
