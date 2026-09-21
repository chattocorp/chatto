import { connectAgent } from "runling/agents";
import { task, Type } from "runling";
import { progressInstructions, type SpecialistContext } from "./agent-text.ts";
import type { ChattoAgentFactory } from "./agent-text.ts";

export const investigationInput = Type.Object({
  question: Type.String({ minLength: 1 }),
});

/** Create focused, read-only research tasks with their own agent lifetime. */
export function createChattoInvestigation({ directory, model, createAgent }: {
  directory: string;
  model: string;
  createAgent: ChattoAgentFactory;
}) {

  return (question: string) =>
    task(
      {
        name: `Investigate: ${question.replace(/\s+/g, " ").trim()}`,
        input: investigationInput,
        output: Type.String(),
      },
      async (taskCtx: SpecialistContext, { question }) => {
        const researcher = await createAgent({
          cwd: directory,
          model,
          thinkingLevel: "medium",
          tools: ["read", "grep", "find", "ls"],
          resources: {
            extensions: false,
            skills: false,
            promptTemplates: false,
          },
          instructions: [
            ...progressInstructions,
            "Investigate the requested repository facts without modifying files.",
            "Answer only the assigned question. Use targeted searches and stop once you have enough evidence; do not perform a broad repository audit.",
            "Return concise findings with file paths. Do not ask the user questions; explain gaps to the coordinator.",
          ],
        });

        try {
          await using connection = connectAgent(taskCtx, researcher, {
            inbox: taskCtx.inbox,
            onText: (text) => taskCtx.emit({ type: "text", text }),
          });

          const report = await connection.runOutcome(question);
          if (report.outcome === "failed") throw new Error(report.summary);

          await taskCtx.emit({
            type: "text",
            text: "Investigation complete. Reviewing the findings for the plan.",
          });

          return report.details ?? report.summary;
        } finally {
          researcher.dispose();
        }
      },
    );
}
