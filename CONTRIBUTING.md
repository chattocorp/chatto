# Contributing

Chatto is not accepting outside contributions at this time, but feedback, bug reports, and ideas are welcome by [email](mailto:hendrik@mans.de).

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

[Conductor](https://conductor.build) and Paseo workspaces run the live local development stack. The default Conductor `chatto` run script in `.conductor/settings.toml` and the Paseo `dev` service in `paseo.json` both delegate to `mise dev`. The mise task uses Conductor's assigned `$CONDUCTOR_PORT`, Paseo's assigned `$PASEO_PORT`, or `4000` outside either tool, then reserves the next ports for bundled services:

| Port                              | Process                                       |
| --------------------------------- | --------------------------------------------- |
| `$CONDUCTOR_PORT` / `$PASEO_PORT` | Vite frontend (user-facing URL)               |
| `+1`                              | Chatto backend webserver                      |
| `+2`                              | Embedded NATS                                 |
| `+3`                              | Prometheus metrics                            |
| `+4`                              | Deployment-wide exporter metrics              |

The repository-level Conductor settings are shared in `.conductor/settings.toml`, and the repository-level Paseo settings are shared in `paseo.json`. Both wire per-workspace ports before starting the backend and frontend development servers so multiple workspaces can run side by side. Conductor also exposes a `docs-website` run script, and Paseo exposes a separate `dev-docs-website` service; both are backed by `mise dev-docs-website` and reuse the workspace base port for the docs website. Put machine-specific Conductor overrides in `.conductor/settings.local.toml`; that file is gitignored and wins over shared settings on your machine. Conductor also reads `.worktreeinclude` to copy gitignored local environment files, such as `.env` and `.env.*`, into new workspaces.

## Developing Outside of Conductor

Use `mise` for local tool versions and tasks:

```sh
mise trust
mise run setup
```

To run the same live development stack Conductor and Paseo use:

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

When both `CONDUCTOR_PORT` and `PASEO_PORT` are unset, `mise dev` uses `4000` for the Vite frontend, `4001` for the Chatto backend, `4002` for embedded NATS, `4003` for Prometheus metrics, and `4004` for exporter metrics. `mise dev-docs-website` uses `4000` for the docs website. `mise run chatto run` still uses the bundled-binary port layout: `4000` for Chatto, `4001` for embedded NATS, `4002` for Prometheus metrics, and `4003` for exporter metrics. Pass explicit CLI arguments after the task name, for example `mise chatto version`.

## Local Bootstrap Users

Local development instances are bootstrapped from `cli/chatto.toml` when the server is otherwise empty.

| Login   | Email               | Password    | Role  |
| ------- | ------------------- | ----------- | ----- |
| `alice` | `alice@example.com` | `foobar123` | owner |
| `bob`   | `bob@example.com`   | `foobar123` | user  |

Use `alice` when you need server administration access.
