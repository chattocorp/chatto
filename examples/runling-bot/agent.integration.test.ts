import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { createRunling } from "runling";
import { generateReply } from "./agent.ts";
import { createReplySender } from "./sender.ts";

/** A provider response still passes through Runling, Pi, and tool validation. */
function completion(delta: object, finishReason: "stop" | "tool_calls") {
  const chunk = {
    id: "synthetic-completion",
    object: "chat.completion.chunk",
    created: 0,
    model: "google/gemini-2.5-flash-lite",
    choices: [
      {
        index: 0,
        delta: { role: "assistant", ...delta },
        finish_reason: finishReason,
      },
    ],
  };
  return new Response(`data: ${JSON.stringify(chunk)}\n\ndata: [DONE]\n\n`, {
    headers: { "Content-Type": "text/event-stream" },
  });
}

for (const recover of [false, true]) {
  test(`real Runling validates and delivers an outcome, recovery=${recover}`, async (t) => {
    const cwd = await mkdtemp(path.join(os.tmpdir(), "chatto-agent-test-"));
    const env = {
      OPENROUTER_API_KEY: "test-only",
      PI_CODING_AGENT_DIR: cwd,
      PI_OFFLINE: "1",
    };
    const previous = Object.fromEntries(
      Object.keys(env).map((key) => [key, process.env[key]]),
    );
    Object.assign(process.env, env);
    let requests = 0;
    const requestErrors: unknown[] = [];
    const posts: string[] = [];
    // Mock only the provider transport. No agent, extension, or event mocks.
    // Fail closed: an unexpected URL must never reach a real service.
    t.mock.method(
      globalThis,
      "fetch",
      async (url: string | URL | Request, init?: RequestInit) => {
        try {
          assert.equal(
            String(url),
            "https://openrouter.ai/api/v1/chat/completions",
          );
          const body = JSON.parse(String(init?.body));
          const system = body.messages.find(
            (message: { role: string }) => message.role === "system",
          ).content;
          assert.match(system, /outcome is a status, never the answer text/);
          for (const status of ["completed", "blocked", "failed"]) {
            assert.ok(system.includes(`"${status}"`));
          }
          assert.ok(
            system.includes('{"outcome":"completed","summary":"Hello!"}'),
          );
          assert.deepEqual(
            body.tools
              .map((tool: { function: { name: string } }) => tool.function.name)
              .sort(),
            ["read_thread", "report_outcome", "web_fetch"],
          );
          const report = body.tools.find(
            (tool: { function: { name: string } }) =>
              tool.function.name === "report_outcome",
          );
          assert.deepEqual(
            report.function.parameters.properties.outcome.anyOf,
            ["completed", "blocked", "failed"].map((value) => ({
              const: value,
              type: "string",
            })),
          );
          assert.deepEqual(
            posts,
            [],
            "No message is posted before the final report",
          );
          requests++;
          if (recover && requests === 2) {
            assert.ok(
              body.messages.some(
                (message: { role: string; content?: string }) =>
                  message.role === "tool" &&
                  message.content?.includes("Validation failed"),
              ),
            );
            return completion({ content: "Interim update" }, "stop");
          }
          if (recover && requests === 3) {
            assert.match(
              JSON.stringify(body.messages.at(-1).content),
              /calling report_outcome with the truthful outcome/,
            );
          }
          const invalid = recover && requests === 1;
          return completion(
            {
              tool_calls: [
                {
                  index: 0,
                  id: `report-${requests}`,
                  type: "function",
                  function: {
                    name: "report_outcome",
                    arguments: JSON.stringify({
                      outcome: invalid ? "Hello!" : "completed",
                      summary: "Hello!",
                    }),
                  },
                },
              ],
            },
            "tool_calls",
          );
        } catch (error) {
          requestErrors.push(error);
          // Avoid provider retry delays when an assertion fails.
          return new Response(
            '{"error":{"message":"Synthetic request assertion failed"}}',
            { status: 400 },
          );
        }
      },
    );
    try {
      const r = createRunling({ cwd, prompt: "", verbose: false });
      const sender = createReplySender(async (text) => {
        posts.push(text);
        return `message-${posts.length}`;
      });
      let runError: unknown;
      try {
        await generateReply(r, {
          message: "Hello",
          readThread: async () => [],
          sender,
        });
      } catch (error) {
        runError = error;
      }
      assert.deepEqual(requestErrors, []);
      assert.equal(runError, undefined);
      assert.equal(requests, recover ? 3 : 1);
      assert.deepEqual(posts, ["Hello!"]);
      assert.equal(sender.id, `message-${posts.length}`);
    } finally {
      t.mock.restoreAll();
      for (const [key, value] of Object.entries(previous)) {
        if (value === undefined) delete process.env[key];
        else process.env[key] = value;
      }
      await rm(cwd, { recursive: true, force: true });
    }
  });
}
