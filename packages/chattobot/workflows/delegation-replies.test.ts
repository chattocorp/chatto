import { expect, test, vi } from "vitest";
import { createWorkflowContext } from "runling";
import type { AgentExtensionAPI, AgentOptions } from "runling/agents";

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock("runling/agents", async importOriginal => ({
  ...await importOriginal<typeof import("runling/agents")>(), runAgentConversation: interact,
}));
// A fake investigation accepts delegation without starting an LLM or subprocess.
vi.mock("./investigate.ts", async importOriginal => ({
  ...await importOriginal<typeof import("./investigate.ts")>(),
  investigationExtension: (_ctx: unknown, _settings: unknown, announce: (text: string, signal: AbortSignal) => Promise<void>) =>
    (pi: AgentExtensionAPI) => pi.registerTool({ name: "fakeInvestigate", execute: async () => announce("Investigation started.", new AbortController().signal) } as never),
}));
import { conversation } from "./chat.ts";

test.each(["accepted", "refused"])("%s delegation posts one host-owned reply and permits later replies", async mode => {
  const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
  const replies: string[] = [];
  const createAgent = async (options: AgentOptions) => {
    expect(options.textDelivery).toBe("final");
    for (const extension of options.extensions ?? []) {
      const factory = typeof extension === "function" ? extension : extension.factory;
      await factory({ registerTool(tool) { tools.set(tool.name, tool as never); } } as AgentExtensionAPI);
    }
    return { runOutcome: vi.fn(), steer: async () => false, dispose() {} };
  };
  interact.mockImplementationOnce(async (ctx, _agent, _prompt, options) => {
    options.onBusy(true);
    await options.prepareMessage("Investigation complete", "notification");
    if (mode === "accepted") await tools.get("fakeInvestigate")!.execute("call", {} as never);
    else {
      for (let i = 0; i < 2; i++) await tools.get("implementChatto")!.execute("call", {
        request: "Implement the plan", announcement: "Starting implementation",
      } as never);
    }
    await ctx.emit("I'm working on the fix now.");
    expect(replies).toHaveLength(1);
    expect(replies[0]).toContain(mode === "accepted" ? "Investigation started" : "Implementation was not started");
    options.onBusy(false);
    options.onBusy(true);
    await ctx.emit("Here is the answer to your next question.");
    return "done";
  });
  await conversation({ ...createWorkflowContext(), emit: async text => { replies.push(text); } }, "Assess this", {
    createAgent, model: "test/model", investigation: { directory: "/unused" },
    implementation: { directory: "/unused", repository: "example/chatto" },
    delivery: { version: 1, id: "delivery", type: "message.created", triggers: ["mention"], occurred_at: "now",
      bot_id: "bot", room_id: "room", thread_root_id: "root", message: { id: "message", author_id: "human", body: "Assess this" } },
    readThread: async () => [], onBusy() {}, setReplyContext() {},
    announce: async text => { replies.push(text); },
  });
  expect(replies).toHaveLength(2);
  expect(replies[1]).toBe("Here is the answer to your next question.");
});
