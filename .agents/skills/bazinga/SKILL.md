---
name: "bazinga"
description: "Invoke this skill when the user asks you to develop a feature back-to-back, with all the trimmings. This skill defines a full feature development workflow that includes code reviews, branch management, and ends in a fully fleshed-out PR."
---

Hi. We're here to develop a feature, or make a change, that the user has asked you for. We're going to follow a back-to-back workflow that consists of the following steps:

- Environment Setup
- Planning
- Planning Checkpoint
- Loop:
  - Writing code
  - Running tests
  - Code review
- Final Head Gate
- Loop:
  - Publish a Pull Request
  - Watch CI
- Final Report

## The Context File

We're going to track the state of our work in a Markdown file in the gitignored `.context/` directory. Please create this file, use it to jot down any thoughts and notes you want to keep in the context, but most importantly: include a checkbox item list of the above steps, including separate "Planning Checkpoint", "Final Head Gate", and "Watch CI" items, and cross them off as things progress.

## Looping

It is imperative to note that the three steps "Writing Code", "Running tests" and "Code review" are expected to loop until the code review finds no more significant issues with your work.

## The Individual Phases

### Environment Setup

- Make sure you work in a git worktree. The user is likely using an agent orchestrator that will already have set this up for you. If we're not in a worktree, stop and alert the user.
- Make sure to name the branch something that properly reflects the work being done. Please follow any instructions the user has provided about the naming conventions of these branches. When in doubt, use Conventional Commit style branch names (eg. `fix/...`, `feat/...` etc.)

### Planning

- Make sure you fully understand the work the user is asking you to do.
- When in doubt, ask the user questions.
- If the user specifies a GitHub Issue for this work, the PR you create must reference the issue (so it can be auto-closed when the PR is merged.)
- Jot down any additional details in your context file.
- Inspect the relevant code and docs before making material design choices.
- For asynchronous, durable, concurrent, multi-replica, or persistence-sensitive work, write a compact failure/interleaving matrix before implementation. Cover the applicable cases: partial work, failed acknowledgement, retry/redelivery, projection or cache lag, cross-replica execution, restart/recovery, persisted cutoff or snapshot rotation, authorization loss/regain, and concurrent access. Use the matrix to drive implementation and tests.

### Planning Checkpoint

- For non-trivial feature work, do not begin implementation immediately after planning. First present a concise implementation brief to the user and call out material choices.
- Treat a change as non-trivial when it affects architecture, persisted data, API shape, permissions, compatibility, user-facing workflows, or documentation commitments.
- The brief should cover intended behavior, data/event/API model, compatibility or migration concerns, UI placement and user flows, expected tests/docs, and any open questions or assumptions.
- Proceed only after the user confirms, or after clearly stating that no material choices are open. Skip this checkpoint only for truly small, low-risk changes or when the user explicitly asks you to proceed without discussion.
- Record the brief and the user's confirmation or the reason for skipping in the context file.

### Writing code

- Write code to implement the requested feature or change as you would normally do.
- Be sure to follow any additional guidance the user may have given you for this.
- Write new tests as you go along, or update existing tests. Please respect the user's preference for tests.
- If the task involves visual work, use a browser or Chrome DevTools MCP to verify your work, in case these are available to you. Also follow any guidance given by design-focused skills and instructions.

### Running tests

- Run relevant tests before making or pushing commits.
- Avoid running the entire test suite unless you think it's justified. Remember that CI will ultimately run the complete test suite for us.

### Code Review

- If you're working on a branch/in a PR, make a commit before having the review done. This allows us to see how the code evolves across multiple reviews and commits. Create individual commits for each fix you make.
- Record the exact reviewed HEAD SHA, target base, diff scope, findings, and dispositions in the context file. Review the complete target-base-to-HEAD diff; an incremental review of only the latest fix is not a final review.
- Perform an adversarial review of the changes. Please consult any skills related to this for guidance. Specifically, review the changes for the following:
  - Code correctness (the changes do what they are supposed to do, and don't break existing functionality)
  - Performance impact (avoid significant performance regressions!)
  - Security impact (avoid introducing security vulnerabilities!)
  - Complexity (prefer simple, readable, maintainable code over clever or complex solutions; identify obvious simplification opportunities as review findings)
  - Documentation (ensure that code comments match, and changes, where appropriate and feasible, are documented in the way the repository expects)
- Address any findings identified in the Code Review.
- Add a regression test for every material finding when reasonably possible, and confirm that it exercises the pre-fix failure. If a regression test is impractical, record why.
- Run an appropriate race detector or concurrency-focused test when the change introduces or materially alters concurrent behavior and the project supports one.
- Treat every commit, merge, rebase, generated-artifact update, or other HEAD mutation as invalidating all previous clean verdicts.
- Repeat the last three steps, including this one, until the code review comes up empty, or only reports findings you don't find necessary to address.

### Final Head Gate

- After the implementation loop, commit all intended changes and identify the exact final HEAD SHA.
- Perform one final adversarial review of the complete diff from the intended target base to that exact SHA. The review must explicitly report the SHA and either list findings or declare the diff clean.
- Do not publish or update the PR from a stale clean verdict. Any subsequent repository mutation returns the workflow to Running tests and Code Review, followed by a new Final Head Gate.
- Do not mark this gate complete merely because tests are green; tests and review are independent evidence.

### Pull Request

- Finally, post a Pull Request with the changes.
- Verify that the PR title and body are accurate after creating or editing it.

### Watch CI

- After opening or updating the PR, wait until GitHub has attached checks to the current PR head. If `gh pr checks <pr>` says no checks are reported, wait briefly and poll again; do not treat that as success.
- Monitor CI with compact structured queries such as `gh pr checks <pr> --json name,state,link`. Poll at a reasonable interval and report only state changes. Do not use `gh run watch`, `gh pr checks --watch`, or another command that repeatedly prints the complete job matrix.
- Confirm that the observed checks belong to the current PR HEAD. Store the last observed status in the context file so unchanged polls do not consume conversation context.
- On failure, query only the failed job and fetch only its failed logs, for example with `gh run view <run-id> --job <job-id> --log-failed` or the equivalent GitHub API endpoint.
- Before rerunning a failed workflow, check whether another workflow for the same PR HEAD is active. Wait for that run first to avoid duplicate work and concurrency cancellation.
- If CI fails, examine the failure logs, try to fix the error, push your fixes, and repeat the CI watch loop from the new PR head until CI is green.
- If you have had to fix CI failures, re-run the code review step to ensure that your fixes did not introduce new issues.
- Do not send the final completion report until CI is green, or until you clearly report a blocker that prevents CI from being observed or fixed.

### Final Report

- Report to the user what you did. If there were Code Review findings that you decided to not address, inform the user about them, together with an explanation why you decided that way.
- Also include a list of review agent findings that you addressed, but keep it concise. This way the user will know that the workflow actually worked.
- Voice any suggestions for new or changed agent rules/instructions that might have helped you get things right faster, or that you feel will help future agent sessions. Ask the user if they want you to apply these changes.
