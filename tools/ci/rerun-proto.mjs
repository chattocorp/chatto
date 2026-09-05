/** Recheck the current PR revision after its public API waiver changes.
 * The caller loads only trusted default-branch code. GitHub reruns the original
 * job with its original permissions. A pending status closes the merge window
 * while the original workflow finishes and the rerun is queued.
 */
export async function rerunProto({
  github,
  context,
  core,
  sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
}) {
  if (
    !["labeled", "unlabeled"].includes(context.payload.action) ||
    context.payload.label?.name !== "api-breaking-change"
  )
    return;
  const pull_number = context.payload.pull_request.number;
  let pendingSha;
  let rerun;
  const status = (state, description) =>
    github.rest.repos.createCommitStatus({
      ...context.repo,
      sha: pendingSha,
      context: "codegen-proto-drift",
      state,
      description,
      target_url: `https://github.com/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`,
    });
  try {
    // Re-read the PR so a new commit supersedes an old label event. Allow time
    // for both the existing workflow and the protobuf rerun to finish.
    for (let attempt = 0; attempt < 120; attempt++) {
      const { data: pr } = await github.rest.pulls.get({
        ...context.repo,
        pull_number,
      });
      if (pr.state !== "open") return;
      if (pendingSha !== pr.head.sha) {
        pendingSha = pr.head.sha;
        rerun = undefined;
        await status("pending", "Revalidating the current public API waiver");
      }
      const { data } = await github.rest.actions.listWorkflowRuns({
        ...context.repo,
        workflow_id: "ci.yml",
        event: "pull_request",
        head_sha: pendingSha,
        per_page: 100,
      });
      const run = data.workflow_runs.find((run) =>
        run.pull_requests.some((pull) => pull.number === pull_number),
      );
      if (run && rerun && rerun.id !== run.id) rerun = undefined;
      if (run && (run.status === "completed" || rerun?.id === run.id)) {
        const jobs = await github.paginate(
          github.rest.actions.listJobsForWorkflowRun,
          { ...context.repo, run_id: run.id, filter: "latest", per_page: 100 },
        );
        const job = jobs.find((job) => job.name === "codegen-proto-drift");
        if (!job) throw new Error("The current CI run has no protobuf check");
        if (
          !job.steps?.some(
            (step) => step.name === "Read current public API waiver",
          )
        ) {
          throw new Error(
            "This CI run predates live waiver checks. Update the PR branch from main and start a new CI run.",
          );
        }
        if (!rerun) {
          await github.rest.actions.reRunJobForWorkflowRun({
            ...context.repo,
            job_id: job.id,
          });
          rerun = { id: run.id, attempt: run.run_attempt };
          core.info(`Requested protobuf revalidation for CI run ${run.id}.`);
        } else if (
          job.run_attempt > rerun.attempt &&
          job.status === "completed"
        ) {
          const passed = job.conclusion === "success";
          await status(
            passed ? "success" : "failure",
            passed
              ? "Public API waiver revalidated"
              : "Protobuf revalidation failed; rerun this label workflow after fixing CI",
          );
          if (!passed) throw new Error("Protobuf revalidation failed");
          return;
        }
      }
      await sleep(20_000);
    }
    throw new Error(
      "Revalidation timed out. Rerun this label workflow after CI completes.",
    );
  } catch (error) {
    if (pendingSha)
      await status(
        "failure",
        "Waiver revalidation failed; inspect and rerun this label workflow",
      );
    throw error;
  }
}
