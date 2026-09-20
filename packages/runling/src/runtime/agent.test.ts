import { createWorkflowContext } from "./context.ts";
import { ansiColor } from "./ansi.ts";
import { AsyncResource } from "node:async_hooks";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { observeRunlingEvents, type RunlingEvent } from "./events.ts";

const fakeModel = {
  id: "claude-opus-4-5",
  name: "Claude Opus 4.5",
  provider: "anthropic",
};

const emptyUsage = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };

let eventHandler: ((event: any) => void) | undefined;
let promptImplementation: ((prompt: string) => Promise<void>) | undefined;
let sessionOptions: any;
let resourceOptions: any;
let settingsCreateArgs: unknown[] | undefined;
let settingsOverrides: any;
let promptCalls = 0;
let sessionCreations = 0;
let createdSessions: any[] = [];
let abortCalls = 0;
let disposed = false;
let modelAvailable = true;

vi.doMock("@earendil-works/pi-coding-agent", async () => {
  const actual = await vi.importActual<typeof import("@earendil-works/pi-coding-agent")>("@earendil-works/pi-coding-agent");
  return {
    convertToLlm: actual.convertToLlm,
    ModelRuntime: {
      create: async () => ({
        getModel: () => (modelAvailable ? fakeModel : undefined),
      }),
    },
    DefaultResourceLoader: class {
      constructor(options: unknown) {
        resourceOptions = options;
      }
      async reload() {}
    },
    getAgentDir: () => "/tmp/agent-dir",
    SettingsManager: {
      create: (...args: unknown[]) => {
        settingsCreateArgs = args;
        return {
          getRetrySettings: () => ({
            enabled: true,
            maxRetries: 3,
            baseDelayMs: 2000,
          }),
          applyOverrides: (overrides: unknown) => {
            settingsOverrides = overrides;
          },
        };
      },
    },
    SessionManager: actual.SessionManager,
    createAgentSession: async (options: unknown) => {
      sessionCreations++;
      sessionOptions = options;
      let messages: any[] = sessionOptions.sessionManager.buildSessionContext().messages;
      const session = {
        sessionManager: sessionOptions.sessionManager,
        agent: {
          steer: vi.fn(),
          clearSteeringQueue: vi.fn(),
          state: {
            get messages() {
              return messages;
            },
            set messages(next: any[]) {
              messages = [...next];
            },
          },
        },
        subscribe(handler: (event: any) => void) {
          eventHandler = handler;
          return () => {
            if (eventHandler === handler) eventHandler = undefined;
          };
        },
        prompt: async (prompt: string) => {
          promptCalls++;
          await promptImplementation?.(prompt);
        },
        abort: async () => {
          abortCalls++;
        },
        dispose: () => {
          disposed = true;
        },
      };
      createdSessions.push(session);
      return { session };
    },
    defineTool: (tool: unknown) => tool,
  };
});

const { agent, AgentOutcomeError, describeTool, runAgent } =
  await import("./agent.ts");

const { log } = await import("./log.ts");

beforeEach(() => {
  eventHandler = undefined;
  promptImplementation = undefined;
  sessionOptions = undefined;
  resourceOptions = undefined;
  settingsCreateArgs = undefined;
  settingsOverrides = undefined;
  promptCalls = 0;
  sessionCreations = 0;
  createdSessions = [];
  abortCalls = 0;
  disposed = false;
  modelAvailable = true;
});

async function reportOutcome(report: {
  outcome: "completed" | "blocked" | "failed";
  summary: string;
  details?: string;
}) {
  await sessionOptions.customTools[0].execute("tool-call", report);
}

function emitAssistantText(text: string) {
  eventHandler?.({
    type: "message_end",
    message: { role: "assistant", content: [{ type: "text", text }] },
  });
}

