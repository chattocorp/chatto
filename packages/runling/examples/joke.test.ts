import { createWorkflowContext } from "runling";
import { vi, describe, expect, test } from "vitest";

import { joke } from "./joke.ts";

describe("joke workflow", () => {
  test("asks for a topic and returns an agent-written joke", async () => {
    const questions: string[] = [];
    const prompts: string[] = [];
    const steps: string[] = [];
    const options: Record<string, unknown>[] = [];
    const f = {
      input: async (question: string) => {
        questions.push(question);
        return "TypeScript";
      },
      runAgent: async (prompt: string, agentOptions: Record<string, unknown>) => {
        prompts.push(prompt);
        options.push(agentOptions);
        return {
          outcome: "completed" as const,
          summary: "A joke",
          details: "TypeScript walked into a bar, but JavaScript let it in anyway.",
          usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
        };
      },
      step: <T>(name: string, work: () => T) => {
        steps.push(name);
        return work();
      },
    } as Record<string, any>;
  mocks.current = f;

    await expect(joke(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: "" })).resolves.toBe(
      "# Joke\n\nTypeScript walked into a bar, but JavaScript let it in anyway.",
    );
    expect(questions).toEqual(["What should the joke be about?"]);
    expect(steps).toEqual(["Write joke", "Review joke for funniness"]);
    expect(prompts).toHaveLength(2);
    expect(prompts[1]).toContain("TypeScript walked into a bar, but JavaScript let it in anyway.");
    expect(options[1]).toMatchObject({
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "low",
      tools: [],
    });
    expect(prompts[0]).toContain('"TypeScript"');
    expect(options[0]).toMatchObject({
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "low",
      tools: [],
    });
  });

  test("uses the invocation prompt without asking for input", async () => {
    let askedForInput = false;
    const prompts: string[] = [];
    const f = {
      prompt: "webhooks",
      input: async () => {
        askedForInput = true;
        return "unused";
      },
      runAgent: async (prompt: string) => {
        prompts.push(prompt);
        return {
          outcome: "completed" as const,
          summary: "A webhook joke",
          usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
        };
      },
      step: <T>(_name: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;

    await joke(createWorkflowContext(), { directory: f.cwd ?? "/project", prompt: "webhooks" });

    expect(askedForInput).toBe(false);
    expect(prompts[0]).toContain('"webhooks"');
    expect(prompts).toHaveLength(2);
    expect(prompts[1]).toContain('"A webhook joke"');
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
