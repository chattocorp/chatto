import { createWorkflowContext } from "runling";
import { vi, describe, expect, test } from "vitest";

import { ask } from "./ask.ts";

const completedReport = {
  outcome: "completed" as const,
  summary: "Tasks receive their declared inputs",
  details:
    "Tasks receive explicit input; helpers are named imports (`src/runtime/index.ts`).",
  usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
};

describe("ask workflow", () => {
  test("answers the CLI question using only read-only repository tools", async () => {
    const prompts: string[] = [];
    const options: Record<string, unknown>[] = [];
    const steps: string[] = [];
    const f = {
      prompt: "How do tasks use runtime helpers?",
      input: () => {
        throw new Error("input should not be requested");
      },
      runAgent: async (prompt: string, agentOptions: Record<string, unknown>) => {
        prompts.push(prompt);
        options.push(agentOptions);
        return completedReport;
      },
      step: <T>(name: string, work: () => T) => {
        steps.push(name);
        return work();
      },
    } as Record<string, any>;
  mocks.current = f;

    await expect(ask(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: "How do tasks use runtime helpers?" })).resolves.toEqual({
      summary: completedReport.summary,
      details: completedReport.details,
      outputs: { answer: completedReport.details },
    });
    expect(steps).toEqual([
      "Investigating repository",
    ]);
    expect(prompts[0]).toContain(
      "How do tasks use runtime helpers?",
    );
    expect(options[0]).toMatchObject({
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "medium",
      tools: ["read", "grep", "find", "ls"],
    });
  });

  test("asks for a question when the CLI prompt is empty", async () => {
    const questions: string[] = [];
    const prompts: string[] = [];
    const f = {
      prompt: "",
      input: async (question: string) => {
        questions.push(question);
        return "Where are agent tools configured?";
      },
      runAgent: async (prompt: string) => {
        prompts.push(prompt);
        return completedReport;
      },
      step: <T>(_name: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;

    await ask(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: "" });

    expect(questions).toEqual([
      "What would you like to know about the repository?",
    ]);
    expect(prompts[0]).toContain("Where are agent tools configured?");
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