function stripAnsi(text: string): string {
  return text.replace(/\x1b\[[0-9;]*m/g, "");
}

function emitAssistantUsage(usage: {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  cost?: { total: number };
}) {
  eventHandler?.({
    type: "message_end",
    message: {
      role: "assistant",
      content: [],
      usage,
    },
  });
}

test("streams bounded text previews, throttles deltas, and shows tools without debug noise", async () => {
  const events: RunlingEvent[] = [];
  const clock = vi.spyOn(performance, "now").mockReturnValue(1000);
  promptImplementation = async () => {
    eventHandler?.({ type: "agent_start" });
    const update = (type: string, text: string) => eventHandler?.({
      type: "message_update",
      assistantMessageEvent: { type, contentIndex: 0, partial: { content: [{ type: "text", text }] } },
    });
    update("text_start", "");
    update("text_delta", "Looking\n at files");
    update("text_delta", "This update is throttled");
    update("text_end", "x".repeat(600));
    eventHandler?.({ type: "tool_execution_start", toolCallId: "read-1", toolName: "read", args: { path: "src/foo.ts" } });
    eventHandler?.({ type: "turn_start" });
    await reportOutcome({ outcome: "completed", summary: "Done" });
  };
  try {
    await observeRunlingEvents(e => events.push(e), () => runAgent(createWorkflowContext(), "Test", { cwd: "/project", model: "anthropic/claude-opus-4-5" }));
    const previews = events.filter(e => e.type === "agent.progress").map(e => e.text);
    expect(previews).toContain("Looking at files");
    expect(previews).not.toContain("This update is throttled");
    expect(previews).toContain("x".repeat(500));
    expect(previews).toContain("Reading src/foo.ts");
    expect(previews.at(-1)).toBe("Done");
  } finally { clock.mockRestore(); }
});

test("previews report-only output such as the joke workflow", async () => {
  const events: RunlingEvent[] = [];
  // The SDK can dispatch from an async resource created outside the run.
  const sdk = new AsyncResource("sdk-dispatch");
  promptImplementation = async () => {
    sdk.runInAsyncScope(() => eventHandler?.({ type: "agent_start" }));
    for (const type of ["toolcall_start", "toolcall_delta", "toolcall_end"]) {
      sdk.runInAsyncScope(() => eventHandler?.({
        type: "message_update",
        assistantMessageEvent: {
          type, contentIndex: 0,
          partial: { content: [{ type: "toolCall", name: "report_outcome", arguments: {} }] },
        },
      }));
    }
    await sdk.runInAsyncScope(() => reportOutcome({ outcome: "completed", summary: "Joke written", details: "A joke\nwith a punchline." }));
  };
  await observeRunlingEvents(e => events.push(e), () => runAgent(createWorkflowContext(), "Write a joke", { cwd: "/project", model: "anthropic/claude-opus-4-5", tools: [] }));
  const previews = events.filter(e => e.type === "agent.progress").map(e => e.text);
  expect(previews.filter(text => text === "Preparing result…")).toHaveLength(1);
  expect(previews.at(-1)).toBe("A joke with a punchline.");
  expect(events.some(e => e.type === "agent.action" && e.action === "A joke\nwith a punchline.")).toBe(true);
});

describe("describeTool", () => {
  test("describes known tools", () => {
    expect(describeTool("read", { path: "src/foo.ts" })).toBe(
      "Reading src/foo.ts",
    );
    expect(describeTool("edit", { path: "src/foo.ts" })).toBe(
      "Editing src/foo.ts",
    );
    expect(describeTool("write", { path: "src/foo.ts" })).toBe(
      "Writing src/foo.ts",
    );
    expect(describeTool("bash", { command: "pnpm test" })).toBe(
      "Running pnpm test",
    );
    expect(describeTool("report_outcome", {})).toBe("Using report_outcome");
  });

  test("falls back to a generic message for unknown tools", () => {
    expect(describeTool("grep", { pattern: "foo" })).toBe("Using grep");
  });
});

describe("runAgent", () => {
  test("uses a distinct, consistent color for each agent", async () => {
    const withColor = vi.spyOn(log, "withColor");
    const colorize = vi.spyOn(log, "colorize");
    promptImplementation = async () => {
      eventHandler?.({ type: "agent_start" });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    try {
      await runAgent(createWorkflowContext(), "Do the first thing", { cwd: "/project",
        model: "anthropic/claude-opus-4-5",
      });
      const firstAgentColors = withColor.mock.calls.map((call) => call[0]);
      withColor.mockClear();

      await runAgent(createWorkflowContext(), "Do the second thing", { cwd: "/project",
        model: "anthropic/claude-opus-4-5",
      });

      const secondAgentColors = withColor.mock.calls.map((call) => call[0]);
      expect(new Set(firstAgentColors).size).toBe(1);
      expect(new Set(secondAgentColors).size).toBe(1);
      expect(firstAgentColors[0]).not.toBe(secondAgentColors[0]);
      expect(colorize).toHaveBeenCalled();
      expect(
        colorize.mock.calls.every((call) =>
          /^\[[a-z]+-[a-z]+-\d{4}\]$/.test(call[0]),
        ),
      ).toBe(true);
    } finally {
      colorize.mockRestore();
      withColor.mockRestore();
    }
  });

  test("logs agent events without adding another step or indentation", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);
    promptImplementation = async () => {
      eventHandler?.({ type: "agent_start" });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    try {
      await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" });
    } finally {
      console.log = originalLog;
    }

    const started = logs.find((line) => line.includes("Agent started"));

    expect(logs.some((line) => /Agent [a-z]+-[a-z]+-\d{4}$/.test(line))).toBe(
      false,
    );
    expect(started).toBeDefined();
    expect(started).not.toMatch(/^\s/);
  });

  test("prefixes every log line with one human-friendly agent ID", async () => {
    const logs: string[] = [];
    const errors: string[] = [];
    const originalLog = console.log;
    const originalError = console.error;
    console.log = (message: string) => logs.push(message);
    console.error = (message: string) => errors.push(message);
    promptImplementation = async () => {
      eventHandler?.({ type: "agent_start" });
      eventHandler?.({
        type: "tool_execution_start",
        toolCallId: "read-1",
        toolName: "read",
        args: { path: "src/foo.ts" },
      });
      eventHandler?.({
        type: "tool_execution_end",
        toolCallId: "bash-1",
        toolName: "bash",
        isError: true,
      });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    try {
      await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" });
    } finally {
      console.log = originalLog;
      console.error = originalError;
    }

    const plainLogs = logs.map(stripAnsi);
    const plainErrors = errors.map(stripAnsi);
    const id = plainLogs
      .find((line) => line.includes("Agent started"))
      ?.match(/\[([a-z]+-[a-z]+-\d{4})\]/)?.[1];

    expect(id).toBeDefined();
    expect(
      plainLogs.some((line) => line.includes(`[${id}] Agent started`)),
    ).toBe(true);
    expect(
      plainLogs.some((line) => line.includes(`[${id}] Reading src/foo.ts`)),
    ).toBe(true);
    const reading = logs.find(
      (line) => line.includes("Reading") && line.includes("src/foo.ts"),
    );
    expect(reading).toBeDefined();
    expect(reading).toContain("\x1b[1m");
    expect(reading).toContain(ansiColor("#40c057") ?? "");
    expect(
      reading!.indexOf("\x1b[0m", reading!.indexOf("Reading")),
    ).toBeLessThan(reading!.indexOf("src/foo.ts"));
    expect(
      plainErrors.some((line) => line.includes(`[${id}] bash failed`)),
    ).toBe(true);
  });

  test("logs automatic retry progress", async () => {
    const logs: string[] = [];
    const errors: string[] = [];
    const originalLog = console.log;
    const originalError = console.error;
    console.log = (message: string) => logs.push(stripAnsi(message));
    console.error = (message: string) => errors.push(stripAnsi(message));
    promptImplementation = async () => {
      eventHandler?.({
        type: "auto_retry_start",
        attempt: 1,
        maxAttempts: 5,
        delayMs: 2000,
        errorMessage: "fetch failed\nwhile streaming",
      });
      eventHandler?.({ type: "auto_retry_end", success: true, attempt: 1 });
      eventHandler?.({
        type: "auto_retry_end",
        success: false,
        attempt: 5,
        finalError: "connection lost",
      });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    try {
      await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" });
    } finally {
      console.log = originalLog;
      console.error = originalError;
    }

    expect(
      logs.some((line) =>
        line.includes(
          "Retrying agent in 2s (attempt 1/5): fetch failed while streaming",
        ),
      ),
    ).toBe(true);
    expect(
      logs.some((line) =>
        line.includes("Agent recovered after 1 retry attempt"),
      ),
    ).toBe(true);
    expect(
      errors.some((line) =>
        line.includes(
          "Agent retry failed after 5 retry attempts: connection lost",
        ),
      ),
    ).toBe(true);
  });

  test("logs detailed turn, tool, and session lifecycle events", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    const originalLevel = log.level;
    console.log = (message: string) => logs.push(stripAnsi(message));
    log.level = "debug";
    promptImplementation = async () => {
      eventHandler?.({ type: "turn_start" });
      eventHandler?.({
        type: "tool_execution_start",
        toolCallId: "read-1",
        toolName: "read",
        args: { path: "src/foo.ts" },
      });
      eventHandler?.({
        type: "tool_execution_end",
        toolCallId: "read-1",
        toolName: "read",
        result: {},
        isError: false,
      });
      eventHandler?.({
        type: "turn_end",
        message: { role: "assistant", content: [] },
        toolResults: [{}],
      });
      eventHandler?.({
        type: "agent_end",
        messages: [{ role: "assistant", content: [] }],
        willRetry: false,
      });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    try {
      await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project",
        model: "anthropic/claude-opus-4-5",
      });
    } finally {
      log.level = originalLevel;
      console.log = originalLog;
    }

    expect(logs.some((line) => line.includes("Turn 1 started"))).toBe(true);
    expect(logs.some((line) => line.includes("read finished in"))).toBe(true);
    expect(
      logs.some((line) => line.includes("Turn 1 finished (1 tool result)")),
    ).toBe(true);
    expect(
      logs.some((line) => line.includes("Agent finished (1 message)")),
    ).toBe(true);
  });

  test("exposes pi session events to callers", async () => {
    const events: string[] = [];
    promptImplementation = async () => {
      eventHandler?.({ type: "agent_start" });
      eventHandler?.({ type: "turn_start" });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project",
      model: "anthropic/claude-opus-4-5",
      onEvent: (event) => events.push(event.type),
    });

    expect(events).toEqual(["agent_start", "turn_start"]);
  });

  test("logs compaction and summarization retry progress", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(stripAnsi(message));
    promptImplementation = async () => {
      eventHandler?.({ type: "compaction_start", reason: "threshold" });
      eventHandler?.({
        type: "summarization_retry_scheduled",
        attempt: 1,
        maxAttempts: 5,
        delayMs: 1500,
        errorMessage: "stream terminated\nwhile summarizing",
      });
      eventHandler?.({
        type: "compaction_end",
        reason: "threshold",
        result: {
          summary: "Earlier work",
          firstKeptEntryId: "entry-1",
          tokensBefore: 120000,
          estimatedTokensAfter: 30000,
        },
        aborted: false,
        willRetry: false,
      });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    try {
      await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project",
        model: "anthropic/claude-opus-4-5",
      });
    } finally {
      console.log = originalLog;
    }

    expect(
      logs.some((line) => line.includes("Compacting context (threshold)")),
    ).toBe(true);
    expect(
      logs.some((line) =>
        line.includes(
          "Retrying context summary in 1500ms (attempt 1/5): stream terminated while summarizing",
        ),
      ),
    ).toBe(true);
    expect(
      logs.some((line) =>
        line.includes("Context compacted from 120,000 to about 30,000 tokens"),
      ),
    ).toBe(true);
  });

  test("raises Pi's agent retry budget to at least five", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(createWorkflowContext(), "Do the thing", {
      model: "anthropic/claude-opus-4-5",
      cwd: "/tmp/project",
      resources: { agentDir: "/tmp/custom-agent" },
    });

    expect(settingsCreateArgs).toEqual(["/tmp/project", "/tmp/custom-agent"]);
    expect(settingsOverrides).toEqual({
      retry: { enabled: true, maxRetries: 5, baseDelayMs: 2000 },
    });
    expect(resourceOptions.settingsManager).toBe(
      sessionOptions.settingsManager,
    );
  });

  test("returns the structured outcome reported by the agent", async () => {
    promptImplementation = async () => {
      await reportOutcome({
        outcome: "blocked",
        summary: "Need access",
        details: "The required repository is unavailable.",
      });
    };

    await expect(
      runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
    ).resolves.toEqual({
      outcome: "blocked",
      summary: "Need access",
      details: "The required repository is unavailable.",
      usage: emptyUsage,
    });
    expect(disposed).toBe(true);
  });

  test("does not treat an unreported plain-text response as success", async () => {
    promptImplementation = async () => emitAssistantText("I could not finish");

    await expect(
      runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
    ).resolves.toEqual({
      outcome: "failed",
      summary: "Agent finished without a valid outcome report",
      usage: emptyUsage,
    });
    expect(promptCalls).toBe(2);
  });

  test("recovers a malformed tool call with one corrective turn", async () => {
    promptImplementation = async () => {
      if (promptCalls === 1) {
        emitAssistantText('functions.report_outcome:0{"outcome":"completed"}');
      } else {
        await reportOutcome({ outcome: "completed", summary: "Done" });
      }
    };

    await expect(
      runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
    ).resolves.toEqual({
      outcome: "completed",
      summary: "Done",
      usage: emptyUsage,
    });
    expect(promptCalls).toBe(2);
  });

  test("returns and logs accumulated token usage including cached tokens", async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 100,
        output: 20,
        cacheRead: 500,
        cacheWrite: 10,
      });
      emitAssistantUsage({
        input: 50,
        output: 25,
        cacheRead: 550,
        cacheWrite: 15,
      });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    try {
      await expect(
        runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
      ).resolves.toEqual({
        outcome: "completed",
        summary: "Done",
        usage: {
          input: 150,
          costIncomplete: true,
          output: 45,
          cacheRead: 1050,
          cacheWrite: 25,
        },
      });
    } finally {
      console.log = originalLog;
    }

    expect(
      logs.some((line) =>
        line.includes(
          "Token usage: in 150, out 45, cache read 1,050, cache write 25",
        ),
      ),
    ).toBe(true);
  });

  test("records usage into the workflow-wide totals", async () => {
    const ctx = createWorkflowContext();
    promptImplementation = async () => {
      emitAssistantUsage({ input: 10, output: 5, cacheRead: 0, cacheWrite: 0 });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(ctx, "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" });

    expect(ctx.usage).toEqual({
      input: 10,
      output: 5,
      cacheRead: 0,
      cacheWrite: 0,
      costIncomplete: true,
    });
  });

  test("emits immutable live usage snapshots before an agent finishes", async () => {
    const events: RunlingEvent[] = [];
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 10,
        output: 2,
        cacheRead: 5,
        cacheWrite: 0,
        cost: { total: 0.01 },
      });
      expect(events.filter((e) => e.type === "agent.usage")).toHaveLength(1);
      expect(events.some((e) => e.type === "agent.finished")).toBe(false);
      emitAssistantUsage({
        input: 20,
        output: 3,
        cacheRead: 7,
        cacheWrite: 1,
        cost: { total: 0.02 },
      });
      throw new Error("Provider unavailable");
    };
    await expect(
      observeRunlingEvents(
        (e) => events.push(e),
        () => runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
      ),
    ).rejects.toThrow("Provider unavailable");
    const snapshots = events.filter((e) => e.type === "agent.usage");
    expect(snapshots.map((e) => e.usage.input)).toEqual([10, 30]);
    expect(snapshots[1]?.usage.cost).toBeCloseTo(0.03);
    const finished = events.find((e) => e.type === "agent.finished");
    expect(finished?.usage).toEqual(snapshots[1]?.usage);
  });

  test("records tokens and cost as each model turn completes", async () => {
    const ctx = createWorkflowContext();
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 10,
        output: 5,
        cacheRead: 20,
        cacheWrite: 1,
        cost: { total: 0.0125 },
      });
      expect(ctx.usage).toEqual({
        input: 10,
        output: 5,
        cacheRead: 20,
        cacheWrite: 1,
        cost: 0.0125,
      });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(ctx, "Do the thing", { cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });
  });

  test("returns token usage even when no outcome is reported", async () => {
    promptImplementation = async () => {
      if (promptCalls === 1) {
        emitAssistantUsage({
          input: 7,
          output: 3,
          cacheRead: 0,
          cacheWrite: 0,
        });
        emitAssistantText("I could not finish");
      }
    };

    await expect(
      runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
    ).resolves.toEqual({
      outcome: "failed",
      summary: "Agent finished without a valid outcome report",
      usage: {
        input: 7,
        output: 3,
        cacheRead: 0,
        cacheWrite: 0,
        costIncomplete: true,
      },
    });
  });

  test("disposes the session when prompting fails", async () => {
    promptImplementation = async () => {
      throw new Error("Provider unavailable");
    };

    await expect(
      runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
    ).rejects.toThrow("Provider unavailable");
    expect(disposed).toBe(true);
  });

  test("records usage when prompting fails", async () => {
    const ctx = createWorkflowContext();
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 10,
        output: 5,
        cacheRead: 20,
        cacheWrite: 1,
      });
      throw new Error("Provider unavailable");
    };

    await expect(
      runAgent(ctx, "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" }),
    ).rejects.toThrow("Provider unavailable");
    expect(ctx.usage).toEqual({
      input: 10,
      output: 5,
      cacheRead: 20,
      cacheWrite: 1,
      costIncomplete: true,
    });
  });

  test("rejects unavailable models before creating a session", async () => {
    modelAvailable = false;

    await expect(
      runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/missing" }),
    ).rejects.toThrow("Model anthropic/missing is unavailable");
    expect(sessionOptions).toBeUndefined();
  });

  test("forwards the requested thinking level to the session", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project",
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "medium",
    });

    expect(sessionOptions.thinkingLevel).toBe("medium");
  });

  test("leaves the thinking level to pi defaults when unspecified", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project", model: "anthropic/claude-opus-4-5" });

    expect(sessionOptions.thinkingLevel).toBeUndefined();
  });

  test("loads and enables Runling's bundled web fetch extension", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(createWorkflowContext(), "Research the topic", { cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });

    expect(sessionOptions.tools).toEqual([
      "read",
      "bash",
      "edit",
      "write",
      "web_fetch",
      "report_outcome",
    ]);
    expect(resourceOptions.extensionFactories).toEqual([
      {
        name: "runling-web-fetch",
        factory: expect.any(Function),
      },
    ]);
  });

  test("loads explicitly supplied extensions when configured extensions are disabled", async () => {
    const localExtension = {
      name: "local-validation",
      factory() {},
    };
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };

    await runAgent(createWorkflowContext(), "Implement the change", { cwd: "/project",
      model: "anthropic/claude-opus-4-5",
      extensions: [localExtension],
      resources: { extensions: false },
    });

    expect(resourceOptions.noExtensions).toBe(true);
    expect(resourceOptions.extensionFactories).toEqual([localExtension]);
    expect(sessionOptions.tools).not.toContain("web_fetch");
  });

  test("enables only explicit web tools with resource discovery disabled", async () => {
    const { default: webFetchExtension } = await import("../../extensions/web-fetch.ts");
    const extension = { name: "runling-web-fetch", factory: webFetchExtension };
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Fetched" });
    };

    await runAgent(createWorkflowContext(), "Read a public URL", {
      cwd: "/project",
      model: "anthropic/claude-opus-4-5",
      tools: ["web_fetch"],
      extensions: [extension],
      resources: {
        extensions: false,
        skills: false,
        promptTemplates: false,
        themes: false,
        contextFiles: false,
      },
    });

    expect(sessionOptions.tools).toEqual(["web_fetch", "report_outcome"]);
    expect(resourceOptions).toMatchObject({
      extensionFactories: [extension],
      noExtensions: true,
      noSkills: true,
      noPromptTemplates: true,
      noThemes: true,
      noContextFiles: true,
    });
  });

  test("applies execution and resource boundaries", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Reviewed" });
    };

    await runAgent(createWorkflowContext(), "Review", {
      model: "anthropic/claude-opus-4-5",
      cwd: "/tmp/project",
      tools: ["read"],
      resources: {
        agentDir: "/tmp/agent",
        extensions: false,
        skills: false,
        promptTemplates: false,
        themes: false,
        contextFiles: false,
      },
    });

    expect(sessionOptions.cwd).toBe("/tmp/project");
    expect(sessionOptions.tools).toEqual(["read", "report_outcome"]);
    expect(resourceOptions).toMatchObject({
      cwd: "/tmp/project",
      agentDir: "/tmp/agent",
      noExtensions: true,
      noSkills: true,
      noPromptTemplates: true,
      noThemes: true,
      noContextFiles: true,
      extensionFactories: [],
    });
    expect(resourceOptions.appendSystemPromptOverride(["Base prompt"])).toEqual(
      [
        "Base prompt",
        expect.stringContaining("non-interactive software factory"),
      ],
    );
  });

  test("aborts and disposes an active session", async () => {
    const controller = new AbortController();
    promptImplementation = async () => controller.abort("Stopped");

    await expect(
      runAgent(createWorkflowContext(), "Do the thing", { cwd: "/project",
        model: "anthropic/claude-opus-4-5",
        signal: controller.signal,
      }),
    ).rejects.toBe("Stopped");
    expect(abortCalls).toBe(1);
    expect(disposed).toBe(true);
  });
});

