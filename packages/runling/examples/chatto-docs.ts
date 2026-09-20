import { input as askInput, runAgent, step, concat, task, Type } from "runling";

const model = "openrouter/z-ai/glm-5.3-flash";
const documentationRoot =
  "https://docs.chatto.run/getting-started/introduction/";

export const chattoDocs = task(
  {
    name: "Answer Chatto documentation question",
    input: Type.Object({ directory: Type.String({ minLength: 1 }), prompt: Type.String({ description: "A question about Chatto" }) }),
    output: Type.Object({
      summary: Type.String(),
      details: Type.Optional(Type.String()),
      outputs: Type.Object({ answer: Type.String() }),
    }),
  },
  async (ctx, { directory, prompt: input }) => {
    const question =
      input.trim() === ""
        ? await askInput(ctx, "What would you like to know about Chatto?")
        : input;

    const report = await step("Consulting Chatto documentation", () =>
      runAgent(
        ctx,
        concat(
          "Answer this question using the official Chatto documentation:",
          question,
          "",
          `Start by fetching ${documentationRoot}`,
          "Follow relevant documentation links from that page as needed before answering.",
        ),
        {
          cwd: directory,
          model,
          thinkingLevel: "low",
          tools: ["web_fetch"],
          instructions: [
            "Use only pages on docs.chatto.run as factual sources.",
            "Base the answer on pages retrieved with web_fetch during this run, not prior knowledge.",
            "Put a concise direct answer in the report summary.",
            "Put supporting explanation in the report details, linking the documentation pages next to the claims they support, but keep it to two links max.",
            "State clearly when the documentation does not answer some part of the question.",
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

export default chattoDocs;
