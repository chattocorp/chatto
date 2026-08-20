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

## Local Development with Conductor

[Conductor](https://conductor.build) runs the complete root
[`pitchfork.toml`](pitchfork.toml) stack as native processes. Start the default
**Dev stack** run mode to launch Chatto, Authling, Mailpit, LiveKit, Storybook,
and the docs website with workspace-specific `*.localhost:42443` HTTPS origins.
For a Conductor workspace named `<workspace>`, Chatto is available at
`https://chatto-<workspace>.localhost:42443`; the other origins follow the
names listed in the [Complete Local Stack](README.md#complete-local-stack)
section. Vite, Astro, and Storybook live-reload their sources. Pitchfork
rebuilds and restarts the Go services after relevant source changes.

The repository-level Conductor settings are shared in
`.conductor/settings.toml`, while the root `pitchfork.toml` defines the native
development stack. Together they isolate concurrent workspaces. Put
machine-specific Conductor overrides in `.conductor/settings.local.toml`; that
file is gitignored and wins over shared settings on your machine. Conductor
reads `.worktreeinclude` to copy gitignored local environment files, such as
`.env` and `.env.*`, into new workspaces. Archiving a workspace stops its
Pitchfork daemons and removes its global proxy registrations.

## Developing Outside of Conductor

Use `mise` for local tool versions and tasks:

```sh
mise trust
mise run setup
```

To run the complete Pitchfork development stack outside Conductor after the
setup described in the README:

```sh
mise dev
```

To run the docs website development server on the workspace base port:

```sh
mise dev-docs-website
```

To run the bundled executable without live reloads:

```sh
mise run chatto run
```

To check SPDX/REUSE license metadata:

```sh
mise license-check
```

`mise dev` uses Pitchfork's workspace-specific native ports and exposes the
public services through HTTPS on port `42443`. `mise dev-docs-website` still
uses `4000` when `CONDUCTOR_PORT` and `CHATTO_DOCS_WEBSITE_PORT` are unset.
`mise run chatto run` uses the
bundled-binary port layout: `4000` for Chatto, `4001` for embedded NATS,
`4002` for Prometheus metrics, and `4003` for exporter metrics. Pass explicit
CLI arguments after the task name, for example `mise chatto version`.

## Local Bootstrap Users

Local development instances are bootstrapped from `cli/chatto.toml` when the server is otherwise empty.

| Login   | Email               | Password    | Role  |
| ------- | ------------------- | ----------- | ----- |
| `alice` | `alice@example.com` | `foobar123` | owner |
| `bob`   | `bob@example.com`   | `foobar123` | user  |

Use `alice` when you need server administration access.
