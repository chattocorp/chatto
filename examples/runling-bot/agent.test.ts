import assert from "node:assert/strict";
import { test } from "node:test";
import {
  createRunling,
  type AgentExtensionAPI,
  type RunlingAgent,
} from "runling";
import { createReplySender } from "./sender.ts";
import { generateReply } from "./agent.ts";

test("delivery, not the outcome report, determines success", async () => {
  const previous = process.env.OPENROUTER_API_KEY;
  process.env.OPENROUTER_API_KEY = "test-only";
  try {
    for (const mode of [
      "success",
      "missing-report",
      "late-error",
      "no-send",
      "send-failure",
    ]) {
      let disposed = false;
      const posted: string[] = [];
      const r = {
        ...createRunling({ cwd: process.cwd(), prompt: "", verbose: false }),
      };
      r.agent = async (options) => {
        assert.equal(options.model, "openrouter/google/gemini-2.5-flash-lite");
        assert.equal(options.thinkingLevel, "off");
        assert.deepEqual(options.tools, [
          "read_thread",
          "web_fetch",
          "send_reply",
        ]);
        let send:
          | ((id: string, args: { text: string }) => Promise<unknown>)
          | undefined;
        const extension = options.extensions?.[0];
        if (typeof extension !== "function")
          throw new Error("Missing chat extension");
        let beforeStart: (() => Promise<unknown>) | undefined;
        const activeTools: string[][] = [];
        await extension({
          on(name: string, handler: () => Promise<unknown>) {
            if (name === "before_agent_start") beforeStart = handler;
          },
          setActiveTools(names: string[]) {
            activeTools.push(names);
          },
          registerTool(tool: { name: string; execute: typeof send }) {
            if (tool.name === "send_reply") send = tool.execute;
          },
        } as unknown as AgentExtensionAPI);
        return {
          async runOutcome() {
            await beforeStart!();
            assert.deepEqual(activeTools[0], [
              "read_thread",
              "web_fetch",
              "send_reply",
            ]);
            if (mode !== "no-send") await send!("call", { text: " Hey! " });
            if (mode !== "no-send")
              assert.deepEqual(activeTools.at(-1), ["report_outcome"]);
            if (mode === "late-error")
              throw new Error("Model disconnected after sending");
            return {
              outcome: mode === "missing-report" ? "failed" : "completed",
              summary: "Internal report",
            };
          },
          dispose() {
            disposed = true;
          },
        } as unknown as RunlingAgent;
      };
      const result = generateReply(r, {
        message: "sup",
        readThread: async () => [],
        sender: createReplySender(async (text) => {
          if (mode === "send-failure") throw new Error("HTTP failed");
          posted.push(text);
          return "reply-id";
        }),
      });
      if (mode === "no-send" || mode === "send-failure") {
        await assert.rejects(result, /did not send|HTTP failed/);
        assert.deepEqual(posted, []);
      } else {
        await result;
        assert.deepEqual(posted, ["Hey!"]);
      }
      assert.equal(disposed, true);
    }
  } finally {
    if (previous === undefined) delete process.env.OPENROUTER_API_KEY;
    else process.env.OPENROUTER_API_KEY = previous;
  }
});
