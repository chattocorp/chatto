import { input as askInput, runAgent, step, concat, task, Type } from "runling";

const model = "openai-codex/gpt-5.6-sol";

export const research = task(
  {
    name: "Research topic",
    input: Type.Object({ directory: Type.String({ minLength: 1 }), prompt: Type.String({ description: "The topic to research" }) }),
    output: Type.Object({
      summary: Type.String(),
      details: Type.Optional(Type.String()),
      outputs: Type.Object({ research: Type.String() }),
    }),
  },
  async (ctx, { directory, prompt: input }) => {
    const topic =
      input.trim() === ""
        ? await askInput(ctx, "What topic should I research?")
        : input;

    const report = await step("Researching topic", () =>
      runAgent(
        ctx,
        concat(
          "Research the following topic using the web_fetch tool:",
          topic,
          "",
          "Consult multiple relevant sources, reconcile material disagreements, and distinguish sourced facts from your own synthesis.",
          "If you do not know suitable source URLs, begin by fetching a public search-results page.",
        ),
        {
          cwd: directory,
          model,
          thinkingLevel: "medium",
          tools: ["web_fetch"],
          instructions: [
            "Base the report on information retrieved with web_fetch during this run.",
            "Prefer primary and authoritative sources when available.",
            "Put a concise conclusion in the report summary.",
            "Put the complete Markdown research report in the report details, with source links next to the claims they support.",
          ],
        },
      ),
    );

    const research = report.details ?? report.summary;
    return {
      summary: report.summary,
      details: report.details,
      outputs: { research },
    };
  },
);

export default research;
