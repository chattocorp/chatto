import { input as askInput, runAgent, step, concat, task, Type } from "runling";

const model = "openai-codex/gpt-5.6-sol";

export const ask = task(
  {
    name: "Answer repository question",
    input: Type.Object({ directory: Type.String({ minLength: 1 }), prompt: Type.String({ description: "A question about the repository" }) }),
    output: Type.Object({
      summary: Type.String(),
      details: Type.Optional(Type.String()),
      outputs: Type.Object({ answer: Type.String() }),
    }),
  },
  async (ctx, { directory, prompt: input }) => {
    const question =
      input.trim() === ""
        ? await askInput(ctx, "What would you like to know about the repository?")
        : input;

    const report = await step("Investigating repository", () =>
      runAgent(
        ctx,
        concat(
          "Answer this question about the repository:",
          question,
          "",
          "Inspect the repository before answering. Ground the answer in the current files rather than assumptions.",
        ),
        {
          cwd: directory,
          model,
          thinkingLevel: "medium",
          tools: ["read", "grep", "find", "ls"],
          instructions: [
            "Inspect the repository without modifying it.",
            "Answer only from evidence available in the repository.",
            "Put a concise direct answer in the report summary.",
            "Put supporting explanation in the report details, citing relevant file paths and line numbers.",
            "State clearly when the repository does not contain enough information to answer part of the question.",
          ],
        },
      ),
    );

    const answer = report.details ?? report.summary;
    return {
      summary: report.summary,
      details: report.details,
      outputs: { answer },
    };
  },
);

export default ask;