describe("agent", () => {
  test("forks the current conversation into an independent session", async () => {
    const instance = await agent({ cwd: "/project",
      model: "anthropic/claude-opus-4-5",
      tools: ["read"],
    });
    createdSessions[0].agent.state.messages = [
      { role: "user", content: [{ type: "text", text: "Investigate" }] },
    ];

    const fork = await instance.fork();

    try {
      expect(fork.id).not.toBe(instance.id);
      expect(sessionCreations).toBe(2);
      expect(createdSessions[1].agent.state.messages).toEqual(
        createdSessions[0].agent.state.messages,
      );
      expect(createdSessions[1].sessionManager.buildSessionContext().messages).toEqual(
        createdSessions[0].agent.state.messages,
      );

      createdSessions[1].agent.state.messages.push({ role: "assistant" });
      expect(createdSessions[0].agent.state.messages).toHaveLength(1);
      expect(createdSessions[1].agent.state.messages).toHaveLength(2);
      expect(sessionOptions.tools).toEqual(["read", "report_outcome"]);
    } finally {
      fork.dispose();
      instance.dispose();
    }
  });

  test("keeps inherited summaries in Pi history across further compaction", async () => {
    const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
    createdSessions[0].agent.state.messages = [
      { role: "compactionSummary", summary: "Investigation found a race condition", tokensBefore: 5000, timestamp: 1 },
      { role: "user", content: "Review the race condition", timestamp: 2 },
    ];
    const fork = await instance.fork();
    try {
      const manager = createdSessions[1].sessionManager;
      // Pi's compactor reads the persisted branch, not agent.state.messages.
      const branch = manager.getBranch();
      expect(JSON.stringify(branch)).toContain("Investigation found a race condition");
      expect(JSON.stringify(branch)).toContain("Review the race condition");
      const inherited = branch.find((entry: any) => entry.type === "message");
      manager.appendMessage({ role: "user", content: "Continue the review", timestamp: 3 });
      manager.appendCompaction("Earlier work", inherited.id, 6000);
      const restored = manager.buildSessionContext().messages;
      expect(JSON.stringify(restored)).toContain("Investigation found a race condition");
      expect(JSON.stringify(restored)).toContain("Continue the review");
      expect(createdSessions[0].agent.state.messages).toHaveLength(2);
    } finally {
      fork.dispose();
      instance.dispose();
    }
  });

  test("does not fork a running or disposed agent", async () => {
    let instance: Awaited<ReturnType<typeof agent>>;
    promptImplementation = async () => {
      await expect(instance.fork()).rejects.toThrow("already running");
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };
    instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });

    await instance.run(createWorkflowContext(), "Work");
    instance.dispose();

    await expect(instance.fork()).rejects.toThrow("has been disposed");
  });

  test("retains one session across sequential runs", async () => {
    promptImplementation = async (prompt) => {
      await reportOutcome({ outcome: "completed", summary: prompt });
    };
    const instance = await agent({ cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });

    try {
      expect(instance.id).toMatch(/^[a-z]+-[a-z]+-\d{4}$/);
      await expect(instance.run(createWorkflowContext(), "Investigate")).resolves.toMatchObject({
        outcome: "completed",
        summary: "Investigate",
      });
      await expect(instance.run(createWorkflowContext(), "Implement")).resolves.toMatchObject({
        outcome: "completed",
        summary: "Implement",
      });

      expect(sessionCreations).toBe(1);
      expect(promptCalls).toBe(2);
      expect(disposed).toBe(false);
    } finally {
      instance.dispose();
    }

    expect(disposed).toBe(true);
  });

  test("uses fresh outcome state for every run", async () => {
    promptImplementation = async (prompt) => {
      await reportOutcome(
        prompt === "First"
          ? { outcome: "completed", summary: "First result" }
          : { outcome: "blocked", summary: "Second result" },
      );
    };
    const instance = await agent({ cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });

    try {
      await expect(instance.run(createWorkflowContext(), "First")).resolves.toMatchObject({
        outcome: "completed",
        summary: "First result",
      });
      await expect(instance.runOutcome(createWorkflowContext(), "Second")).resolves.toMatchObject({
        outcome: "blocked",
        summary: "Second result",
      });
    } finally {
      instance.dispose();
    }
  });

  test("rejects concurrent runs", async () => {
    let release: (() => void) | undefined;
    promptImplementation = async () => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };
    const instance = await agent({ cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });

    try {
      const first = instance.run(createWorkflowContext(), "First");
      await expect(instance.run(createWorkflowContext(), "Second")).rejects.toThrow("already running");
      release?.();
      await first;
    } finally {
      instance.dispose();
    }
  });

  test("rejects runs after disposal", async () => {
    const instance = await agent({ cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });
    instance.dispose();

    await expect(instance.run(createWorkflowContext(), "Too late")).rejects.toThrow("has been disposed");
  });

  test("can run again after an aborted turn", async () => {
    const controller = new AbortController();
    promptImplementation = async () => controller.abort("Stopped");
    const instance = await agent({ cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });

    try {
      await expect(
        instance.run(createWorkflowContext(), "First", { signal: controller.signal }),
      ).rejects.toBe("Stopped");
      expect(disposed).toBe(false);

      promptImplementation = async () => {
        await reportOutcome({ outcome: "completed", summary: "Recovered" });
      };
      await expect(instance.run(createWorkflowContext(), "Second")).resolves.toMatchObject({
        outcome: "completed",
        summary: "Recovered",
      });
    } finally {
      instance.dispose();
    }
  });

  test("supports automatic disposal", async () => {
    {
      await using instance = await agent({ cwd: "/project",
        model: "anthropic/claude-opus-4-5",
      });
      expect(disposed).toBe(false);
      expect(instance.id).toBeTypeOf("string");
    }

    expect(disposed).toBe(true);
  });

  test("throws unsuccessful outcomes while preserving their report", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: "blocked", summary: "Need input" });
    };
    await using instance = await agent({ cwd: "/project",
      model: "anthropic/claude-opus-4-5",
    });

    try {
      await instance.run(createWorkflowContext(), "Do the thing");
      throw new Error("Expected run to fail");
    } catch (error) {
      expect(error).toBeInstanceOf(AgentOutcomeError);
      expect((error as Error).message).toBe("Need input");
      expect((error as { report: unknown }).report).toEqual({
        outcome: "blocked",
        summary: "Need input",
        usage: emptyUsage,
      });
    }
  });
});

