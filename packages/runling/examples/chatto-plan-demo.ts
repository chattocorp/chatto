import { realpath } from "node:fs/promises";
import { agent, input, task, Type, TimeoutError, type RunlingAgent } from "runling";
import { createChattoWebhook, messageSignal, postToChatto, type ChattoPost } from "./chatto/webhook.ts";
import { withChattoTyping, sendChattoTyping, type ChattoTyping } from "./chatto/typing.ts";
import { runChattoAgent } from "./chatto/agent.ts";
import { refreshCheckout } from "./chatto/checkout.ts";

type Planner = Pick<RunlingAgent, "runOutcome" | "dispose" | "steer">;
export interface PlanDemoOptions {
  directory: string;
  post: ChattoPost;
  typing?: ChattoTyping;
  /** Time allowed for each human answer, in seconds. */
  timeout?: number;
  model?: string;
  refresh?: typeof refreshCheckout;
  createPlanner?: (directory: string) => Promise<Planner>;
}

// Hold the checkout for the entire interview, including input waits.
const processState = globalThis as typeof globalThis & {
  __runlingChattoPlanningDirectories?: Set<string>;
};
const busyDirectories = processState.__runlingChattoPlanningDirectories ??= new Set<string>();

const implementationPreview = task(async function implementationMode(_ctx, plan: string, revision: string) {
  return { status: "implementation-stub", plan, revision };
});

export function createChattoPlanDemo({
  directory, post, typing, timeout = 900, model = "openai-codex/gpt-5.6-sol",
  refresh = refreshCheckout,
  createPlanner = cwd => agent({
    cwd, model, thinkingLevel: "medium",
    tools: ["read", "grep", "find", "ls"],
    resources: { extensions: false, skills: false, promptTemplates: false },
    instructions: [
      "You are planning a Chatto feature or bug fix with its user. Inspect the repository; do not modify files or implement anything.",
      "Ask one concise question at a time when its answer materially affects the plan. Find repository facts yourself.",
      "Report blocked with your question in summary when you need an answer.",
      "Report completed when you have a plan ready for review. Put the Markdown plan in details: goal, relevant code, decisions, implementation steps, and tests.",
      "Keep the plan under 6000 characters so it is readable in chat. Revise it when the user gives feedback.",
      "Only the workflow handles /implement and /cancel. Never claim implementation has started or finished.",
    ],
  }),
}: PlanDemoOptions) {
  if (!Number.isFinite(timeout) || timeout < 0 || Math.ceil(timeout * 1000) > 2_147_483_647) {
    throw new RangeError("timeout must be seconds between 0 and 2147483.647");
  }
  return createChattoWebhook({
    name: "Chatto plan demo",
    output: Type.Object({ status: Type.String(), plan: Type.String(), revision: Type.String() }),
    post,
    async run(ctx, delivery, destination, inbox) {
      // Check at invocation so an unused demo cannot prevent the console starting.
      if (!directory.trim()) {
        throw new Error("Set CHATTO_WORKING_COPY in packages/runling/.env before running this demo");
      }
      const say = (text: string) => post(destination, text, messageSignal(ctx));
      let root: string | undefined;
      let acquired = false;
      try {
        root = await realpath(directory);
        if (busyDirectories.has(root)) {
          await say("I am already planning another change in this checkout. Please send a new DM after that conversation ends.");
          return { status: "busy", plan: "", revision: "" };
        }
        busyDirectories.add(root);
        acquired = true;
        await say("I’ll update my Chatto checkout and investigate your request. Send additional thoughts at any time. Use /cancel to stop; /implement accepts a completed plan (demo only).");
        const revision = await withChattoTyping(ctx, destination, typing, () => refresh(ctx, root!));
        const planner = await withChattoTyping(ctx, destination, typing, () => createPlanner(root!));
        try {
          let message = `Plan this Chatto change at revision ${revision}:\n\n${delivery.message.body}`;
          const feedback = async () => {
            const feedback: string[] = [];
            while (true) {
              const messages = inbox.drain();
              feedback.push(...messages.filter(value => value.trim() !== "/implement"));
              if (!messages.some(value => value.trim() === "/implement")) return feedback;
              await say("Please wait for a completed plan, then send /implement to accept it. That command was not saved as approval.");
              // Messages can arrive while the notice is sent. Include those too.
            }
          };
          while (true) {
            ctx.signal.throwIfAborted();
            const before = await feedback();
            if (before.length) message += `\n\nAdditional user messages:\n${before.join("\n\n")}`;
            const report = await runChattoAgent(ctx, planner, message, {
              destination, inbox, post, typing, reservedCommands: ["/implement"],
            });
            if (report.outcome === "failed") throw new Error(`Planning failed: ${report.summary}`);
            const during = await feedback();
            if (during.length) {
              await say("I received your additional messages and will include them before posting the next question or plan.");
              message = `Additional user messages:\n\n${during.join("\n\n")}\n\nIncorporate these before asking a question or presenting the revised plan.`;
              continue;
            }
            const plan = report.outcome === "completed" ? report.details ?? report.summary : "";
            const question = plan
              ? `${plan}\n\nReply with feedback to revise this plan, /implement to accept it, or /cancel to stop. Implementation is a stub in this demo.`
              : `${report.summary}\n\nReply to continue planning, or /cancel to stop.`;
            let answer: string;
            while (true) {
              answer = await input(ctx, question, { timeout });
              if (answer.trim() !== "/implement" || plan) break;
              await say("There is no completed plan to accept yet. Please answer the planning question first.");
            }
            // Do not yield between checking pending feedback and accepting a plan.
            const extra = inbox.drain().filter(value => value.trim() !== "/implement");
            if (answer.trim() === "/implement" && extra.length) {
              await say("I’ll review your additional feedback first. Please approve the updated plan when it is ready.");
            }
            if (answer.trim() === "/implement" && !extra.length) {
              const result = await implementationPreview(ctx, plan, revision);
              await say("Implementation mode reached. This is a demo stub: the plan is accepted, but no implementation files were changed.");
              return result;
            }
            await say("Thanks — I’m reviewing your answer and updating the plan.");
            message = `The user replied:\n\n${answer.trim() === "/implement" ? "Review the additional feedback; the previous approval is no longer applicable." : answer}\n\n${extra.join("\n\n")}\n\nContinue planning. Ask the next material question or return the revised plan for review.`;
          }
        } finally {
          planner.dispose();
        }
      } catch (error) {
        if (!(error instanceof TimeoutError) && !ctx.signal.aborted) {
          await say("Planning stopped because an operation failed. Check this run in Runling for details, then send a new DM to retry.").catch(() => {});
        }
        throw error;
      } finally {
        if (acquired) busyDirectories.delete(root!);
      }
    },
  });
}

export default createChattoPlanDemo({
  directory: process.env.CHATTO_WORKING_COPY ?? "",
  model: process.env.CHATTO_PLAN_MODEL ?? "openai-codex/gpt-5.6-sol",
  post: postToChatto,
  typing: sendChattoTyping,
});
