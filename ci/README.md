# Local Chatto CI

This Runling project runs Chatto's first local CI jobs in Docker. It is a
development tool in this repository. It does not receive GitHub events or
publish GitHub checks.

## Run the tests

Install the repository dependencies with `mise setup-frontend`. Start a local
Docker daemon, such as OrbStack, then run from the repository root:

```sh
mise x -- pnpm --dir ci test:chatto
```

The pipeline builds the `chatto-ci:local` test image, then runs two child tasks
concurrently. Docker caches the image layers between runs. Each job uses the
immutable image ID from its own build.
Runling owns image building, subprocess output, cancellation, and private source
manifests. This project supplies Chatto's tool versions, container setup, and jobs.

| Job      | Work                                                                                                                                   |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Backend  | `mise run --skip-tools test-cli`: build the frontend, prepare legal files, run the development seed smoke test, then run the Go tests. |
| Frontend | `mise run --skip-tools test-frontend`: run server, Chromium component, and Storybook unit tests.                                       |

Each job starts with an empty dependency and Go test cache. Each test
container has four CPUs, a 6 GB memory limit, and a 30-minute deadline. The image
includes the Go and Node versions from `mise.toml`, pnpm from the root package
manifest, and Chromium matching the installed frontend Playwright package.

To run one job:

```sh
mise x -- pnpm --dir ci runling run workflows/tests.ts --input '{"jobs":["backend"]}'
mise x -- pnpm --dir ci runling run workflows/tests.ts --input '{"jobs":["frontend"]}'
```

To use the console and timeline:

```sh
mise x -- pnpm --dir ci start
```

Open the printed local URL and start **tests** with `{}`. Use `{"jobs":["backend"]}`
or `{"jobs":["frontend"]}` to select one job. Stop the server when finished.
Run history is in `ci/.runling/`. There is no need for model credentials.

Test output streams to the terminal and the run journal. In the console's
**Log** view, expand **Chatto Go tests** or **Chatto frontend tests** to read
that job's output. You can also select the task in **Timeline** to open its
activity details. Each job includes its Docker command output, with separate logs
for the two jobs. Saved runs retain the output after a server restart.
Output opens at the latest entry. Scroll up to inspect earlier output. Use
**Beginning** to jump to the start and **Latest output** to follow the job again.
**Download log** saves the full job log.

## Pipeline code

`docker/image.ts` defines the image-build task. `workflows/tests.ts` defines the backend
and frontend test tasks, then connects them: build once and pass the image ID to
both tasks. Each test task calls `docker.run` with its own command. Invocation
handles stop a pending sibling when a job fails.

`docker/Dockerfile` supplies the tools. `docker/container.sh` only copies the selected source
into the writable workspace and executes the command supplied by the task.
Chatto's existing `mise.toml` owns dependency installation, builds, and test setup.
The jobs use `--skip-tools` because the image already supplies the required tools.

## Source and cleanup

The pipeline mounts this checkout at `/repo` as read-only. A host-side Git file
list includes tracked and unignored files, including local modifications and
new files. Deleted files, ignored dependencies, build output, local environment
files, and linked-worktree Git metadata are not copied into the test workspace.
Tracked files remain included even if an ignore rule matches them.

Each container copies those files into its own `/workspace`, then installs
Linux dependencies with the frozen lockfile. Host `node_modules`, build output,
and application data remain unchanged. Source is read when the job starts; do
not edit files during a run when the result must describe one consistent tree.

Cancellation stops the job containers. If one job fails, the root task cancels
and awaits its pending sibling. Each test task owns its source manifest and
releases it after its container stops.
Runling removes the containers, including their writable workspaces. It retains
the local test image and Docker build cache for later runs. Dependency caches
inside test containers are currently disposable.

Docker must use a local context that can bind-mount this checkout. The whole
checkout is visible through the read-only mount, including ignored files, even
though only selected files enter the test workspace. Run this project only on a
trusted local daemon. Host credentials and the Docker socket are not passed to
test containers.

Image builds contact Docker Hub, GitHub Container Registry, Microsoft's container registry, Ubuntu package
mirrors, and npm. Test setup contacts npm, the Go module proxy and checksum
database, and any upstream hosts needed for module downloads. These services see
the Docker host's IP address and requested packages. The pipeline does not upload
the repository or test output. Package telemetry is disabled in the image.

## Scope

This first version covers Chatto's CLI and frontend test suites. It does not yet
run the shared Go module suites, other workspace packages, Authling, browser E2E
tests, desktop checks, lint, compatibility tests, or release jobs. These remain
in the existing GitHub workflows. No existing workflow is replaced.

Check the CI project itself without starting Docker:

```sh
mise x -- pnpm --filter @chatto/ci check
mise x -- pnpm --filter @chatto/ci test
```

Its tests check Chatto's image version selection. Generic build and source
manifest tests live in Runling.
