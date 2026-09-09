import assert from "node:assert/strict";
import { test } from "node:test";
import {
  createRunling,
  type AgentExtensionAPI,
  type RunlingAgent,
  type Runling,
} from "runling";
import { createReplySender } from "./sender.ts";
import { generateReply } from "./agent.ts";

type Options = Parameters<Runling["agent"]>[0];
type Event = Parameters<NonNullable<Options["onEvent"]>>[0];

/** Exercise the event boundary without a model request or credentials. */
async function fixture(
  run: (
    emit: (event: object) => void,
    restart: () => Promise<void>,
  ) => Promise<object>,
  post: (text: string) => Promise<string> = async () => "reply",
) {
  const previous = process.env.OPENROUTER_API_KEY;
  process.env.OPENROUTER_API_KEY = "test-only";
  let disposed = false;
  const posted: string[] = [];
  const sender = createReplySender(async (text) => {
    posted.push(text);
    return post(text);
  });
  const r = {
    ...createRunling({ cwd: process.cwd(), prompt: "", verbose: false }),
  };
  r.agent = async (options) => {
    assert.equal(options.model, "openrouter/google/gemini-2.5-flash-lite");
    assert.equal(options.thinkingLevel, "off");
    assert.deepEqual(options.tools, ["read_thread", "web_fetch"]);
    assert.deepEqual(options.resources, {
      extensions: false,
      skills: false,
      promptTemplates: false,
      themes: false,
      contextFiles: false,
    });
    let beforeStart: (() => Promise<{ systemPrompt: string }>) | undefined;
    const extension = options.extensions?.[0];
    assert.equal(typeof extension, "function");
    const registered: string[] = [];
    await (extension as (api: AgentExtensionAPI) => unknown)({
      on(name: string, handler: typeof beforeStart) {
        if (name === "before_agent_start") beforeStart = handler;
      },
      setActiveTools() {
        assert.fail(
          "Do not override Runling's tool availability on any prompt",
        );
      },
      registerTool(tool: { name: string }) {
        registered.push(tool.name);
      },
    } as unknown as AgentExtensionAPI);
    assert.deepEqual(registered, ["read_thread"]);
    const restart = async () => {
      const result = await beforeStart!();
      assert.match(result.systemPrompt, /report_outcome/);
      assert.doesNotMatch(result.systemPrompt, /send_reply/);
    };
    return {
      async runOutcome() {
        await restart();
        return run((event) => options.onEvent!(event as Event), restart);
      },
      dispose() {
        disposed = true;
      },
    } as unknown as RunlingAgent;
  };
  let error: unknown;
  try {
    await generateReply(r, {
      message: "Hello",
      readThread: async () => [],
      sender,
    });
  } catch (caught) {
    error = caught;
  } finally {
    if (previous === undefined) delete process.env.OPENROUTER_API_KEY;
    else process.env.OPENROUTER_API_KEY = previous;
  }
  assert.equal(disposed, true);
  return { posted, sender, error };
}

const textEvent = (text: string) => ({
  type: "message_end",
  message: { role: "assistant", content: [{ type: "text", text }] },
});
const reportEvent = {
  type: "tool_execution_end",
  toolName: "report_outcome",
  isError: false,
};

for (const details of ["Full answer", undefined, "   "]) {
  test(`ignores intermediate text and posts only final ${JSON.stringify(details)}`, async () => {
    const result = await fixture(async (emit) => {
      emit({
        type: "message_update",
        assistantMessageEvent: { type: "text_delta", delta: "partial" },
      });
      emit({
        type: "message_end",
        message: { role: "user", content: "private input" },
      });
      emit({
        type: "message_end",
        message: {
          role: "toolResult",
          content: [{ type: "text", text: "tool output" }],
        },
      });
      emit({
        type: "message_end",
        message: {
          role: "assistant",
          content: [
            { type: "thinking", thinking: "private reasoning" },
            { type: "toolCall", name: "web_fetch", arguments: {} },
            { type: "text", text: "First update" },
          ],
        },
      });
      emit(textEvent("  "));
      emit(textEvent("Second update"));
      emit(reportEvent);
      return { outcome: "completed", summary: "Short answer", details };
    });
    assert.equal(result.error, undefined);
    assert.deepEqual(result.posted, [details?.trim() || "Short answer"]);
    assert.equal(result.sender.id, "reply");
  });
}

test("recovery prompts preserve reporting and do not replay interim messages", async () => {
  const result = await fixture(async (emit, restart) => {
    emit(textEvent("Interim update"));
    await restart(); // Runling's missing-report recovery calls session.prompt again.
    emit(reportEvent);
    return { outcome: "completed", summary: "Final answer" };
  });
  assert.equal(result.error, undefined);
  assert.deepEqual(result.posted, ["Final answer"]);
});

test("final text is posted once even when the agent emitted it earlier", async () => {
  const result = await fixture(async (emit) => {
    emit(textEvent("The answer"));
    emit(reportEvent);
    return { outcome: "completed", summary: "The answer" };
  });
  assert.equal(result.error, undefined);
  assert.deepEqual(result.posted, ["The answer"]);
  assert.equal(result.sender.id, "reply");
});

for (const outcome of ["blocked", "failed"]) {
  test(`delivers a valid ${outcome} answer without a second error notification`, async () => {
    const result = await fixture(async (emit) => {
      emit(reportEvent);
      return { outcome, summary: "Please supply the missing information" };
    });
    assert.match(String(result.error), new RegExp(outcome));
    await result.sender.notifyFailure();
    assert.deepEqual(result.posted, ["Please supply the missing information"]);
  });
}

for (const mode of ["missing", "invalid", "exception"]) {
  test(`${mode} report hides intermediate text and sends only an error`, async () => {
    const result = await fixture(async (emit) => {
      emit(textEvent("Interim update"));
      if (mode === "exception") throw new Error("Model disconnected");
      if (mode === "invalid") emit({ ...reportEvent, isError: true });
      return { outcome: "failed", summary: "Internal missing-report error" };
    });
    assert.ok(result.error);
    assert.equal(result.sender.id, undefined);
    await result.sender.notifyFailure();
    assert.deepEqual(result.posted, [
      "Sorry, I couldn't generate a reply. Please try again.",
    ]);
  });
}

test("does not retry a failed final POST and disposes", async () => {
  const result = await fixture(
    async (emit) => {
      emit(textEvent("First update"));
      emit(textEvent("Second update"));
      emit(reportEvent);
      return { outcome: "completed", summary: "Final answer" };
    },
    async () => {
      throw new Error("HTTP failed");
    },
  );
  assert.match(String(result.error), /HTTP failed/);
  await result.sender.notifyFailure();
  assert.deepEqual(result.posted, ["Final answer"]);
});