test("requires an explicit directory before creating an agent", async () => {
  // @ts-expect-error Agents require an explicit directory.
  await expect(agent({ model: "anthropic/claude-opus-4-5" })).rejects.toThrow("An explicit directory is required");
  await expect(agent({ model: "anthropic/claude-opus-4-5", cwd: "" })).rejects.toThrow("An explicit directory is required");
});

test("attributes each reusable agent interaction to its supplied context", async () => {
  const first = createWorkflowContext();
  const second = createWorkflowContext();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    await reportOutcome({ outcome: "completed", summary: "Done" });
  };
  const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  try {
    const one = await instance.run(first, "First");
    const two = await instance.runOutcome(second, "Second");
    expect(one.usage.input).toBe(10);
    expect(two.usage.input).toBe(10);
    expect(first.usage).toEqual({ ...emptyUsage, input: 10, cost: 0.25 });
    expect(second.usage).toEqual(first.usage);
    await instance.run(second, "Third");
    expect(first.usage.input).toBe(10);
    expect(second.usage).toEqual({ ...emptyUsage, input: 20, cost: 0.5 });
  } finally {
    instance.dispose();
  }
});

test("records fork usage only in the context passed to the fork interaction", async () => {
  const parentContext = createWorkflowContext();
  const childContext = createWorkflowContext();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    await reportOutcome({ outcome: "completed", summary: "Done" });
  };
  const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  try {
    await instance.run(parentContext, "Investigate");
    const fork = await instance.fork();
    try {
      await fork.run(childContext, "Review");
      expect(parentContext.usage.input).toBe(10);
      expect(childContext.usage.input).toBe(10);
      await fork.run(parentContext, "Review again");
      expect(parentContext.usage.input).toBe(20);
      expect(childContext.usage.input).toBe(10);
    } finally {
      fork.dispose();
    }
  } finally {
    instance.dispose();
  }
});

