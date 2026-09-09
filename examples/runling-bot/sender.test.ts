import assert from "node:assert/strict";
import { test } from "node:test";
import { createReplySender } from "./sender.ts";

for (const fails of [false, true]) {
  test(`concurrent final answers share one POST, failure=${fails}`, async () => {
    const posts: string[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const sender = createReplySender(async (text) => {
      posts.push(text);
      await gate;
      if (fails) throw new Error("Response lost");
      return "reply";
    });
    const first = sender.sendFinal("First");
    assert.equal(sender.sendFinal("Second"), first);
    await sender.notifyFailure();
    assert.deepEqual(posts, ["First"]);
    assert.equal(sender.id, undefined);
    release();
    if (fails) await assert.rejects(first, /Response lost/);
    else assert.equal(await first, "reply");
    await sender.notifyFailure();
    assert.equal(posts.length, 1);
    assert.equal(sender.id, fails ? undefined : "reply");
  });
}

for (const fails of [false, true]) {
  test(`error notification gets one attempt, failure=${fails}`, async () => {
    const posts: object[] = [];
    const sender = createReplySender(async (text, stepName) => {
      posts.push({ text, stepName });
      if (fails) throw new Error("HTTP failed");
      return "notification";
    });
    await Promise.all([sender.notifyFailure(), sender.notifyFailure()]);
    await sender.notifyFailure();
    assert.deepEqual(posts, [
      {
        text: "Sorry, I couldn't generate a reply. Please try again.",
        stepName: "Send error reply",
      },
    ]);
    assert.equal(sender.id, undefined);
  });
}

test("empty text does not consume the reply attempt", async () => {
  const sender = createReplySender(async () => "reply");
  await assert.rejects(sender.sendFinal("  "), /empty/);
  assert.equal(await sender.sendFinal("Hello"), "reply");
});
