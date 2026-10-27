# Runling

Run and orchestrate TypeScript workflows of any size and kind.

See the [documentation index](docs/README.md) for API guides, architecture
decisions (ADRs), and feature records (FDRs).

The console's Log view shows one updating row per activity. Expand a row for
recorded details, or enable **Detailed log** for the chronological diagnostic
lines. Logs and timeline activity remain visible across workflow reactivation.

## Tasks, runs, and messages

A **task** is a function with schemas. It has a name, an input schema, and an
result schema. It can also have a `steering` schema for messages from its parent
and an `output` schema for messages to its parent. If a task has no message
schema, it accepts any message. A task always returns a promise.

```ts
import { task, Type } from 'runling';

const greet = task(
  { name: 'Greet', input: Type.String(), result: Type.String() },
  async (_ctx, name) => `Hello, ${name}`
);
```

A **workflow run** is one invocation of a top-level task. The host is the
parent of the top-level task. The host is `runling run`, the web console, or a
[router](docs/webhook-routing.md). The result of the run is the output of the
task. Runling does not change the shape of the output.

A task can invoke other tasks in two ways:

- A direct call, `await greet(ctx, 'Ada')`, starts the task and waits for its
  result. The child cannot receive steering. Runling records its streamed output in the
  journal and then discards it.
- `ctx.start(task, input)` starts the task and returns an invocation handle.
  Use `steer()` and `closeSteering()` to send messages to the child, `output` to
  read its messages, `result` to get its final value, and `cancel()` to stop it.

Inside a task, read messages from the parent with `ctx.steering` and send messages
to the parent with `ctx.emit()`. `ctx.signal` tells the task when to stop.

```ts
const counter = task(
  {
    name: 'Counter',
    input: Type.Number(),
    result: Type.Number(),
    steering: Type.Number(),
    output: Type.Number()
  },
  async (ctx, total) => {
    for await (const amount of ctx.steering) await ctx.emit((total += amount));
    return total;
  }
);

// In the parent task:
await using child = ctx.start(counter, 0);
await child.steer(5);
child.closeSteering();
for await (const total of child.output) log.info(`Total: ${total}`);
return await child.result;
```

Messages are buffered and in order. `steer()` puts a message in the queue. It
does not tell you that the child used the message. Disposal cancels unfinished
work and waits for cleanup. See [task invocations and messages](docs/task-channels.md)
for the complete contract.

Use `step(label, work)` to show a named part of a task in the log and on the
timeline. A step has no schemas and no messages.

## Messages from the host

The host can send messages to a running workflow, and it shows the messages
that the top-level task emits:

- `runling run` on an interactive terminal: each line that you type is a
  steering message to the top-level task. The journal records its streamed output;
  the terminal prints only the final root result after the live run details.
- Web console: a running run has a message composer below its log. Text that
  is valid JSON is sent as a JSON value. Other text is sent as a string.
- HTTP: `POST /api/runs/:id/messages` with the body `{ "message": <json> }`.
- Routers: keep the handle that `ctx.start` returns and call `steer()` later.

The message must agree with the `steering` schema of the top-level task. When a
task waits for a message, the console shows "Waiting for a message". See
[workflow messages](docs/workflow-messages.md).

## Features

- Runs single workflows from the terminal and prints their final output
- Listens to webhooks (and other triggers) to execute workflows
- Workflows are TypeScript functions with input and result schemas
- Embeds the Pi SDK for easy peasy agent/LLM integration
- Automatic monitoring of token usage and cost

## Run references

Each run started by `runling serve` or `runling run` has a readable reference such as
`brave-otters-4821`. The console shows it in the run list and provides a copy
button in the run details. Use it when discussing a run or searching its logs:

```sh
rg -l '"reference":"brave-otters-4821"' .runling/runs
```

References from `runling serve` are unique within one journal directory and survive
server restarts. `runling run` picks a random reference without checking the
directory.
Journal filenames and run URLs still use UUIDs. Older runs without a reference
show their UUID in the details instead.

A workflow reads its run's ID and reference from `ctx.run`, for example to tell
a user which run to report. Child tasks share the run of their parent.
`ctx.run` is absent in a context from `createTaskContext()` and in
`runWorkflow` without its `run` option.

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
import { task } from 'runling';
import { z } from 'zod';