test("retains usage recorded before cancellation without counting it again", async () => {
  const ctx = createWorkflowContext();
  const controller = new AbortController();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    controller.abort("Stopped");
  };
  await expect(runAgent(ctx, "Work", {
    cwd: "/project", model: "anthropic/claude-opus-4-5", signal: controller.signal,
  })).rejects.toBe("Stopped");
  expect(ctx.usage).toEqual({ ...emptyUsage, input: 10, cost: 0.25 });
});

test("context abort stops an active agent even with a separate call signal", async () => {
  const ctx = createWorkflowContext();
  const external = new AbortController();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    // Catch the task's throw to check that the agent also observes the signal.
    try { ctx.abort("Budget exceeded"); } catch {}
  };
  await expect(runAgent(ctx, "Work", {
    cwd: "/project", model: "anthropic/claude-opus-4-5", signal: external.signal,
  })).rejects.toThrow("Budget exceeded");
  expect(abortCalls).toBe(1);
  expect(disposed).toBe(true);
  expect(external.signal.aborted).toBe(false);
  expect(ctx.usage).toEqual({ ...emptyUsage, input: 10, cost: 0.25 });
});

test("does not create an agent for an aborted context", async () => {
  const ctx = createWorkflowContext();
  try { ctx.abort("Already stopped"); } catch {}
  await expect(runAgent(ctx, "Work", {
    cwd: "/project", model: "anthropic/claude-opus-4-5",
  })).rejects.toBe(ctx.signal.reason);
  expect(sessionCreations).toBe(0);
});

