import { expect, test, vi } from "vitest";
import {
  createWorkflowContext,
  emptyTokenUsage,
  runWorkflow,
  type AgentOptions,
  type AgentRunOptions,
  type AgentResult,
  type AgentExtensionAPI,
  type WorkflowContext,
} from "runling";
import type { ChattoPost } from "./chatto/webhook.ts";
import { createChattoCoordinatorDemo } from "./chatto-coordinator-demo.ts";

const delivery = (body: string, id: string, thread: string | null = null) => ({
  version: 1 as const,
  id,
  type: "message.created" as const,
  triggers: ["direct_message"],
  occurred_at: "now",
  bot_id: "bot",
  room_id: "dm",
  thread_root_id: thread,
  message: { id, body, author_id: "alice" },
});
const report = (summary: string): AgentResult => ({
  outcome: "completed",
  summary,
  usage: emptyTokenUsage(),
});

test("the coordinator calls ordinary and agentic tasks as tools in one workflow", async () => {
  const registered = new Map<string, any>();
  const sent = Promise.withResolvers<void>();
  const post = vi.fn<ChattoPost>(async (_target, text) => {
    if (text === "Research complete") await sent.promise;
  });
  const events: any[] = [];
  let coordinatorCtx!: WorkflowContext<unknown>;
  let researcherCtx!: WorkflowContext<unknown>;
  const researcher = {
    runOutcome: vi.fn(
      async (
        ctx: WorkflowContext<unknown>,
        _prompt: string,
        options?: AgentRunOptions,
      ) => {
        options?.onText?.(
          "Found the theme registry; checking how themes are selected.",
        );
        researcherCtx = ctx;
        ctx.recordUsage({
          input: 5,
          output: 2,
          cacheRead: 0,
          cacheWrite: 0,
          cost: 0.01,
        });
        return report("Theme code is in app.css");
      },
    ),
    steer: vi.fn(async () => false),
    dispose: vi.fn(),
  };
  const coordinator = {
    runOutcome: vi.fn(
      async (
        ctx: WorkflowContext<unknown>,
        _prompt: string,
        options?: AgentRunOptions,
      ) => {
        coordinatorCtx = ctx;
        ctx.recordUsage({
          input: 3,
          output: 1,
          cacheRead: 0,
          cacheWrite: 0,
          cost: 0.01,
        });
        const findings = await registered
          .get("investigate")
          .execute("research", { question: "Find theme code" }, ctx.signal);
        expect(findings.content[0].text).toContain("app.css");
        options?.onText?.("Research complete");
        const answer = await registered
          .get("ask_user")
          .execute("question", { question: "Which palette?" }, ctx.signal);
        expect(answer.content[0].text).toBe("Darcula");
        return report("Agreed plan");
      },
    ),
    steer: vi.fn(async () => true),
    dispose: vi.fn(),
  };
  const createAgent = vi.fn(async (options: AgentOptions) => {
    if (!options.extensions) return researcher;
    for (const extension of options.extensions) {
      const factory =
        typeof extension === "function" ? extension : extension.factory;
      await factory({
        registerTool: (tool: any) => registered.set(tool.name, tool),
      } as unknown as AgentExtensionAPI);
    }
    expect(options.tools).toEqual(["investigate", "ask_user", "implement"]);
    return coordinator;
  });
  const bot = createChattoCoordinatorDemo({
    directory: process.cwd(),
    post,
    createAgent,
  });
  const run = runWorkflow(bot, {
    input: delivery("Add Darcula", "root"),
    onEvent: (event) => events.push(event),
  });
  await vi.waitFor(() =>
    expect(
      post.mock.calls.some((call) => call[1] === "Research complete"),
    ).toBe(true),
  );
  expect(post.mock.calls.some((call) => call[1] === "Which palette?")).toBe(
    false,
  );
  sent.resolve();
  await vi.waitFor(() =>
    expect(post.mock.calls.some((call) => call[1] === "Which palette?")).toBe(
      true,
    ),
  );
  await bot.route({ start: async () => {
    throw new Error("must not start another run");
  } }, delivery("Darcula", "answer", "root"));
  const execution = await run;
  expect(execution.ok).toBe(true);
  expect(execution.output).toEqual({ summary: "Agreed plan" });
  expect(execution.usage).toMatchObject({ input: 8, output: 3, cost: 0.02 });
  expect(researcherCtx.usage).toBe(coordinatorCtx.usage);
  const messages = post.mock.calls.map((call) => call[1]);
  expect(messages).not.toContain("Investigating: Find theme code");
  expect(messages).toContain(
    "Found the theme registry; checking how themes are selected.",
  );
  expect(messages).toContain(
    "Investigation complete. Reviewing the findings for the plan.",
  );
  expect(messages).not.toContain("Theme code is in app.css");
  expect(
    events
      .filter((event) => event.type === "step.started")
      .map((event) => event.label),
  ).toEqual([
    "Chatto coordinator demo",
    "Investigate: Find theme code",
    "Ask Chatto user",
  ]);
  expect(researcher.dispose).toHaveBeenCalledOnce();
  expect(coordinator.dispose).toHaveBeenCalledOnce();
});

