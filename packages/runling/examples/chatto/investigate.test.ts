import { expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage } from "runling";
import { createChattoInvestigation } from "./investigate.ts";
import type { ChattoAgentFactory } from "./agent-text.ts";

test.each(["completed", "failed"] as const)("disposes the researcher after a %s report", async (outcome) => {
  const worker = {
    runOutcome: vi.fn(async () => ({ outcome, summary: "Summary", details: "Findings", usage: emptyTokenUsage() })),
    steer: vi.fn(async () => false),
    dispose: vi.fn(),
  };
  const createAgent = vi.fn<ChattoAgentFactory>(async () => worker);
  const investigate = createChattoInvestigation({ directory: "/repo", model: "test", createAgent });
  const result = investigate("Find files")(createWorkflowContext(), { question: "Find files" });

  if (outcome === "failed") {
    await expect(result).rejects.toThrow("Summary");
  } else {
    expect(await result).toBe("Findings");
  }

  expect(worker.dispose).toHaveBeenCalledOnce();
  expect(createAgent).toHaveBeenCalledWith(expect.objectContaining({
    cwd: "/repo",
    tools: ["read", "grep", "find", "ls"],
  }));
});

test("cancellation releases the connection and disposes an unresponsive researcher", async () => {
  const started = Promise.withResolvers<void>();
  const worker = {
    runOutcome: vi.fn(() => {
      started.resolve();
      return new Promise<never>(() => {});
    }),
    steer: vi.fn(async () => false),
    dispose: vi.fn(),
  };
  const investigate = createChattoInvestigation({ directory: "/repo", model: "test", createAgent: async () => worker });
  const controller = new AbortController();
  const ctx = { ...createWorkflowContext(), signal: controller.signal };
  const result = investigate("Find files")(ctx, { question: "Find files" });
  await started.promise;
  controller.abort(new Error("stop"));

  await expect(result).rejects.toThrow("stop");
  expect(worker.dispose).toHaveBeenCalledOnce();
});
