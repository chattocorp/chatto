import { createWorkflowContext } from "runling";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { type Exec } from "runling";
import * as git from "runling/git";
import { implement } from "./implement.ts";

vi.mock("runling/git", () => ({
  getPwd: vi.fn(async () => ({ hasChanges: Promise.resolve(true) })),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

const contextValues = {
  cwd: "/project",
  prompt: "Make the change",
  verbose: false,
};

const completedReport = {
  outcome: "completed" as const,
  summary: "Made the change",
  usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
};

interface RuntimeOptions {
  runCheck?: () => Promise<void>;
  runTests?: () => Promise<void>;
  runAgent?: (
    prompt: string,
    options?: Record<string, unknown>,
  ) => Promise<typeof completedReport>;
}

function runtimeWith({
  runCheck = async () => {},
  runTests = async () => {},
  runAgent = async () => completedReport,
}: RuntimeOptions = {}) {
  const messages: string[] = [];
  const agentOptions: Record<string, unknown>[] = [];
  let disposedAgents = 0;
  class TestShellError extends Error {
    stdout = Buffer.from("stdout");
    stderr = Buffer.from("stderr");
  }
  const exec = ((strings: TemplateStringsArray) => {
    const command = strings.join("");
    const run = command === "pnpm run check" ? runCheck : runTests;
    const shellPromise = {
      cwd: () => shellPromise,
      async nothrow() {
        try {
          await run();
          return {
            exitCode: 0,
            stdout: Buffer.from(""),
            stderr: Buffer.from(""),
          };
        } catch (error) {
          if (!(error instanceof TestShellError)) throw error;
          return {
            exitCode: 1,
            stdout: error.stdout,
            stderr: error.stderr,
          };
        }
      },
    };
    return shellPromise;
  }) as unknown as Exec;

  const runling = {
    ...contextValues,
    exec,
    agent: async function (
      this: { cwd: string },
      options: Record<string, unknown>,
    ) {
      const resolvedOptions = { ...options, cwd: options.cwd };
      agentOptions.push(resolvedOptions);
      let disposed = false;
      const dispose = () => {
        if (disposed) return;
        disposed = true;
        disposedAgents++;
      };
      return {
        id: "test-agent-0000",
        run: (_ctx: unknown, prompt: string) => runAgent(prompt, resolvedOptions),
        dispose,
        async [Symbol.asyncDispose]() {
          dispose();
        },
      };
    },
    log: { info: (message: string) => messages.push(message) },
    step: <T>(label: string, work: () => T) => {
      messages.push(label);
      return work();
    },
  } as Record<string, any>;
  mocks.current = runling;

  return {
    runling,
    messages,
    agentOptions,
    get disposedAgents() {
      return disposedAgents;
    },
    TestShellError,
  };
}

describe("implement workflow", () => {
  test("returns the implementing agent's summary", async () => {
    const prompts: string[] = [];
    const { runling } = runtimeWith({
      runAgent: async (prompt) => {
        prompts.push(prompt);
        return {
          ...completedReport,
          summary: "Implementation summary",
        };
      },
    });

    await expect(implement(createWorkflowContext(), { directory: runling.cwd ?? "/project", prompt: "Make the change" })).resolves.toBe("Implementation summary");
    expect(prompts).toEqual(["Make the change"]);
    expect(git.getPwd).toHaveBeenCalledExactlyOnceWith(runling.cwd);
  });

  test("runs agents on Sol with medium thinking", async () => {
    const setup = runtimeWith();

    await expect(implement(createWorkflowContext(), { directory: setup.runling.cwd ?? "/project", prompt: "Make the change" })).resolves.toBe("Made the change");

    expect(setup.agentOptions).toHaveLength(1);
    expect(setup.disposedAgents).toBe(1);
    expect(setup.agentOptions[0]).toMatchObject({
      cwd: "/project",
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "medium",
      instructions: [
        "Write tests for new or changed features.",
        "Summarize what you changed and why in your final report.",
      ],
    });
  });

  test("feeds validation failures back to the implementing agent", async () => {
    let checks = 0;
    const prompts: string[] = [];
    let TestShellError: new () => Error;
    const setup = runtimeWith({
      runCheck: async () => {
        checks++;
        if (checks === 1) {
          throw new TestShellError();
        }
      },
      runAgent: async (prompt) => {
        prompts.push(prompt);
        return {
          ...completedReport,
          summary:
            prompts.length === 1 ? "Initial summary" : "Repaired summary",
        };
      },
    });
    TestShellError = setup.TestShellError;

    await expect(implement(createWorkflowContext(), { directory: setup.runling.cwd ?? "/project", prompt: "Make the change" })).resolves.toBe("Repaired summary");

    expect(setup.agentOptions).toHaveLength(1);
    expect(setup.disposedAgents).toBe(1);
    expect(prompts).toHaveLength(2);
    expect(prompts[1]).toContain("Project validation failed");
    expect(prompts[1]).toContain("summarize the complete implementation");
    expect(prompts[1]).toContain("stdout");
    expect(prompts[1]).toContain("stderr");
  });

  test("runs checks and tests once when both pass", async () => {
    let checks = 0;
    let tests = 0;
    const { runling, messages } = runtimeWith({
      runCheck: async () => {
        checks++;
      },
      runTests: async () => {
        tests++;
      },
    });

    await expect(implement(createWorkflowContext(), { directory: runling.cwd ?? "/project", prompt: "Make the change" })).resolves.toBe("Made the change");

    expect(checks).toBe(1);
    expect(tests).toBe(1);
    expect(messages).toEqual([
      "Implementing change",
      "Validate",
      "Run checks",
      "Run tests",
    ]);
  });

  test("awaits a repair before rerunning checks and tests", async () => {
    let tests = 0;
    let repaired = false;
    let TestShellError: new () => Error;
    const setup = runtimeWith({
      runTests: async () => {
        tests++;
        if (tests === 1) {
          throw new TestShellError();
        }
        expect(repaired).toBe(true);
      },
      runAgent: async () => {
        if (tests > 0) {
          await Promise.resolve();
          repaired = true;
        }
        return completedReport;
      },
    });
    TestShellError = setup.TestShellError;

    await expect(implement(createWorkflowContext(), { directory: setup.runling.cwd ?? "/project", prompt: "Make the change" })).resolves.toBe("Made the change");

    expect(tests).toBe(2);
    expect(setup.messages).toEqual([
      "Implementing change",
      "Validate",
      "Run checks",
      "Run tests",
      "Repairing validation (attempt 1/3)",
      "Validate",
      "Run checks",
      "Run tests",
    ]);
  });
});

const mocks = vi.hoisted(() => ({ current: {} as Record<string, any> }));
vi.mock("runling", async (importOriginal) => {
  const actual = await importOriginal<typeof import("runling")>();
  return {
    ...actual,
    agent: (options: unknown) => mocks.current.agent(options),
    runAgent: (_ctx: unknown, ...args: unknown[]) => mocks.current.runAgent(...args),
    input: (_ctx: unknown, ...args: unknown[]) => mocks.current.input(...args),
    step: (name: string, work: () => unknown) => mocks.current.step(name, work),
    log: { info: (message: string) => mocks.current.log?.info(message) },
    exec: (...args: unknown[]) => {
      const command = mocks.current.exec(...args);
      return Object.assign(command, { cwd: (directory: string) => {
        expect(directory).toBe(mocks.current.cwd ?? "/project");
        return command;
      } });
    },
  };
});