test.each(["delivered", "rejected", "parallel"])(
  "routes busy messages (%s) and cancels child work",
  async (mode) => {
    const registered = new Map<string, any>();
    let childCtx!: WorkflowContext<unknown>;
    const researcher = {
      runOutcome: vi.fn(async (ctx: WorkflowContext<unknown>) => {
        childCtx = ctx;
        return new Promise<AgentResult>((_resolve, reject) =>
          ctx.signal.addEventListener(
            "abort",
            () => reject(ctx.signal.reason),
            { once: true },
          ),
        );
      }),
      steer: vi.fn(async () => mode !== "rejected"),
      dispose: vi.fn(),
    };
    const coordinator = {
      runOutcome: vi.fn(async (ctx: WorkflowContext<unknown>) => {
        const tool = registered.get("investigate");
        await Promise.all(
          Array.from({ length: mode === "parallel" ? 2 : 1 }, (_, i) =>
            tool.execute(
              `research-${i}`,
              { question: "Find theme code" },
              ctx.signal,
            ),
          ),
        );
        return report("Done");
      }),
      steer: vi.fn(async () => true),
      dispose: vi.fn(),
    };
    const post = vi.fn<ChattoPost>(async () => {});
    const bot = createChattoCoordinatorDemo({
      directory: process.cwd(),
      post,
      createAgent: async (options) => {
        if (!options.extensions) return researcher;
        for (const extension of options.extensions) {
          await (
            typeof extension === "function" ? extension : extension.factory
          )({
            registerTool: (tool: any) => registered.set(tool.name, tool),
          } as unknown as AgentExtensionAPI);
        }
        return coordinator;
      },
    });
    const ctx = createWorkflowContext();
    const run = bot(ctx, delivery("Feature", "root"));
    const failed = expect(run).rejects.toThrow("Cancelled from Chatto");
    await vi.waitFor(() =>
      expect(researcher.runOutcome).toHaveBeenCalledTimes(
        mode === "parallel" ? 2 : 1,
      ),
    );
    await bot.route(
      { start: async () => {
        throw new Error("unexpected run");
      } }, delivery("Also consider accessibility", "steering", "root"),
    );
    await vi.waitFor(() =>
      expect(coordinator.steer).toHaveBeenCalledWith(
        "Also consider accessibility",
      ),
    );
    if (mode === "parallel") expect(researcher.steer).not.toHaveBeenCalled();
    else {
      await vi.waitFor(() =>
        expect(researcher.steer).toHaveBeenCalledWith(
          "Also consider accessibility",
        ),
      );
    }
    expect(post.mock.calls.some(call => call[1].startsWith("Passed your message"))).toBe(false);
    await bot.route({ start: async () => ({ id: "unused" }) }, delivery("/cancel", "cancel", "root"));
    await failed;
    expect(childCtx.signal.aborted).toBe(true);
    expect(researcher.dispose).toHaveBeenCalledTimes(
      mode === "parallel" ? 2 : 1,
    );
    expect(coordinator.dispose).toHaveBeenCalledOnce();
  },
);

