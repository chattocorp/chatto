import { createChattoInvestigation, investigationInput } from "./chatto/investigate.ts";
import { createChattoApproval } from "./chatto/approve-implementation.ts";
import { createChattoConversation } from "./chatto/conversation.ts";
import {
  agent,
  taskTool,
  defineAgentExtension,
} from "runling/agents";
import type {
  ChattoAgentFactory,
  SpecialistUpdate,
} from "./chatto/agent-text.ts";
import { spawn, input, task, Type, type TaskHandle } from "runling";
import { sendChattoTyping, type ChattoTyping } from "./chatto/typing.ts";
import { createChattoImplementation } from "./chatto/implement.ts";
import { runChattoAgent } from "./chatto/agent.ts";
import {
  createChattoWebhook,
  postToChatto,
  type ChattoPost,
} from "./chatto/webhook.ts";

export interface CoordinatorDemoOptions {
  directory: string;
  post: ChattoPost;
  typing?: ChattoTyping;
  model?: string;
  timeout?: number;
  createAgent?: ChattoAgentFactory;
  implement?: ReturnType<typeof createChattoImplementation>;
}

/** An ordinary root task defines the capabilities of its agentic coordinator. */
export function createChattoCoordinatorDemo({
  directory,
  post,
  typing,
  model = "openai-codex/gpt-5.6-sol",
  timeout = 900,
  createAgent = agent,
  implement,
}: CoordinatorDemoOptions) {
  if (
    !Number.isFinite(timeout) ||
    timeout < 0 ||
    Math.ceil(timeout * 1000) > 2_147_483_647
  ) {
    throw new RangeError("timeout must be seconds between 0 and 2147483.647");
  }
  return createChattoWebhook({
    name: "Chatto coordinator demo",
    output: Type.Object({ summary: Type.String() }),
    post,
    async run(rootCtx, delivery, destination, inbox) {
      // Check before creating agents or tasks that can access a checkout.
      if (!directory.trim()) {
        throw new Error("Set CHATTO_WORKING_COPY in packages/runling/.env before running this demo");
      }
      const children = new Set<TaskHandle<string, SpecialistUpdate, unknown>>();

      async function delegate<T>(
        child: TaskHandle<string, SpecialistUpdate, T>,
      ): Promise<T> {
        children.add(child);

        try {
          for await (const update of child.updates) {
            if (update.type === "text") {
              await say(update.text);
            }
          }

          return await child.result;
        } finally {
          children.delete(child);
          child.cancel();
        }
      }

      const { ctx, say, agentOptions } = createChattoConversation(rootCtx, {
        destination, inbox, post, typing,
      });

      const investigate = createChattoInvestigation({ directory, model, createAgent });
      const askUser = task(
        {
          name: "Ask Chatto user",
          input: Type.Object({ question: Type.String({ minLength: 1 }) }),
          output: Type.String(),
        },
        (taskCtx, { question }) => input(taskCtx, question, { timeout }),
      );

      const implementPlan = createChattoApproval({
        directory, model, timeout, createAgent, implement, delegate, say,
      });

      // Context stays explicit at the task call. These tools belong to this run only.
      const tools = defineAgentExtension((pi) => {
        pi.registerTool(taskTool(ctx, {
          name: "investigate",
          label: "Investigate Chatto",
          description: "Ask one read-only specialist a focused repository question. Each call starts a new agent with its own cost. Reuse returned findings; combine related questions in one call. Returns findings to you, not the user.",
          parameters: investigationInput,
        }, (toolCtx, args) => delegate(spawn(toolCtx, investigate(args.question), args))));

        pi.registerTool(taskTool(ctx, {
          name: "ask_user",
          label: "Ask user",
          description: "Post a question to the Chatto thread and wait for the user's answer. Use this for clarification or plan feedback.",
          parameters: askUser.input,
        }, askUser));

        pi.registerTool(taskTool(ctx, {
          name: "implement",
          label: "Implement plan",
          description: "Present the complete proposed plan, request explicit user approval, then implement and validate it in a separate worktree. This tool asks for approval itself; do not ask for /implement separately. Returns feedback if the user declines.",
          parameters: implementPlan.input,
        }, implementPlan));
      });

      await say(
        "I’ll coordinate this request with specialist tasks. Send thoughts at any time, or /cancel to stop. I’ll ask you to approve the exact plan before implementing it.",
      );

      const coordinator = await createAgent({
        cwd: directory,
        model,
        thinkingLevel: "medium",
        tools: ["investigate", "ask_user", "implement"],
        extensions: [tools],
        resources: { extensions: false, skills: false, promptTemplates: false },
        instructions: [
          "You coordinate a Chatto planning conversation. Choose your tools and sequence yourself.",
          "Use investigate for repository facts; use ask_user for material questions and feedback on your proposed plan.",
          "Default to one focused investigation. Combine related questions so one specialist can reuse what it reads. Reuse its findings rather than asking another agent to repeat the same work.",
          "Use parallel investigations only for independent questions that need different evidence and materially benefit from separate agents. Do not split a small feature into generic frontend/backend/testing investigations by default.",
          "Keep each investigation question concise and specific: it is also the task label shown in the timeline. Ask a follow-up investigation only for a concrete unresolved gap.",
          "Respond to incoming steering and conversational asides with brief text messages; these are sent to the user immediately after each assistant message completes.",
          "When asking a question, call ask_user with the complete user-facing message, including any greeting. Do not emit a separate assistant-text preface or repeat the question. For example, put Hey! What would you like to build? entirely in ask_user.question.",
          "Use implement with the complete generated plan when it is ready for approval. The tool obtains approval and runs the coding task. You cannot grant approval yourself.",
          "If implement returns feedback, revise the plan and call it again. If it fails, explain the failure and retained worktree; never claim success. After success, summarize the result and worktree location.",
          "Likewise, implement posts the plan and approval question itself; do not emit a separate preface or duplicate plan. Report completed when the user's request is handled, or when they want only a plan. Do not report blocked to ask questions: call ask_user instead.",
        ],
      });

      try {
        let prompt = delivery.message.body;

        while (true) {
          const report = await runChattoAgent(
            ctx,
            {
              runOutcome: coordinator.runOutcome.bind(coordinator),
              steer: async (text) => {
                // Keep every message in the coordinator conversation, even if
                // the specialist finishes before reading its queued copy.
                const noted = coordinator.steer(text);
                void noted.catch(() => {});

                const child =
                  children.size === 1 ? [...children][0] : undefined;
                if (child) {
                  await child.send(text).catch(() => {});
                }

                return noted;
              },
            },
            prompt,
            agentOptions,
          );

          ctx.signal.throwIfAborted();
          if (report.outcome === "failed") throw new Error(report.summary);

          const remaining = inbox.drain();
          if (remaining.length) {
            prompt = `Consider these additional messages before finishing:\n\n${remaining.join("\n\n")}`;
            continue;
          }

          const summary = report.details ?? report.summary;
          await say(summary);
          return { summary };
        }
      } finally {
        coordinator.dispose();
      }
    },
  });
}

export default createChattoCoordinatorDemo({
  directory: process.env.CHATTO_WORKING_COPY ?? "",
  model: process.env.CHATTO_PLAN_MODEL ?? "openai-codex/gpt-5.6-sol",
  post: postToChatto,
  typing: sendChattoTyping,
});
