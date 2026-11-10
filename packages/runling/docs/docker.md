# Docker containers

Import Docker helpers from `runling/docker`. Install the Docker CLI and start a
daemon, such as OrbStack or Docker Desktop. The helper uses the selected Docker
context and environment. It does not start a daemon or require a Docker SDK.

## Build an image

```ts
const image = await docker.build(ctx, {
  contextDirectory: '/absolute/path/to/build-context',
  tag: 'app-tests:local',
  buildArgs: { NODE_VERSION: '24.1.0' },
  timeoutMs: 10 * 60_000
});

await docker.run(ctx, { image, command: ['npm', 'test'] });
```

`build()` returns the immutable image ID. Later jobs can use that ID even when
another build replaces the tag. Runling owns the image-ID file and removes it
before the operation settles. The built image and daemon build cache remain
available for reuse; they are not disposable Runling containers.

Build output streams into a command activity and optional `onOutput` consumers.
Task cancellation, an additional `signal`, and an optional `timeoutMs` cancel
the CLI. The operation waits for the CLI and output streams to close. Failure
throws `DockerOperationError`; cancellation preserves the signal reason.

`contextDirectory` selects the build input. `context` selects a Docker daemon.
Docker sends build input to that daemon according to the directory's
`.dockerignore`. The daemon can contact registries and any hosts used by build
commands. These services see its IP address and requested resources.
Build arguments are public configuration and can appear in build output. Do not
pass credentials as build arguments.

## Run a command

```ts
import { docker } from 'runling/docker';

// Inside a task:
await docker.run(ctx, {
  image: 'alpine:3.22',
  command: ['sh', '-c', 'echo "Build started"; sleep 1; echo "Build passed"'],
  timeoutMs: 30_000
});
```

Commands use argument arrays. Shell syntax requires an explicit shell command.
An absent local image is pulled from its registry. Pulling contacts that registry
and reveals the Docker host's IP address and requested image. Use a pinned digest
when a pipeline requires a specific image. A selected remote Docker context
sends commands, environment values, and copied files to that daemon.

`run()` returns a container ID and exit code after exit and removal. A nonzero
exit throws `DockerExitError`, with `id` and `exitCode`. `DockerTimeoutError`
identifies an execution or readiness deadline. Cancellation rejects with the
signal's reason. `DockerOperationError` identifies a failed Docker operation
without including raw CLI diagnostics or command arguments.

Output appears under one command activity in the console and timeline. The
adapter does not accumulate a complete output buffer. Use `onOutput` to receive
`{ stream: 'stdout' | 'stderr', text: string }` chunks. A throwing callback fails
the container operation. Ordering is preserved within each stream; stdout and
stderr have no combined order. Runling splits long log lines into bounded entries.
The activity's final output buffer is empty because its output is in live logs.

## Own a service

```ts
await using network = await docker.network(ctx);
await using database = await docker.start(ctx, {
  image: 'postgres:17-alpine',
  network: network.name,
  networkAliases: ['database'],
  env: { POSTGRES_PASSWORD: databasePassword },
  ready: {
    command: ['pg_isready', '-U', 'postgres'],
    timeoutMs: 30_000
  }
});

// Start child tasks here. Dispose or await them before releasing the service.
// Other containers on this network can connect to the hostname `database`.
```

`start()` returns after container startup and, when supplied, its readiness
probe. Each failed probe is retried until its deadline. Probe output is not
recorded. Disposal stops the container, allows two seconds for its stop signal,
then removes it. The removal operation can force termination. Normal service
disposal records a completed activity. A pending `wait()` rejects when disposal
stops the service. Call `dispose()` directly when a lexical scope is unsuitable.
Disposal is safe to call more than once.

Pass the task context as the first argument of each operation. The operation
uses `ctx.signal` for cancellation and `ctx.run?.id` for its run label. Container
options can supply an additional signal; it does not replace task cancellation.
Cancellation also starts container removal.
Cleanup uses separate deadlines and can continue after the workflow deadline.
Creation and start have independent 30-second control deadlines and observe
workflow cancellation after the operation returns, so cleanup does not race a
normal daemon-side creation. Image pulling observes cancellation directly.

Networks use explicit disposal. Declare a network before its containers so
`await using` releases it last. Declare child invocations after their resources
and dispose them before those resources. This ordering also applies on failure.

## Files and resource limits

Use `mounts: [{ source: hostDirectory, target: '/work', readOnly: true }]` for bind
mounts. Source and target paths must be absolute and cannot contain commas or
control characters. Source paths refer to the daemon's host, which can differ
from the CLI host. `workdir` selects a container working directory.

Use `cpus`, `memory` (for example `256m`), `user`, and `readOnly` to configure a
container. Choose these limits and allowed mounts in trusted application code.
The adapter does not enforce a policy for untrusted repositories or supply the
Docker socket to build containers.

To collect files after a successful exit, own the container explicitly:

```ts
await using build = await docker.start(ctx, {
  image: 'alpine:3.22',
  command: ['sh', '-c', 'echo result > /result.txt']
});
await build.wait();
await build.copyFrom('/result.txt', absoluteArtifactPath);
```

`copyTo(hostPath, containerPath)` supplies files to a running container.
Both copy operations require absolute paths. Failed containers are removed, so
collect failure diagnostics through live logs rather than post-exit file copies.

## Environment values and recovery

Environment values use a temporary file with private permissions and are
removed after container creation. They are absent from CLI arguments and
Runling command previews. All nonempty environment values are masked in output,
including values split across chunks. Multiline values are rejected. Exact-value
masking cannot identify encoded or transformed secrets, or private data that was
not supplied as an environment value. Do not put credentials in command arguments,
image references, labels, or workflow inputs.

The Docker daemon stores container configuration, including environment values.
Use a trusted daemon. Grant only the credentials needed for a job and keep the
controller's Docker access and GitHub credentials outside build containers.

Containers and networks have the labels `dev.runling.managed=true`, `dev.runling.resource`, and
an optional `dev.runling.run`. Add application labels through each operation's
`labels` option. Select a Docker context with its `context` option;
the `dev.runling.*` prefix is reserved. A crash, lost daemon response, or failed
cleanup can leave a labelled resource. There is no automatic prune or restart
reconciliation. Never assume workflow state snapshots recover containers.

## Migration from the factory API

Replace the `createDocker` import with `docker`. Remove the factory call and
pass `ctx` as the first argument to `run`, `start`, and `network`. The task context
now supplies cancellation and the run label. Move factory `context` and `labels`
settings into each operation's options. The factory and public adapter class
are removed.

## Try the local pipeline

From the repository root:

```sh
mise x -- pnpm --dir packages/runling runling run examples/docker.ts --input '{}'
```

The example starts a service and runs two child checks concurrently. It then
removes the containers and their network. It needs no model credentials.
Run `mise x -- pnpm --dir packages/runling dev` to select **docker** in the web
console. Stop the server when finished.

Run daemon integration tests explicitly:

```sh
mise x -- pnpm --filter runling test:docker
```

Ordinary unit tests do not require Docker. Integration tests pull Alpine and
create resources labelled with a unique test run. They remove only those
resources and fail if a test leaked a container or network.

See [FDR-015](fdr/FDR-015-managed-docker-resources.md) for behavior and rationale.