test("reusable agents reject an aborted context but accept a fresh context", async () => {
  const stopped = createWorkflowContext();
  const fresh = createWorkflowContext();
  const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  try {
    promptImplementation = async () => {
      try { stopped.abort("Stop"); } catch {}
    };
    await expect(instance.run(stopped, "First")).rejects.toBe(stopped.signal.reason);
    expect(disposed).toBe(false);
    await expect(instance.runOutcome(stopped, "Again")).rejects.toBe(stopped.signal.reason);
    expect(promptCalls).toBe(1);
    promptImplementation = async () => {
      await reportOutcome({ outcome: "completed", summary: "Done" });
    };
    await expect(instance.run(fresh, "Next")).resolves.toMatchObject({ summary: "Done" });
    expect(fresh.signal.aborted).toBe(false);
  } finally { instance.dispose(); }
});

test("per-call cancellation leaves the workflow context usable", async () => {
  const ctx = createWorkflowContext();
  const external = new AbortController();
  promptImplementation = async () => { external.abort("This call only"); };
  await expect(runAgent(ctx, "Work", {
    cwd: "/project", model: "anthropic/claude-opus-4-5", signal: external.signal,
  })).rejects.toBe("This call only");
  expect(ctx.signal.aborted).toBe(false);
});


test("steers an active interaction and invalidates its earlier report on delivery", async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = async () => {
    if (promptCalls === 1) {
      await reportOutcome({ outcome: "completed", summary: "Old plan" });
      await gate.promise;
    } else {
      await reportOutcome({ outcome: "completed", summary: "Revised plan" });
    }
  };
  const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  expect(await instance.steer("idle")).toBe(false);
  const run = instance.runOutcome(createWorkflowContext(), "Plan");
  await vi.waitFor(() => expect(promptCalls).toBe(1));
  const delivery = instance.steer("Include tests");
  const message = createdSessions[0].agent.steer.mock.calls[0][0];
  expect(message.content).toEqual([{ type: "text", text: "Include tests" }]);
  eventHandler?.({ type: "message_start", message });
  expect(await delivery).toBe(true);
  gate.resolve();
  expect(await run).toMatchObject({ summary: "Revised plan" });
  expect(promptCalls).toBe(2);
  instance.dispose();
  expect(await instance.steer("disposed")).toBe(false);
});

