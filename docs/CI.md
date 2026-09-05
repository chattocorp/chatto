# Continuous integration

The `ci` workflow selects checks from the changed files. It uses the merge base
for pull requests, including stacked pull requests. Push runs use the previous
commit from the push event. Unknown paths select all checks.

## Check selection

| Change | Selected checks |
| --- | --- |
| Repository prose and agent instructions | License metadata, workflow lint, CI helper tests, selection validation |
| Docs website, including Markdown content | Production docs build |
| Desktop source and desktop build tools | Desktop bundles, native macOS tests, workspace checks |
| Chatto frontend, API types, or Lingua | Workspace checks and tests, desktop, Chatto tests and E2E, protobuf checks, performance |
| Chatto Go source | Chatto tests and E2E, protobuf checks, performance |
| Authling source | Authling Go and browser tests, Chatto–Authling login integration |
| Authling protobufs or Buf configuration | Authling checks and protobuf checks |
| Shared Go modules | Shared module tests, both products' Go tests, both products' E2E, protobuf checks, performance |
| Either product's Go module dependencies | All Go module checks and both products' E2E, protobuf checks, performance |
| Root dependencies, tool configuration, CI files, or unknown paths | All checks |

License and workflow checks run on every revision. The existing required check
names remain in use. A job that is not needed is skipped, except for the four required E2E
checks, which report success without running test steps. The required
`codegen-proto-drift` job always verifies that selection succeeded, even when no
protobuf work is needed. A selection failure must not turn the required checks
green through skips.

The four Chatto E2E check names remain present. For an Authling-only change,
shard 1 runs the login integration test. The other three jobs do not install
dependencies or build servers. Media tests run separately for Chatto changes.

Main pushes still build an image for every commit. Publication accepts selected
checks that pass and checks that were not needed. A failed or cancelled
prerequisite prevents publication.

A daily run at 04:23 UTC executes the full suite. Use **Run workflow** on `ci`
with `suite=full` to run all checks, or `suite=performance` to run the performance
comparison. Scheduled and manual performance runs compare against the previous
commit. Pull requests compare against their base revision. Both revisions run
on the same runner.

## E2E shard assignment

Each E2E job keeps its own build and browser setup. There is no shared build job.
`tools/ci/e2e-shards.mjs` lists the current Playwright tests, groups them by file,
and assigns the longest estimated file to the shard with the least estimated
work. This keeps serial suites together. Playwright still runs parallel tests
within each shard.

The estimates in `tools/ci/e2e-duration-hints.json` are mean test durations in
milliseconds by file. They were collected from successful test results in
[CI run 33960213979](https://github.com/chattocorp/chatto/actions/runs/33960213979)
on 5 September 2026. Retry attempts are not included. The estimates affect
assignment only. New files use the median estimate; deleted files are ignored.
Each current file belongs to exactly one shard. Keep the same estimates in all
four jobs. Update them together when measured durations change substantially.

To inspect assignment without starting servers or browsers:

```sh
mise x -- node tools/ci/e2e-shards.mjs 1/4 --list
```

The command writes the assignment to `.context/ci/e2e-shards-1.json`. The
estimated test duration is the sum of test durations, not job elapsed time.
Runner contention, setup, and retries can change the measured job duration.

## Go package parallelism

Use **Run workflow** with `suite=benchmark-go` to compare `-p 1` and `-p 2` on
one Ubuntu runner and one revision. The benchmark first compiles the tests. It
then runs the full Chatto Go suite in the order 1, 2, 2, 1, with the test-result
cache disabled. The summary contains elapsed times and exit codes. Logs and
measurements are available in the `go-parallelism-benchmark` artifact.

Do not increase concurrency based on compilation time alone. Check every pass
for failures and compare both test passes for each setting.

## Public API waiver changes

Ordinary label changes do not start or cancel `ci`. Changes to
`api-breaking-change` start `proto-label.yml`. This workflow reads only trusted
default-branch code. It waits for the current revision's CI run to finish, then
requests a rerun of `codegen-proto-drift`. It does not execute pull-request code
with its write token. The rerun uses the original job's read-only permissions
and reads the current labels before it checks compatibility.

The label workflow becomes active after it is merged into the default branch.
If it cannot find a completed CI run within 20 minutes, it fails with a request
to rerun the protobuf job. For CI runs that predate live waiver checks, update
the PR branch from main and start a new CI run first. Label changes do not
bypass persisted-schema checks.
