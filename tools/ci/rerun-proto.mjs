/** Recheck the current PR revision after its public API waiver changes.
 * The caller runs trusted default-branch code. It never checks out PR code.
 * GitHub reruns the original job with its original, read-only permissions.
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
  // Wait for an active run to finish; GitHub cannot rerun an active workflow.
  // Re-read the PR each time so a new commit supersedes an old label event.
  for (let attempt = 0; attempt < 60; attempt++) {
    const { data: pr } = await github.rest.pulls.get({
      ...context.repo,
      pull_number,
    });
    if (pr.state !== "open") return;
    const { data } = await github.rest.actions.listWorkflowRuns({
      ...context.repo,
      workflow_id: "ci.yml",
      event: "pull_request",
      head_sha: pr.head.sha,
      per_page: 100,
    });
    const run = data.workflow_runs.find((run) =>
      run.pull_requests.some((pull) => pull.number === pull_number),
    );
    if (run?.status === "completed") {
      const jobs = await github.paginate(
        github.rest.actions.listJobsForWorkflowRun,
        { ...context.repo, run_id: run.id, filter: "latest", per_page: 100 },
      );
      const job = jobs.find((job) => job.name === "codegen-proto-drift");
      if (!job) throw new Error("The current CI run has no protobuf check");
      await github.rest.actions.reRunJobForWorkflowRun({
        ...context.repo,
        job_id: job.id,
      });
      core.info(`Requested protobuf revalidation for CI run ${run.id}.`);
      return;
    }
    await sleep(20_000);
  }
  throw new Error(
    "No completed CI run is available. Rerun the protobuf job after CI completes.",
  );
}