test("returns undelivered steering at completion and does not carry it to another context", async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = async () => { await gate.promise; await reportOutcome({ outcome: "completed", summary: "Done" }); };
  const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  const run = instance.runOutcome(createWorkflowContext(), "First");
  const queued = instance.steer("Late feedback");
  eventHandler?.({ type: "agent_end", messages: [] });
  expect(await instance.steer("Too late")).toBe(false);
  gate.resolve();
  await run;
  expect(await queued).toBe(false);
  expect(createdSessions[0].agent.clearSteeringQueue).toHaveBeenCalledOnce();
  await instance.runOutcome(createWorkflowContext(), "Second");
  expect(createdSessions[0].agent.steer).toHaveBeenCalledOnce();
  instance.dispose();
});

test("settles pending steering on disposal and refuses steering after cancellation", async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = () => gate.promise;
  const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  const ctx = createWorkflowContext();
  const run = instance.runOutcome(ctx, "Plan");
  const rejected = expect(run).rejects.toThrow("stop");
  const pending = instance.steer("Feedback");
  try { ctx.abort("stop"); } catch { /* expected */ }
  expect(await instance.steer("after cancellation")).toBe(false);
  instance.dispose();
  expect(await pending).toBe(false);
  gate.resolve();
  await rejected;
});

