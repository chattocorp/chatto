import { expect, test, vi } from "vitest";
import { createWorkflowContext, task, Type, type TaskHandle } from "runling";
import { createChattoApproval } from "./approve-implementation.ts";
import type { SpecialistContext, SpecialistUpdate } from "./agent-text.ts";

function setup() {
  const work = vi.fn(async () => ({ summary: "Done", directory: "/work", branch: "feature" }));
  const implement = task({
    name: "Implement",
    input: Type.Object({ directory: Type.String(), plan: Type.String(), model: Type.String() }),
    output: Type.Object({ summary: Type.String(), directory: Type.String(), branch: Type.String() }),
  }, (ctx: SpecialistContext) => work());
  const say = vi.fn(async (_text: string) => {});
  const approval = createChattoApproval({
    directory: "/repo",
    model: "test",
    timeout: 10,
    createAgent: async () => { throw new Error("Unused"); },
    implement,
    say,
    async delegate<T>(child: TaskHandle<string, SpecialistUpdate, T>) {
      for await (const _update of child.updates) { /* Drain task updates. */ }
      return await child.result;
    },
  });

  return { approval, work, say };
}

test("feedback permits a revised plan; approval state is isolated per conversation", async () => {
  const first = setup();
  const second = setup();
  const onInput = vi.fn().mockResolvedValueOnce("Change the plan").mockResolvedValue("/implement");
  const ctx = { ...createWorkflowContext(), onInput };

  expect(await first.approval(ctx, { plan: "Original" })).toContain("User feedback: Change the plan");
  expect(first.work).not.toHaveBeenCalled();
  expect(await first.approval(ctx, { plan: "Revised" })).toContain("Worktree: /work");
  expect(await second.approval(ctx, { plan: "Independent" })).toContain("Worktree: /work");
  expect(onInput.mock.calls[1]?.[0].message).toContain("Revised");
});

test("concurrent approval calls are rejected while the first question is pending", async () => {
  const { approval, work } = setup();
  const answer = Promise.withResolvers<string>();
  const onInput = vi.fn(() => answer.promise);
  const ctx = { ...createWorkflowContext(), onInput };
  const pending = approval(ctx, { plan: "First" });
  await vi.waitFor(() => expect(onInput).toHaveBeenCalledOnce());

  await expect(approval(ctx, { plan: "Second" })).rejects.toThrow("already pending");
  answer.resolve("No");
  await pending;
  expect(work).not.toHaveBeenCalled();
});

test("a failed implementation is reported and cannot be attempted twice", async () => {
  const { approval, work, say } = setup();
  const error = new Error("Checks failed in /work");
  work.mockRejectedValueOnce(error);
  const ctx = { ...createWorkflowContext(), onInput: async () => "/implement" };

  await expect(approval(ctx, { plan: "Plan" })).rejects.toBe(error);
  expect(say).toHaveBeenCalledWith(error.message);
  await expect(approval(ctx, { plan: "Retry" })).rejects.toThrow("was attempted");
  expect(work).toHaveBeenCalledOnce();
});

test("cancellation during approval never starts implementation", async () => {
  const { approval, work } = setup();
  const controller = new AbortController();
  const ctx = {
    ...createWorkflowContext(),
    signal: controller.signal,
    onInput: async () => {
      controller.abort(new Error("cancelled"));
      return "/implement";
    },
  };

  await expect(approval(ctx, { plan: "Plan" })).rejects.toThrow("cancelled");
  expect(work).not.toHaveBeenCalled();
});
