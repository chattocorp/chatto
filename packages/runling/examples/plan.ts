import { agent, input as askInput, concat, task, Type } from "runling";

const model = "openai-codex/gpt-5.6-sol";
const thinkingLevel = "medium";

export const plan = task(
  {
    name: "Plan change",
    input: Type.Object({ directory: Type.String({ minLength: 1 }), prompt: Type.String({ description: "The change to plan" }) }),
    output: Type.Object({
      summary: Type.String(),
      details: Type.Optional(Type.String()),
      outputs: Type.Object({ plan: Type.String() }),
    }),
  },
  async (ctx, { directory, prompt: input }) => {
    const request =
      input.trim() === ""
        ? await askInput(ctx, "What would you like to build or change?")
        : input;

    await using planner = await agent({
      cwd: directory,
      model,
      thinkingLevel,
      tools: ["read", "grep", "find", "ls"],
      instructions: [
        "Inspect the repository without modifying it.",
        "Interview the human before finalizing the implementation plan.",
        "Ask one question at a time, only when its answer would materially change the implementation.",
        "Do not ask questions whose answers can be discovered from the repository.",
        "To ask a question, report a blocked outcome and put the complete concise question in the report summary.",
        "When the request is sufficiently specified, report a completed outcome with a concise summary and a detailed Markdown implementation plan.",
        "The plan should capture the goal, relevant repository context, decisions made, implementation steps, validation, and material risks.",
      ],
    });

    let message = concat(
      "Investigate this request, then begin the planning interview.",
      "Ask a question rather than making an arbitrary choice whenever the answer would materially affect the result.",
      "",
      "Request:",
      request,
    );

    while (true) {
      const report = await planner.runOutcome(ctx, message);

      if (report.outcome === "completed") {
        const completedPlan = report.details ?? report.summary;
        return {
          summary: report.summary,
          details: report.details,
          outputs: { plan: completedPlan },
        };
      }

      if (report.outcome === "failed") {
        throw new Error(`Planning failed: ${report.summary}`);
      }

      const answer = await askInput(ctx, report.summary);
      message = concat(
        "The human answered your question:",
        answer,
        "",
        "Continue the interview. Ask the next material question, or complete the plan if the request is now sufficiently specified.",
      );
    }
  },
);

export default plan;