test("forwards completed assistant text during the interaction, excluding reasoning and tools", async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = async () => { await gate.promise; await reportOutcome({ outcome: "completed", summary: "Done" }); };
  const instance = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  const onText = vi.fn();
  const run = instance.runOutcome(createWorkflowContext(), "Plan", { onText });
  eventHandler?.({ type: "message_end", message: { role: "assistant", content: [
    { type: "thinking", thinking: "Private reasoning" },
    { type: "text", text: "Here is the joke." },
    { type: "toolCall", id: "tool", name: "read", arguments: {} },
  ], usage: emptyUsage } });
  eventHandler?.({ type: "message_end", message: { role: "toolResult", content: [{ type: "text", text: "file contents" }] } });
  expect(onText.mock.calls).toEqual([["Here is the joke."]]);
  gate.resolve();
  await run;
  await instance.runOutcome(createWorkflowContext(), "Next");
  expect(onText).toHaveBeenCalledOnce();
  instance.dispose();
});

test("retains earlier findings when a later report refers back to them, without leaking across interactions", async () => {
  promptImplementation = async () => {
    await reportOutcome({ outcome: "completed", summary: "Found locale files", details: "Edit locales.ts and messages/bar.json." });
    await reportOutcome({ outcome: "completed", summary: "See the preceding outcome." });
  };
  await using worker = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5" });
  const result = await worker.runOutcome(createWorkflowContext(), "Investigate");
  expect(result.summary).toBe("See the preceding outcome.");
  expect(result.details).toContain("Edit locales.ts and messages/bar.json.");
  expect(result.details).toContain("latest; supersedes earlier conclusions");

  promptImplementation = async () => {
    await reportOutcome({ outcome: "completed", summary: "Unrelated task" });
  };
  const next = await worker.runOutcome(createWorkflowContext(), "Other");
  expect(next.details).toBeUndefined();
});

test("text mode delivers a natural response once without report tools or retries", async () => {
  const text = vi.fn();
  promptImplementation = async () => {
    emitAssistantText("Hello!");
  };
  const bot = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5", tools: [], output: "text" });
  try {
    const result = await bot.run(createWorkflowContext(), "Hi", { onText: text });
    expect(result).toMatchObject({ outcome: "completed", summary: "Hello!" });
    expect(text.mock.calls).toEqual([["Hello!"]]);
    expect(promptCalls).toBe(1);
    expect(sessionOptions.customTools).toEqual([]);
    expect(sessionOptions.tools).toEqual([]);
    expect(resourceOptions.appendSystemPromptOverride(["Base"])).toEqual(["Base"]);
  } finally {
    bot.dispose();
  }
});

test("text mode keeps progress delivery but returns only the final response", async () => {
  const text = vi.fn();
  promptImplementation = async () => {
    emitAssistantText("Checking the page.");
    emitAssistantText("Here is the answer.");
  };
  const bot = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5", output: "text" });
  try {
    expect((await bot.run(createWorkflowContext(), "Look it up", { onText: text })).summary).toBe("Here is the answer.");
    expect(text.mock.calls).toEqual([["Checking the page."], ["Here is the answer."]]);
  } finally {
    bot.dispose();
  }
});

test.each(["error", "aborted", "empty"])("text mode does not report success after %s", async reason => {
  promptImplementation = async () => {
    eventHandler?.({
      type: "message_end",
      message: {
        role: "assistant", content: reason === "empty" ? [] : [{ type: "text", text: "Partial" }],
        stopReason: reason === "empty" ? "stop" : reason,
        errorMessage: reason === "empty" ? undefined : "Provider failed",
      },
    });
  };
  const result = await runAgent(createWorkflowContext(), "Hi", {
    cwd: "/project", model: "anthropic/claude-opus-4-5", output: "text",
  });
  expect(result.outcome).toBe("failed");
  expect(promptCalls).toBe(1);
});

test("text mode retains usage accounting and resets response state between turns", async () => {
  const ctx = createWorkflowContext();
  const bot = await agent({ cwd: "/project", model: "anthropic/claude-opus-4-5", output: "text" });
  promptImplementation = async () => {
    eventHandler?.({
      type: "message_end",
      message: {
        role: "assistant", content: [{ type: "text", text: "First" }],
        usage: { input: 10, output: 2, cacheRead: 0, cacheWrite: 0 },
        stopReason: "stop",
      },
    });
  };
  try {
    const first = await bot.run(ctx, "Hi");
    expect(first.usage.input).toBe(10);
    expect(ctx.usage.input).toBe(10);

    promptImplementation = async () => {};
    expect((await bot.runOutcome(ctx, "Again")).outcome).toBe("failed");
    expect(ctx.usage.input).toBe(10);
  } finally {
    bot.dispose();
  }
});