test("implementation requires approval of the displayed plan and cannot run twice", async () => {
  const { createChattoImplementation } = await import("./chatto/implement.ts");
  const registered = new Map<string, any>();
  const post = vi.fn<ChattoPost>(async () => {});
  const worker = {
    runOutcome: vi.fn(async (_ctx: WorkflowContext<unknown>, _prompt: string) =>
      report("Built it"),
    ),
    dispose: vi.fn(),
  };
  const prepare = vi.fn(async () => ({
    directory: "/isolated",
    branch: "runling/test",
  }));
  const implement = createChattoImplementation({
    prepare,
    createAgent: async () => worker,
    validate: async () => null,
    hasChanges: async () => true,
  });
  const coordinator = {
    runOutcome: vi.fn(async (ctx: WorkflowContext<unknown>) => {
      const tool = registered.get("implement");
      const declined = await tool.execute(
        "first",
        { plan: "First proposed plan" },
        ctx.signal,
      );
      expect(declined.content[0].text).toContain("not approved");
      const approved = await tool.execute(
        "second",
        { plan: "Revised exact plan" },
        ctx.signal,
      );
      expect(approved.content[0].text).toContain("/isolated");
      await expect(
        tool.execute("duplicate", { plan: "Another plan" }, ctx.signal),
      ).rejects.toThrow("already pending or was attempted");
      return report("Done; changes are in /isolated");
    }),
    steer: vi.fn(async () => true),
    dispose: vi.fn(),
  };
  const bot = createChattoCoordinatorDemo({
    directory: process.cwd(),
    post,
    implement,
    createAgent: async (options) => {
      for (const extension of options.extensions ?? []) {
        await (typeof extension === "function" ? extension : extension.factory)(
          {
            registerTool: (tool: any) => registered.set(tool.name, tool),
          } as unknown as AgentExtensionAPI,
        );
      }
      return coordinator;
    },
  });
  const run = bot(createWorkflowContext(), delivery("Build a theme", "root"));
  await vi.waitFor(() =>
    expect(
      post.mock.calls.some((call) => call[1].includes("First proposed plan")),
    ).toBe(true),
  );
  expect(prepare).not.toHaveBeenCalled();
  await expect(
    registered
      .get("implement")
      .execute(
        "parallel",
        { plan: "Concurrent plan" },
        new AbortController().signal,
      ),
  ).rejects.toThrow("already pending");
  await bot.route(
    { start: async () => ({ id: "unused" }) }, delivery("Use the existing API", "feedback", "root"),
  );
  await vi.waitFor(() =>
    expect(
      post.mock.calls.some((call) => call[1].includes("Revised exact plan")),
    ).toBe(true),
  );
  expect(prepare).not.toHaveBeenCalled();
  await bot.route({ start: async () => ({ id: "unused" }) }, delivery("/implement", "approval", "root"));
  await run;
  expect(prepare).toHaveBeenCalledOnce();
  expect(worker.runOutcome.mock.calls[0]![1]).toBe(
    "Implement this approved plan:\n\nRevised exact plan",
  );
  const messages = post.mock.calls.map((call) => call[1]);
  expect(
    messages.filter(
      (text) =>
        text === "Preparing an isolated worktree and installing dependencies.",
    ),
  ).toHaveLength(1);
  expect(messages).toContain("Running project checks and tests (attempt 1/3).");
  expect(
    messages.indexOf(
      "Preparing an isolated worktree and installing dependencies.",
    ),
  ).toBeLessThan(
    messages.indexOf("Running project checks and tests (attempt 1/3)."),
  );
  expect(worker.dispose).toHaveBeenCalledOnce();
});

test("reports approval timeout without starting implementation", async () => {
  const tools = new Map<string, any>();
  const implement = vi.fn();
  let returned = "";
  const coordinator = {
    runOutcome: vi.fn(async (ctx: WorkflowContext<unknown>) => {
      const result = await tools.get("implement").execute("approve", { plan: "Small fix" }, ctx.signal);
      returned = result.content[0].text;
      return report(returned);
    }),
    steer: vi.fn(async () => true),
    dispose: vi.fn(),
  };
  const bot = createChattoCoordinatorDemo({
    directory: process.cwd(),
    post: async () => {},
    timeout: 0.01,
    implement: implement as any,
    createAgent: async options => {
      for (const extension of options.extensions ?? []) {
        await (typeof extension === "function" ? extension : extension.factory)({
          registerTool: (tool: any) => tools.set(tool.name, tool),
        } as unknown as AgentExtensionAPI);
      }
      return coordinator;
    },
  });
  await bot(createWorkflowContext(), delivery("Small fix", "timeout-root"));
  expect(returned).toContain("Approval timed out");
  expect(returned).toContain("Implementation did not start");
  expect(implement).not.toHaveBeenCalled();
});