export default task({ name: 'Echo', input: z.string(), result: z.string() }, (ctx, input) => input);
```

Run it with `pnpm runling run workflows/echo.ts "hello"`. The command shows the
run reference, task progress, tool activity, token usage, duration, and journal
path. It then prints the root task's final output once, without a log prefix or
task indentation. Intermediate replies and updates stay in the run journal.
The [CLI output contract](docs/fdr/FDR-011-cli-output.md) defines the display
requirements and their rationale.
Use `--verbose` to show debug logs. On a terminal, the command shows a
string as Markdown. It shows other values as JSON. Use `--json` to print one
JSON document with the output, the error, the duration, and the token usage;
run details go to stderr. Use
`--input <json>` to give a JSON input instead of a prompt string.

On an interactive terminal, each line that you type is a message to the
top-level task. The first Ctrl-C cancels the workflow and lets its cleanup run.
A second Ctrl-C exits at once.

Each run writes its journal to `.runling/runs/<id>.jsonl` in the working
directory, through the same code as `runling serve`. A server started in that directory shows these
runs in its history. It reads them when it starts, and it does not mark a run
interrupted while the process that writes it is still alive.

Much more exciting though is Runling's ability to spin up a long-running process that will automatically execute workflows in response to webhooks being sent to it.

Create `runling.config.ts` in the project root:

```ts
import { defineWebConfig, startWorkflow } from 'runling/web';
import echo from './workflows/echo.ts';

export default defineWebConfig({
  webhooks: {
    echo: startWorkflow(echo)
  }
});
```

Run `pnpm runling serve`, then open `http://localhost:5173`.

Configuration and source code load once by default. Restart the server to load
changes, or use `runling serve --watch` to enable project file watching and reload.
Server shutdown marks active executions interrupted and saves root-task state.
The server also snapshots state every 60 seconds. Register recovery-aware roots
in the `workflows` configuration map to re-enter them after restart with their
saved `ctx.state`. Roots own recovery of children, connections, and agent sessions;
message queues are not restored. See [workflow state](docs/workflow-state.md).

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

Tasks receive a task context as their first argument. Call
`ctx.abort("Cannot continue")` to stop the run and report it as failed.

## Task schemas

Each task needs an input schema and a result schema. The message schemas
`steering` and `output` are optional. Tasks accept
[Standard Schema](https://standardschema.dev/schema) validators, such as Zod and
Valibot, and TypeBox schemas from `Type`. Runling validates the input, the
result, and each stream item at runtime and gives the task the parsed values.

Webhooks and agent tools need a JSON Schema of the input. Zod and TypeBox
support this directly. For Valibot, wrap schemas with `toStandardJsonSchema` from
`@valibot/to-json-schema`. [`taskTool`](docs/agents.md#tasks-as-agent-tools)
needs a TypeBox input schema.

## Agents

`agent()` defines a task that owns an agent session per invocation. It accepts
a prompt, returns the final text, receives steering messages, and emits completed
replies. It can be a CLI workflow, a child task, or an agent tool through
`taskTool()`. Use `createAgentSession()` for a custom conversation loop and
`runAgent()` for one explicit interaction with an outcome report.
Start ordinary [child tasks](docs/task-channels.md#background-children) without
waiting immediately when the parent must continue. `createClassifier` provides
one isolated turn with a validated JSON result. Runling owns private temporary
storage and transcript mechanics: use `agentSessionFile` for run-owned sessions
or `AgentSessionStore` when a session must continue across runs.
See [agents](docs/agents.md), including the
[migration from session-returning `agent()`](docs/agents.md#migration-from-session-returning-agent).

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

Workspace consumers import Runling's TypeScript source. Use a TypeScript loader
such as tsx, Vite, or jiti; the CLI installs its loader automatically. No Runling
build or special export condition is required. Workspace `serve` uses Vite for
the console and preserves the application's working directory.

Builds remain necessary for publication. Pack with pnpm so `publishConfig`
replaces source exports with compiled JavaScript and declarations. Published
installs serve the compiled console without Vite. See
[ADR-013](docs/adr/ADR-013-source-workspace-exports.md).

To run the bundled demos, run `mise x -- pnpm --dir packages/runling dev` from
the monorepo root. Demos that use agents need model-provider credentials in Pi
or in `packages/runling/.env`. Restart the server after changing `.env`.

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
