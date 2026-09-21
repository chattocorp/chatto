import { createWorkflowContext } from "runling";
import { vi, describe, expect, test } from "vitest";
import type { AgentResult } from "runling";
import { plan } from "./plan.ts";

const usage = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };

const runtimeWith = (
  prompt: string,
  reports: AgentResult[],
  answers: string[],
) => {
  const agentPrompts: string[] = [];
  const questions: string[] = [];
  let agentOptions: Record<string, unknown> | undefined;
  let disposed = false;
  const f = {
    prompt,
    cwd: "/project",
    verbose: false,
    input: async (question: string) => {
      questions.push(question);
      return answers.shift() ?? "";
    },
    agent: async function (
      this: { cwd: string },
      options: Record<string, unknown>,
    ) {
      agentOptions = { ...options, cwd: options.cwd };
      return {
        id: "patient-pandas-1234",
        async runOutcome(_ctx: unknown, agentPrompt: string) {
          agentPrompts.push(agentPrompt);
          const report = reports.shift();
          if (report === undefined) throw new Error("No agent report prepared");
          return report;
        },
        dispose() {
          disposed = true;
        },
        async [Symbol.asyncDispose]() {
          disposed = true;
        },
      };
    },
    step: <T>(_label: string, work: () => T) => work(),
  } as Record<string, any>;
  mocks.current = f;

  return {
    f,
    agentPrompts,
    questions,
    get agentOptions() {
      return agentOptions;
    },
    get disposed() {
      return disposed;
    },
  };
};

describe("plan workflow", () => {
  test("interviews until the agent can complete an implementation plan", async () => {
    const runtime = runtimeWith(
      "Add deployment support",
      [
        {
          outcome: "blocked",
          summary: "Which hosting provider should we target?",
          usage,
        },
        {
          outcome: "blocked",
          summary: "Should deployments happen automatically after merge?",
          usage,
        },
        {
          outcome: "completed",
          summary: "Plan deployment support",
          details: "## Plan\n\n1. Add the deployment adapter.\n2. Test it.",
          usage,
        },
      ],
      ["Fly.io", "Yes, for main"],
    );

    await expect(plan(createWorkflowContext(), { directory: runtime.f.cwd ?? "/project", prompt: "Add deployment support" })).resolves.toEqual({
      summary: "Plan deployment support",
      details: "## Plan\n\n1. Add the deployment adapter.\n2. Test it.",
      outputs: {
        plan: "## Plan\n\n1. Add the deployment adapter.\n2. Test it.",
      },
    });
    expect(runtime.questions).toEqual([
      "Which hosting provider should we target?",
      "Should deployments happen automatically after merge?",
    ]);
    expect(runtime.agentPrompts).toHaveLength(3);
    expect(runtime.agentPrompts[0]).toContain("Add deployment support");
    expect(runtime.agentPrompts[1]).toContain("Fly.io");
    expect(runtime.agentPrompts[2]).toContain("Yes, for main");
    expect(runtime.agentOptions).toMatchObject({
      cwd: "/project",
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "medium",
      tools: ["read", "grep", "find", "ls"],
    });
    expect(runtime.disposed).toBe(true);
  });

  test("asks for the initial request when no CLI prompt was provided", async () => {
    const runtime = runtimeWith(
      "",
      [
        {
          outcome: "completed",
          summary: "Plan the requested cache",
          details: "## Plan\n\nAdd a cache.",
          usage,
        },
      ],
      ["Add a cache"],
    );

    await plan(createWorkflowContext(), { directory: runtime.f.cwd ?? "/project", prompt: "" });

    expect(runtime.questions).toEqual([
      "What would you like to build or change?",
    ]);
    expect(runtime.agentPrompts[0]).toContain("Add a cache");
  });

  test("fails clearly when the planning agent fails", async () => {
    const runtime = runtimeWith(
      "Plan something",
      [
        {
          outcome: "failed",
          summary: "Could not inspect the repository",
          usage,
        },
      ],
      [],
    );

    await expect(plan(createWorkflowContext(), { directory: runtime.f.cwd ?? "/project", prompt: "Plan something" })).rejects.toThrow(
      "Planning failed: Could not inspect the repository",
    );
    expect(runtime.disposed).toBe(true);
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
