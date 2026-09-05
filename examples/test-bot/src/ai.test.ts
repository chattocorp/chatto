import assert from "node:assert/strict";
import test from "node:test";
import { createAIResponder } from "./ai.js";

test("runs a Pi session with the web_fetch extension and faux provider", async () => {
  const responder = await createAIResponder({
    provider: "faux",
    fauxResponse: "A generated test reply",
  });

  assert.equal(responder.provider, "faux");
  assert.equal(
    await responder.respond(
      {
        sessionId: "chatto-thread-test",
        turns: [
          { role: "user", content: "Person abc: Earlier question" },
          { role: "assistant", content: "Earlier answer" },
          { role: "user", content: "Person abc: Hello" },
        ],
      },
      new AbortController().signal,
    ),
    "A generated test reply",
  );
});

test("requires an explicit model for a real provider", async () => {
  await assert.rejects(
    createAIResponder({ provider: "anthropic" }),
    /CHATTO_TEST_BOT_AI_MODEL is required/,
  );
});
