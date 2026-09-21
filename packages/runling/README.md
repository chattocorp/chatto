# Runling

Run and orchestrate TypeScript-based worklows of any size and kind.

## Features

- Runs single workflows with a nice TUI visualization and/or logging
- Listens to webhooks (and other triggers) to execute workflows
- Workflows are simple functions, optionally decorated with input/output schemas
- Embeds the Pi SDK for easy peasy agent/LLM integration
- Automatic monitoring of token usage and cost

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

Use the console to start a run or send a request:

```sh
curl http://localhost:5173/api/webhooks/echo \
  -H 'content-type: application/json' -d '"hello"'
```

And off it goes!

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
