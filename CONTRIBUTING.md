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

## Local Development with Conductor or Paseo

[Conductor](https://conductor.build) and [Paseo](https://paseo.sh) run the
complete root [`compose.yml`](compose.yml) stack through OrbStack. Start
Conductor's default Compose run mode or Paseo's `compose` script to build and
launch Chatto, Authling, Mailpit, LiveKit, Storybook, and the docs website with
workspace-specific `*.orb.local` domains. The checkout directory is the
Compose project name; for a worktree named `<workspace>`, Chatto is available
at `https://chatto.<workspace>.orb.local`. The other origins follow the names
listed in the [Complete Local Stack](README.md#complete-local-stack) section.
Use Conductor's **Compose (watch)** run mode, Paseo's `compose-watch` script, or
`docker compose up --build --watch` when dependency-manifest and development-
image changes should restart or rebuild the affected services automatically.
Ordinary Chatto, Authling, and Storybook source changes live-reload in either
mode through the bind-mounted checkout.

The repository-level Conductor settings are shared in
`.conductor/settings.toml`, and the repository-level Paseo settings are shared
in `paseo.json`. Both isolate concurrent workspaces. Put machine-specific
Conductor overrides in `.conductor/settings.local.toml`; that file is
gitignored and wins over shared settings on your machine. Both tools read
`.worktreeinclude` to copy gitignored local environment files, such as `.env`
and `.env.*`, into new workspaces. Archiving a Paseo worktree runs
`docker compose down -v` to remove its containers, network, and development
volumes.

## Developing Outside of Conductor

Use `mise` for local tool versions and tasks:

```sh
mise trust
mise run setup
```

To run the live backend and frontend development stack outside Conductor:

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

When both `CONDUCTOR_PORT` and `PASEO_PORT` are unset, `mise dev` uses `4000` for the Vite frontend, `4001` for the Chatto backend, `4002` for embedded NATS, `4003` for Prometheus metrics, and `4004` for exporter metrics. `mise dev-docs-website` uses `4000` for the docs website. `mise run chatto run` still uses the bundled-binary port layout: `4000` for Chatto, `4001` for embedded NATS, `4002` for Prometheus metrics, and `4003` for exporter metrics. Pass explicit CLI arguments after the task name, for example `mise chatto version`.

## Local Bootstrap Users

Local development instances are bootstrapped from `cli/chatto.toml` when the server is otherwise empty.

| Login   | Email               | Password    | Role  |
| ------- | ------------------- | ----------- | ----- |
| `alice` | `alice@example.com` | `foobar123` | owner |
| `bob`   | `bob@example.com`   | `foobar123` | user  |

Use `alice` when you need server administration access.
