import { createWorkflowContext } from "runling";
import { vi, describe, expect, test } from "vitest";

import { research } from "./research.ts";

const completedReport = {
  outcome: "completed" as const,
  summary: "Bun is a JavaScript runtime and toolkit",
  details:
    "# Bun\n\nBun is a JavaScript runtime and toolkit. [Source](https://bun.com/docs)",
  usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
};

describe("research workflow", () => {
  test("researches the CLI topic with only the web tool", async () => {
    const prompts: string[] = [];
    const options: Record<string, unknown>[] = [];
    const steps: string[] = [];
    const f = {
      prompt: "The Bun JavaScript runtime",
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

    await expect(research(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: "The Bun JavaScript runtime" })).resolves.toEqual({
      summary: completedReport.summary,
      details: completedReport.details,
      outputs: { research: completedReport.details },
    });
    expect(steps).toEqual(["Researching topic"]);
    expect(prompts[0]).toContain("The Bun JavaScript runtime");
    expect(prompts[0]).toContain("multiple relevant sources");
    expect(options[0]).toMatchObject({
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "medium",
      tools: ["web_fetch"],
    });
  });

  test("asks for a topic when the CLI prompt is empty", async () => {
    const questions: string[] = [];
    const prompts: string[] = [];
    const f = {
      prompt: "  ",
      input: async (question: string) => {
        questions.push(question);
        return "Agentic software factories";
      },
      runAgent: async (prompt: string) => {
        prompts.push(prompt);
        return completedReport;
      },
      step: <T>(_name: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;

    await research(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: "  " });

    expect(questions).toEqual(["What topic should I research?"]);
    expect(prompts[0]).toContain("Agentic software factories");
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
