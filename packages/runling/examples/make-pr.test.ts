import { createWorkflowContext } from "runling";
import { vi, describe, expect, test } from "vitest";
import type { Exec, WorkflowResult } from "runling";
import {
  createWorktree,
  describePullRequest,
  postReview,
} from "./make-pr.ts";

describe("make-pr workflow", () => {
  test("updates the remote default branch and branches the worktree from it", async () => {
    const commands: Array<{ template: string; values: unknown[] }> = [];
    const exec = ((strings: TemplateStringsArray, ...values: unknown[]) => {
      commands.push({ template: strings.join("_argument_"), values });
      return Object.assign(Promise.resolve(), {
        text: async () => "master\n",
      });
    }) as unknown as Exec;
    const f = {
      exec,
      step: <T>(_label: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;

    await createWorktree(
      f.cwd ?? "/project",
      "runling/bright-otters-2468",
      "/worktrees/bright-otters-2468",
    );

    expect(commands).toEqual([
      {
        template:
          "gh repo view --json defaultBranchRef --jq .defaultBranchRef.name",
        values: [],
      },
      {
        template:
          "git fetch origin +refs/heads/_argument_:refs/remotes/origin/_argument_",
        values: ["master", "master"],
      },
      {
        template:
          "git worktree add -b _argument_ _argument_ origin/_argument_",
        values: [
          "runling/bright-otters-2468",
          "/worktrees/bright-otters-2468",
          "master",
        ],
      },
    ]);
  });

  test("builds PR metadata from the implementation summary and current commit", async () => {
    let disposed = false;
    let prompt = "";
    let agentOptions: Record<string, unknown> | undefined;
    const f = {
      cwd: "/worktree",
      agent: async function (
        this: { cwd: string },
        options: Record<string, unknown>,
      ) {
        agentOptions = { ...options, cwd: options.cwd };
        const dispose = () => {
          disposed = true;
        };
        return {
          id: "test-agent-0000",
          async run(_ctx: unknown, receivedPrompt: string) {
            prompt = receivedPrompt;
            return {
              outcome: "completed" as const,
              summary: "Explain the change",
              details: "## Summary\n\nExplains the change and why.",
              usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
            };
          },
          dispose,
          async [Symbol.asyncDispose]() {
            dispose();
          },
        };
      },
      step: <T>(_label: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;

    await expect(
      describePullRequest(
        createWorkflowContext(),
        f.cwd ?? "/project",
        "Changed the behavior to fix the bug",
        "commit abc123\n\ndiff --git a/foo.ts b/foo.ts",
      ),
    ).resolves.toEqual({
      title: "Explain the change",
      body: "## Summary\n\nExplains the change and why.",
    });

    expect(agentOptions).toMatchObject({
      cwd: "/worktree",
      model: "openai-codex/gpt-5.6-sol",
      thinkingLevel: "medium",
      tools: ["read", "grep", "find", "ls"],
    });
    expect(prompt).toContain("Changed the behavior to fix the bug");
    expect(prompt).toContain("diff --git a/foo.ts b/foo.ts");
    expect(disposed).toBe(true);
  });

  test("posts a workflow review as a pull request comment", async () => {
    let command: TemplateStringsArray | undefined;
    let expressions: unknown[] = [];
    const exec = ((strings: TemplateStringsArray, ...values: unknown[]) => {
      command = strings;
      expressions = values;
      return Promise.resolve();
    }) as unknown as Exec;
    const f = {
      exec,
      step: <T>(_label: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;
    const result: WorkflowResult = {
      summary: "Found one issue",
      details: "## Findings\n\nFix the race.",
    };

    await postReview(f.cwd ?? "/project", "https://github.com/example/project/pull/42", result);

    expect(command?.join("_argument_")).toBe(
      "gh pr comment _argument_ --body _argument_",
    );
    expect(expressions).toEqual([
      "https://github.com/example/project/pull/42",
      "## Findings\n\nFix the race.",
    ]);
  });

  test("uses the review summary when it has no details", async () => {
    let body: unknown;
    const exec = ((_strings: TemplateStringsArray, ...values: unknown[]) => {
      body = values[1];
      return Promise.resolve();
    }) as unknown as Exec;
    const f = {
      exec,
      step: <T>(_label: string, work: () => T) => work(),
    } as Record<string, any>;
  mocks.current = f;

    await postReview(f.cwd ?? "/project", "https://github.com/example/project/pull/42", {
      summary: "No issues found",
    });

    expect(body).toBe("No issues found");
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
