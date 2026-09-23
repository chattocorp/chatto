import { createWorkflowContext } from "runling";
import { vi, describe, expect, test } from "vitest";

import { chattoDocs } from "./chatto-docs.ts";

const completedReport = {
  outcome: "completed" as const,
  summary: "Chatto is an open-source communication platform",
  details:
    "Chatto is an open-source communication platform. [Introduction](https://docs.chatto.run/getting-started/introduction/)",
  usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
};

describe("chatto-docs workflow", () => {
  test("answers the CLI question using only the web tool", async () => {
    const prompts: string[] = [];
    const options: Record<string, unknown>[] = [];
    const steps: string[] = [];
    const f = {
      prompt: "What is Chatto?",
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

    await expect(chattoDocs(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: "What is Chatto?" })).resolves.toEqual({
      summary: completedReport.summary,
      details: completedReport.details,
      outputs: { answer: completedReport.details },
    });
    expect(steps).toEqual([
      "Consulting Chatto documentation",
    ]);
    expect(prompts[0]).toContain("What is Chatto?");
    expect(prompts[0]).toContain(
      "https://docs.chatto.run/getting-started/introduction/",
    );
    expect(options[0]).toMatchObject({
      model: "openrouter/z-ai/glm-5.3-flash",
      thinkingLevel: "low",
      tools: ["web_fetch"],
      instructions: expect.arrayContaining([
        "Use only pages on docs.chatto.run as factual sources.",
      ]),
    });
  });

  test("asks for a question when the CLI prompt is empty", async () => {
    const questions: string[] = [];
    const prompts: string[] = [];
    const f = {
      prompt: " ",
      input: async (question: string) => {
        questions.push(question);
        return "How do I get started?";
      },
      runAgent: async (prompt: string) => {
        prompts.push(prompt);
        return completedReport;
      },
      step: <T>(_name: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;

    await chattoDocs(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: " " });

    expect(questions).toEqual(["What would you like to know about Chatto?"]);
    expect(prompts[0]).toContain("How do I get started?");
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
