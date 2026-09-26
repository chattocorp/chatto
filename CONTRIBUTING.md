# Contributing

Chatto is not accepting outside contributions at this time. Report bugs in the
`#bug-reports` channel on the [Chatto HQ community
server](https://chat.chatto.run/); maintainers will create GitHub issues for
actionable reports. Other feedback and ideas are welcome there or by
[email](mailto:hendrik.mans@chattocorp.eu).

## Agentic Engineering

Chatto is intentionally developed with coding agents, and the tracked agent
workflow files in `.agents/`, `.claude/`, and `.conductor/` are part of how we
document and operate the project. They are public on purpose: they show the
coding conventions, review habits, maintenance workflows, and local workspace
setup we expect agents to follow.

If you explore the codebase, report an issue, or prepare a patch, we encourage
you to work agentically: give your agent the repository instructions, ask it to
read the relevant FDRs/ADRs/docs before changing behavior, and have it run the
narrowest meaningful checks for its change. Keep personal credentials,
machine-specific settings, and private prompts out of tracked files; use local
settings such as `.conductor/settings.local.toml` or your tool's user-level
configuration for those.

## Local Development Stack

The root pnpm workspace contains the JavaScript apps, examples, and libraries.
[`@chatto/client`](packages/chatto-client/README.md) provides shared request,
message, thread, reaction, and typing helpers for bot integrations.
The independent [Runling](packages/runling/README.md) workflow and agent
orchestrator lives in `packages/runling/` and is published to npm as `runling`.
It keeps its own version and MIT license. The Chatto bot uses this local package.
Use `mise check-runling`, `mise test-runling`, and `mise test-runling-package`
to verify it without running the complete Chatto test suite.

[ChattoBot](packages/chattobot/README.md) is a separate private workspace
package. Run `mise dev-chattobot` to start its realtime bot and Runling console.
Use `mise check-chattobot` and `mise test-chattobot` to verify it.

Root pnpm scripts use Turborepo to build workspace dependencies before their
consumers. Prefer `mise` tasks or root scripts such as `mise x -- pnpm run
check:frontend`; a command inside a package only runs that package's script.
Turbo caches library, frontend, and Runling builds. All Git worktrees share
the cache in the main checkout's `.turbo/cache`, so a new worktree restores
unchanged builds. Verification tasks run without Turbo caching. Remote caching
and telemetry are disabled by the repository configuration and scripts. See [ADR-102](docs/adr/ADR-102-turborepo-workspace-tasks.md)
for the task and cache boundaries.

Run Chatto, Authling, Mailpit, LiveKit, and the Runling bot:

```sh
mise trust
mise install
mise setup
mise dev
```

`mise dev` builds the frontend and a Chatto binary that includes it. Then it
runs the services in one supervised process group. Turbo restores unchanged
frontend builds from a cache that all worktrees share. Restart `mise dev` to
see a change in Chatto, its frontend, or Authling. `mise setup` installs
the Chatto and Authling dependencies and the LiveKit server. It does not build
anything.

All services use plain HTTP. In Conductor, `<workspace>` is the workspace name
and the base port is `$CONDUCTOR_PORT`. Outside Conductor, `<workspace>` is
`local` and the base port is `4000`:

| Service  | URL                                                |
| -------- | -------------------------------------------------- |
| Chatto   | `http://chatto.<workspace>.localhost:<base>`       |
| Authling | `http://authling.<workspace>.localhost:<base + 2>` |
| Runling  | `http://localhost:<base + 3>`                      |
| Mailpit  | `http://localhost:<base + 9>`                      |

Browsers resolve names beneath `.localhost` to your computer. You do not need
to change DNS or `/etc/hosts`. Each workspace has its own hostnames, so the
workspaces do not share browser cookies. The comment above the `dev` task in
`mise.toml` lists all ports.

For hot module replacement during frontend work, run this command in another
terminal:

```sh
mise dev-frontend
```

Vite then serves the frontend at `http://chatto.<workspace>.localhost:<base + 1>`
and sends API requests to the Chatto server of `mise dev`. To use a different
Chatto server, set `CHATTO_BACKEND_URL`, for example
`CHATTO_BACKEND_URL=https://dev.chatto.run mise dev-frontend`.

Create an Authling account, read its verification code in Mailpit, then choose
**Authling** on the Chatto login screen. Chatto asks for a username at first
login. The stack also creates Chatto owner `alice` and member `bob`; both use
the development-only password `foobar123`.

The stack starts the [Runling bot example](examples/runling-bot/README.md)
on loopback at the base port plus three (`http://localhost:4003` outside
Conductor). It uses the bootstrap TestBot account and receives the backend URL
and API key path automatically. On an empty server, bootstrap also creates
TestBot’s outbound webhook. Existing servers keep their saved configuration.

Chatto uses Authling as its development OIDC provider. Chatto stores embedded
NATS data in `cli/data/nats/` and search data in `cli/data/search/`. Authling
identity data is in `.context/dev/<workspace>/authling/`.

These credentials and accounts are for local development only. Stop `mise dev`
to stop the services. With the stack stopped, remove `cli/data/` to reset
Chatto, or remove the Authling identity directory to reset Authling. A new
Conductor workspace name also creates a new Authling issuer and state
directory.

If a worktree has NATS data in the former `cli/data/jetstream/` location, use
the migration steps in [Local Chatto Data](#local-chatto-data).

### Generate Test Data

With `mise dev` running, run this command in another terminal:

```sh
mise seed -- --seed 42 --users 20 --rooms 5 --messages 200 --thread-replies 40
```

This adds 20 users with generated names and 200 messages across five rooms.
The message total includes 40 thread replies. Users join different rooms, with
joins and some leaves interleaved with messages. Sign in as `alice` to browse
them; generated users have no password.

The same seed and counts reproduce content with the same generator version;
IDs and timestamps change. Each run adds data. A failed run can leave partial
data. Add `--json` to get the generated IDs and text, or `--help` for options.
Seeding is available only in development and test builds.

## Local Development with Conductor

[Conductor](https://conductor.build) runs the regular root `mise dev` stack as
native processes. Start the default **Dev stack** run mode to build and launch
Chatto, Authling, the Runling bot, Mailpit, and LiveKit on the workspace's ten
allocated ports. Chatto is at `http://chatto.<workspace>.localhost:<port>`.
The Open button lists this URL and the other service URLs. The
[Local Development Stack](#local-development-stack) section describes
the complete layout. Restart the stack after you change Chatto, its frontend,
or Authling. For hot module replacement, also start the **Vite frontend** run
mode.

The repository-level Conductor settings are shared in
`.conductor/settings.toml`, while the root `mise.toml` defines the native
development stack. Workspace-named `.localhost` hostnames and Conductor's
allocated ports isolate concurrent workspaces. Put
machine-specific Conductor overrides in `.conductor/settings.local.toml`; that
file is gitignored and wins over shared settings on your machine. Conductor
reads `.worktreeinclude` to copy gitignored local environment files, such as
`.env` and `.env.*`, into new workspaces. Stopping the run command stops all
child processes.

## Local Development with Codex

The Codex desktop environment is in `.codex/environments/environment.toml`.
Select **Chatto** in the app's local environment settings. New worktrees use
the same setup commands as Conductor. The **Dev stack**, **Storybook**, and
**Docs website** actions run the corresponding `mise` tasks in the integrated
terminal. The cleanup script stops workspace processes before Codex deletes
the worktree.

Start an action, then open its URL in the app's browser. With the default
local settings, Chatto uses `http://chatto.local.localhost:4000` and Authling
uses `http://authling.local.localhost:4002`. Storybook and the docs website
show their URLs in the terminal.

This configuration does not allocate ports or hostnames for each worktree.
The development stack uses base port `4000` and workspace name `local` outside
Conductor. Run one such stack at a time, or set distinct `CONDUCTOR_PORT` and
`CONDUCTOR_WORKSPACE_NAME` values for each workspace before starting its
actions. These variables are the existing `mise` inputs for port and hostname
isolation. Conductor's preview URL list, `.worktreeinclude` handling, Git
settings, and PR prompt are not part of the Codex environment configuration.

## Developing Outside of Conductor

Use `mise` for local tool versions and tasks:

```sh
mise trust
mise run setup
```

To run the regular development stack outside Conductor after the setup
described in [Local Development Stack](#local-development-stack):

```sh
mise dev
```

To run the Vite frontend development server with hot module replacement:

```sh
mise dev-frontend
```

To run the docs website development server:

```sh
mise dev-docs-website
```

To run only the bundled Chatto executable, without Authling and the other
services:

```sh
mise run chatto run
```

To check SPDX/REUSE license metadata:

```sh
mise license-check
```

To format changed JavaScript or TypeScript files in the root pnpm workspace,
pass their paths to Prettier. Use the check command to verify the result:

```sh
mise x -- pnpm format -- path/to/file.ts
mise x -- pnpm format:check -- path/to/file.ts
```

Prettier uses the nearest configuration file. The frontend keeps its Svelte and
Tailwind plugin settings in `apps/frontend/.prettierrc`. Authling uses its own
pnpm workspace and toolchain.

`mise dev` uses Conductor's allocated port block and falls back to base port
`4000` outside Conductor. `mise dev-frontend` uses the base port plus one.
Storybook starts at port `6006` and the docs website at port `4321`. Both
select the next free port when another workspace uses it. `mise run chatto run`
uses the same Chatto, embedded NATS, LiveKit, and Mailpit ports as `mise dev`.
Pass explicit CLI arguments after the task name, for example
`mise chatto version`.

### Local Chatto Data

The local `cli/chatto.toml` file is the source for local data paths. It keeps
embedded NATS data in `cli/data/nats/` and the search index in
`cli/data/search/`. The `mise dev` and `mise chatto` tasks use these paths.

Older worktrees can have embedded NATS data in `cli/data/jetstream/`. Stop all
Chatto processes before you migrate this data. If
`cli/data/nats/jetstream/` does not exist, preserve the old store and copy it
to the new location:

```sh
mkdir -p cli/data/nats
cp -a cli/data/jetstream cli/data/nats/
```

Start Chatto and check the data before you remove the old
`cli/data/jetstream/` directory. If both JetStream directories already contain
data, do not merge them. Preserve both directories and select the store that
contains the data that you need.

## Local Bootstrap Users

Local development instances are bootstrapped from `cli/chatto.toml` when the server is otherwise empty.

| Login   | Email               | Password    | Role  |
| ------- | ------------------- | ----------- | ----- |
| `alice` | `alice@example.com` | `foobar123` | owner |
| `bob`   | `bob@example.com`   | `foobar123` | user  |

Use `alice` when you need server administration access.

## Synthetic Test Data

See [Generate Test Data](#generate-test-data) to populate a running
development server. For e2e setup, use the shared helpers:

```ts
import { seedData, loginSeededUser } from './fixtures/seed';
import * as routes from './routes';

const scene = await seedData(page.request, {
  seed: 42,
  users: 3,
  rooms: 2,
  messages: 20,
  threadReplies: 5
});
await loginSeededUser(page.request, { id: scene.rooms[0].memberIds[0] });
await page.goto(routes.room(scene.rooms[0].id));
```

The helper returns when the generated data is ready to read. Each room’s
`memberIds` lists its final generated members. Use the returned IDs and text
in assertions. Create the session before loading the app, with a fresh
browser context for each viewer. These HTTP helpers require a `test_endpoints`
build; release builds exclude them.

Seed the starting state, then perform the action under test in the browser.
For live-delivery tests, connect the receiver before sending the new message.
Keep the normal login flow when testing authentication. The performance fixture
retains its separate, fixed workload.

## E2E Shards

CI runs the non-media e2e tests on four runners. Each runner uses four workers.
The shard script collects the current suite, sorts tests by file and source
line, then assigns consecutive tests to different runners. This spreads large
groups of slow tests across the runners without a stored timing database.
Tests must run independently; do not use this split for serial suites.

To run the first CI shard locally, start in `apps/frontend/`:

```sh
mise x -- node scripts/run-e2e-shard.mjs 1/4 --grep-invert @ffmpeg
```

Add `--list` to inspect the selection without starting test servers. The media
and performance suites keep their separate CI jobs. CI uploads an
`e2e-timings-N-of-4` JSON artifact for each shard, including successful runs.
Compare the slowest test step and complete job across repeated runs; summed
test durations alone do not measure CI wall time.

The inactive-server notification tests advance the browser clock past the
background poll interval. They wait for real projection catch-up and socket
closure before and after the advance. Keep server requests and message delivery
real when adding tests that control browser time.
