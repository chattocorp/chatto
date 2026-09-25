# Runling

Run and orchestrate TypeScript workflows of any size and kind.

See the [documentation index](docs/README.md) for API guides, architecture
decisions (ADRs), and feature records (FDRs).

## Tasks, runs, and messages

A **task** is a function that takes a context and input and returns a result.
It can exchange messages while it runs. A workflow is a task that coordinates
other tasks; an agent is one way to implement a task.

Call a task directly when you want to await its result. Use `ctx.spawn(ctx => work(ctx, input))`
for concurrent work. The returned **run** has `send()`, `output`, `result`, and
`cancel()`. Inside the child, use `ctx.inbox`, `ctx.emit()`, and `ctx.signal`.

```ts
const run = ctx.spawn((ctx: WorkflowContext<string, string>) =>
  investigate(ctx, { question }));
try {
  await run.send("Also check private rooms");
  for await (const message of run.output) {
    // Handle progress or findings while the child works.
  }
  return await run.result;
} finally {
  await run[Symbol.asyncDispose]();
}
```

Messages are buffered and ordered. Sending accepts a message into the inbox;
it does not confirm that the child acted on it. Disposal cancels unfinished work
and awaits cooperative cleanup. See [task channels](docs/task-channels.md) for
limits, ownership, and compatibility with the standalone `spawn` function.

## Features

- Runs single workflows with a nice TUI visualization and/or logging
- Listens to webhooks (and other triggers) to execute workflows
- Workflows are simple functions, optionally decorated with input/output schemas
- Embeds the Pi SDK for easy peasy agent/LLM integration
- Automatic monitoring of token usage and cost

## Run references

Each run started by `runling serve` has a readable reference such as
`brave-otters-4821`. The console shows it in the run list and provides a copy
button in the run details. Use it when discussing a run or searching its logs:

```sh
rg -l '"reference":"brave-otters-4821"' .runling/runs
```

References are unique within one journal directory and survive server restarts.
Journal filenames and run URLs still use UUIDs. Older runs without a reference
show their UUID in the details instead.

## Non-Features

Runling is defined more through what it does _not_ do. Here's some stuff that's not in and also not planned:

- User accounts/authentication (put it behind a reverse proxy instead)
- Coordinating multiple replicas through a datastore (Runling is small and simple)
- Visual editing of workflows (it's just JS/TS; your agent loves it!)
- 3D graphics (what?!)

## Getting Started

Add the `runling` package to your project:

```sh
pnpm add runling zod
```

Add `"runling": "runling"` to the `scripts` in your `package.json`.

Create `workflows/echo.ts`:

```ts
import { task } from "runling";
import { z } from "zod";

export default task(
  { name: "Echo", input: z.string(), output: z.string() },
  (ctx, input) => input,
);
```

Run it with `pnpm runling run workflows/echo.ts "hello"`.

Much more exciting though is Runling's ability to spin up a long-running process that will automatically execute workflows in response to webhooks being sent to it.

Create `runling.config.ts` in the project root:

```ts
import { defineWebConfig, startWorkflow } from "runling/web";
import echo from "./workflows/echo.ts";

export default defineWebConfig({
  webhooks: {
    echo: startWorkflow(echo)
  }
});
```

Run `pnpm runling serve`, then open `http://localhost:5173`.

Configuration and source code load once by default. Restart the server to load
changes, or use `runling serve --watch` to enable project file watching and reload.
Server shutdown marks active runs interrupted. Runs are process-local: restart
preserves their history, but does not restore workflows, state, or agent sessions.
New inputs start new runs.

Select a run to read its recorded log lines and task milestones. The **Log** view opens by default
and follows new lines. Scroll up to pause following, then select **Follow latest**
to return to the bottom. Large logs first show the latest 500 lines; select
**Show earlier logs** to read more. Use the **Timeline** tab to inspect task
timing. On wide screens, the log and details appear side by side. Select a task
reference or agent name in the log to show its details on the right. Close those
details to see the run input, result, and token usage. On smaller screens, open
**Run details** for that data. Long log lines show up to four rows. Select **More** or **Less**
to expand or collapse a line.
For a running run with active work, the console shows how long it has been
since the last recorded event once that gap reaches two minutes.

The terminal shows compact, colored diagnostics and a single ready line with
the console URL. Set `NO_COLOR` to disable colors in server and workflow logs. Structured server
records remain in `.runling/logs/server.jsonl` beside the configuration file.

Use the console to start a run or send a request:

```sh
curl http://localhost:5173/api/webhooks/echo \
  -H 'content-type: application/json' -d '"hello"'
```

And off it goes!

Webhooks are optional. For outbound connections and other long-running inputs,
configure named `sources`. Each source receives cancellation, retained
process-local state, and `dispatch(router, input)`. Sources start before any
HTTP request. See [event sources](docs/event-sources.md) for lifecycle rules.

Run `pnpm runling --help` to list commands. Use `run --help` or
`serve --help` to see command options, and `--version` to print the version.

## Aborting a workflow

Tasks receive a workflow context as their first argument. Call
`ctx.abort("Cannot continue")` to stop the run and report it as failed.

## Task schemas

Tasks accept [Standard Schema](https://standardschema.dev/schema) validators
such as Zod and Valibot. They validate input and output at runtime, use parsed
values, and return a Promise. Existing TypeBox schemas remain supported.

Webhooks also require Standard JSON Schema export. Zod supports this directly;
for Valibot, wrap schemas with `toStandardJsonSchema` from
`@valibot/to-json-schema`.

## Monorepo development and releases

Runling is an independent package in the root pnpm workspace. Run these
commands from the repository root:

```sh
mise setup-frontend
mise check-runling
mise test-runling
mise test-runling-package
```

The package test builds the runtime and web console, installs a tarball in a
temporary consumer project, and checks the CLI, public types, webhooks, live
events, and web assets. It contacts the npm registry to install dependencies.
The test does not need agent-provider credentials.

Use `mise build-runling` after source changes before running a workspace
consumer. `mise dev` builds Runling before it starts the Chatto bot.

To run the bundled demos, copy `.env.example` to `.env` inside
`packages/runling/`, set the values for the demo, and run
`mise x -- pnpm --dir packages/runling dev` from the monorepo root.
The planning and coordinator demos require `CHATTO_WORKING_COPY` to name an
absolute path to a dedicated Chatto checkout. Restart the server after changing
`.env`. See the [planning demo](examples/chatto-plan-demo.md) for checkout rules.

Runling has its own entry in the root release-please configuration and
manifest. Its imported version is 0.7.0, from upstream commit
`2fe061bc9904d4a5cb2c0db3973dbe0fbe337f1c`. The existing changelog records
releases made in the former repository. New releases use tags such as
`runling/v0.8.0`. Chatto releases exclude Runling package changes.

After CI succeeds, the root release workflow uses `RELEASE_PLEASE_PAT` to
create release PRs and tags. A Runling tag starts `publish-runling.yml`, which
checks the version, rejects versions previously published to npm, and tests
the package. A separate job publishes that exact tarball with npm provenance.
Manual workflow runs only test and save the tarball.

Before the first release from this repository, configure the `runling` npm
package's [trusted publisher](https://docs.npmjs.com/trusted-publishers/) with
owner `chattocorp`, repository `chatto`, workflow `publish-runling.yml`, and
environment `npm`. Enable publishing for that trust relationship and configure
the GitHub `npm` environment's protection rules. The former repository's
publisher configuration does not authorize this repository. Disable its
release workflow when this migration is merged so only one repository owns
new Runling releases.

## License

MIT. See [LICENSE](LICENSE).
