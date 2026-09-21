import { mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { agent, exec, log, step, concat, randomId, task, Type, type WorkflowContext, type WorkflowResult } from "runling";
import { implement } from "./implement.ts";
import { review } from "./review.ts";

const worktreesDirectory = "../runling-worktrees";
const model = "openai-codex/gpt-5.6-sol";
const thinkingLevel = "medium";

// Ground generated PR copy in both the agent's intent and the committed diff.
export const describePullRequest = (
  ctx: WorkflowContext,
  directory: string,
  implementationSummary: string,
  committedChange: string,
) => step("Describe pull request", async () => {
    await using writer = await agent({
      cwd: directory,
      model,
      thinkingLevel,
      tools: ["read", "grep", "find", "ls"],
      instructions: ["Inspect the repository without modifying it."],
    });

    const report = await writer.run(
      ctx,
      concat(
        "Write the title and Markdown description for a pull request containing the current commit.",
        "Use the supplied commit and diff, inspecting repository files when useful.",
        "Use your report summary as the concise pull request title and report details as the complete Markdown body.",
        "Mention that `pnpm run check` and `pnpm test` passed.",
        "",
        "Implementation summary:",
        implementationSummary,
        "",
        "Commit and diff:",
        committedChange,
      ),
    );

    return {
      title: report.summary.slice(0, 120),
      body: report.details ?? implementationSummary,
    };
  });

export const postReview = (
  directory: string,
  pullRequestUrl: string,
  result: WorkflowResult,
) => step("Post review", async () => {
    const body = result.details ?? result.summary;
    await exec`gh pr comment ${pullRequestUrl} --body ${body}`.cwd(directory);
  });

export const createWorktree = (
  directory: string,
  branchName: string,
  worktreePath: string,
) => step("Create worktree", async () => {
    // Ask GitHub instead of assuming the default branch is named main.
    const baseBranch = (
      await exec`gh repo view --json defaultBranchRef --jq .defaultBranchRef.name`.cwd(directory).text()
    ).trim();
    if (baseBranch === "") {
      throw new Error("GitHub did not report a default branch");
    }

    await exec`git fetch origin +refs/heads/${baseBranch}:refs/remotes/origin/${baseBranch}`.cwd(directory);
    await exec`git worktree add -b ${branchName} ${worktreePath} origin/${baseBranch}`.cwd(directory);
  });

const makePullRequest = task(
  {
    name: "Make pull request",
    input: Type.Object({ directory: Type.String({ minLength: 1 }), prompt: Type.String({ description: "The requested code change" }) }),
    output: Type.Object({
      summary: Type.String(),
      outputs: Type.Object({
        branchName: Type.String(),
        pullRequestUrl: Type.String(),
      }),
    }),
  },
  async (ctx, { directory, prompt: input }) => {
    await exec`gh auth status`.cwd(directory);

    const worktreeId = randomId();
    const branchName = `runling/${worktreeId}`;
    const worktreesPath = resolve(directory, worktreesDirectory);
    const worktreePath = resolve(worktreesPath, worktreeId);

    // Keep implementation work isolated from the caller's checkout.
    await mkdir(worktreesPath, { recursive: true });
    await createWorktree(directory, branchName, worktreePath);
    log.info(`Working in ${worktreePath}`);

    await exec`pnpm install --frozen-lockfile`.cwd(worktreePath);
    const implementationSummary = await implement(ctx, { directory: worktreePath, prompt: input });

    // Review the complete staged change before capturing it in a commit.
    await exec`git add --all`.cwd(worktreePath);
    const reviewResult = await review(ctx, { directory: worktreePath, prompt: "" });
    await exec`git commit -m ${implementationSummary}`.cwd(worktreePath);

    // Describe exactly what will appear in the pull request.
    const committedChange =
      await exec`git show --format=fuller --stat --patch --no-ext-diff HEAD`.cwd(worktreePath).text();
    const pullRequest = await describePullRequest(
      ctx,
      worktreePath,
      implementationSummary,
      committedChange,
    );
    await exec`git push --set-upstream origin ${branchName}`.cwd(worktreePath);
    const createdPullRequest =
      await exec`gh pr create --head ${branchName} --title ${pullRequest.title} --body ${pullRequest.body}`.cwd(worktreePath).quiet();
    const pullRequestUrl = createdPullRequest.stdout.toString().trim();
    await postReview(worktreePath, pullRequestUrl, reviewResult);

    // Failed runs intentionally retain their worktree for inspection.
    await exec`git worktree remove ${worktreePath}`.cwd(directory);

    return {
      summary: `Opened ${pullRequestUrl}`,
      outputs: {
        branchName,
        pullRequestUrl,
      },
    };
  },
);

export default makePullRequest;
