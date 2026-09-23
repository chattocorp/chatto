import { input, spawn, task, Type, TimeoutError, type TaskHandle } from "runling";
import { createChattoImplementation } from "./implement.ts";
import type { SpecialistUpdate } from "./agent-text.ts";
import type { ChattoAgentFactory } from "./agent-text.ts";

/** Approval and attempt state belong to one conversation, never the shared webhook. */
export function createChattoApproval({ directory, model, timeout, createAgent, implement, delegate, say }: {
  directory: string;
  model: string;
  timeout: number;
  createAgent: ChattoAgentFactory;
  implement?: ReturnType<typeof createChattoImplementation>;
  delegate: <T>(child: TaskHandle<string, SpecialistUpdate, T>) => Promise<T>;
  say: (text: string) => Promise<void>;
}) {
  const implementationTask =
    implement ??
    createChattoImplementation({ createAgent });

  let approving = false;
  let implementationAttempted = false;

  return task(
    {
      name: "Approve Chatto implementation",
      input: Type.Object({
        plan: Type.String({ minLength: 1, maxLength: 20_000 }),
      }),
      output: Type.String(),
    },
    async (taskCtx, { plan }) => {
      if (approving || implementationAttempted) {
        throw new Error(
          "An implementation is already pending or was attempted in this conversation. Start a new DM for another attempt.",
        );
      }

      approving = true;
      try {
        let answer: string;

        try {
          answer = await input(
            taskCtx,
            `${plan}\n\nReply /implement to implement this exact plan in a new worktree, or send feedback to revise it.`,
            { timeout },
          );
        } catch (error) {
          if (error instanceof TimeoutError && !taskCtx.signal.aborted) {
            return "Approval timed out while waiting for the user's /implement reply. Implementation did not start. No worktree was created and no files were changed. Tell the user and end this conversation.";
          }
          throw error;
        }
        if (answer.trim() !== "/implement") {
          return `Implementation was not approved. User feedback: ${answer}`;
        }

        taskCtx.signal.throwIfAborted();
        implementationAttempted = true;

        try {
          const result = await delegate(
            spawn(taskCtx, implementationTask, { directory, plan, model }),
          );
          return `${result.summary}\n\nWorktree: ${result.directory}\nBranch: ${result.branch}\nChecks and tests passed. Review the changes in this worktree.`;
        } catch (error) {
          await say(
            error instanceof Error
              ? error.message
              : "Implementation failed; inspect the run for details.",
          ).catch(() => {});
          throw error;
        }
      } finally {
        approving = false;
      }
    },
  );
}
