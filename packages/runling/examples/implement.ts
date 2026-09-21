import { agent, exec, step, concat, task, Type } from "runling";
import * as git from "runling/git";

const model = "openai-codex/gpt-5.6-sol";
const thinkingLevel = "medium";
const agentInstructions = [
  "Write tests for new or changed features.",
  "Summarize what you changed and why in your final report.",
];
const maxValidationAttempts = 3;
const maxValidationFeedbackLength = 18_000;

const validate = (directory: string) =>
  step("Validate", async () => {
    const check = await step("Run checks", () =>
      exec`pnpm run check`.cwd(directory).nothrow(),
    );
    return check.exitCode === 0
      ? step("Run tests", () => exec`pnpm test`.cwd(directory).nothrow())
      : check;
  });

const validationFailure = (validation: {
  stdout: Uint8Array;
  stderr: Uint8Array;
}) => {
  const output = [validation.stdout.toString(), validation.stderr.toString()]
    .filter((part) => part.trim() !== "")
    .join("\n")
    .trim();
  const details = output === "" ? "Validation failed without output." : output;

  return details.length <= maxValidationFeedbackLength
    ? details
    : `${details.slice(0, maxValidationFeedbackLength)}\n\n[Validation output truncated]`;
};

export const implement = task(
  {
    name: "Implement",
    input: Type.Object({ directory: Type.String({ minLength: 1 }), prompt: Type.String({ description: "The requested code change" }) }),
    output: Type.String({ description: "A summary of the implementation" }),
  },
  async (ctx, { directory, prompt: input }): Promise<string> => {
    const pwd = await git.getPwd(directory);

    await using implementationAgent = await agent({
      cwd: directory,
      model,
      thinkingLevel,
      instructions: agentInstructions,
    });

    let implementationReport = await step("Implementing change", () =>
      implementationAgent.run(ctx, input),
    );

    if (!(await pwd.hasChanges)) {
      throw new Error("Agent completed without changing the worktree");
    }

    let validation = await validate(directory);
    for (let attempt = 1; validation.exitCode !== 0; attempt++) {
      if (attempt === maxValidationAttempts) {
        throw new Error(
          `Project validation failed after ${maxValidationAttempts} attempts.\n${validationFailure(validation)}`,
        );
      }

      implementationReport = await step(
        `Repairing validation (attempt ${attempt}/${maxValidationAttempts})`,
        () =>
          implementationAgent.run(
            ctx,
            concat(
              "Project validation failed. Fix the implementation and tests so that both `pnpm run check` and `pnpm test` pass.",
              "In your report, summarize the complete implementation, including this repair.",
              "",
              "Failure output:",
              validationFailure(validation),
            ),
          ),
      );

      validation = await validate(directory);
    }

    if (!(await pwd.hasChanges)) {
      throw new Error("The validated worktree no longer contains any changes");
    }

    return implementationReport.summary;
  },
);

export default implement;
