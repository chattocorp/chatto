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

[Conductor](https://conductor.build) and Paseo workspaces build and run the bundled Chatto executable. The Conductor `run` script in `.conductor/settings.toml` and the Paseo service script in `paseo.json` both delegate to `mise run chatto run`. The mise task uses Conductor's assigned `$CONDUCTOR_PORT`, Paseo's assigned `$PASEO_PORT`, or `4000` outside either tool, then reserves the next ports for bundled services:

| Port                             | Process                            |
| -------------------------------- | ---------------------------------- |
| `$CONDUCTOR_PORT` / `$PASEO_PORT` | Chatto webserver (user-facing URL) |
| `+1`                             | Embedded NATS                      |
| `+2`                             | Prometheus metrics                 |
| `+3`                             | Deployment-wide exporter metrics   |

The repository-level Conductor settings are shared in `.conductor/settings.toml`, and the repository-level Paseo settings are shared in `paseo.json`. Both build the frontend and development CLI, wire per-workspace ports, and start `bin/chatto run` without live reloads. Put machine-specific Conductor overrides in `.conductor/settings.local.toml`; that file is gitignored and wins over shared settings on your machine. Conductor also reads `.worktreeinclude` to copy gitignored local environment files, such as `.env` and `.env.*`, into new workspaces.

## Developing Outside of Conductor

Use `mise` for local tool versions and tasks:

```sh
mise trust
mise run setup
```

To run the bundled executable without live reloads using the same port wiring as Conductor:

```sh
mise run chatto run
```

When both `CONDUCTOR_PORT` and `PASEO_PORT` are unset, `mise run chatto run` uses `4000` for Chatto, `4001` for embedded NATS, `4002` for Prometheus metrics, and `4003` for exporter metrics. Pass explicit CLI arguments after the task name, for example `mise chatto version`.

For the live-reload development stack, use Tilt:

```sh
mise run dev
```

The Tilt stack uses Vite on port `5173`, the Go backend on port `4000`, embedded NATS on port `4555`, and Prometheus metrics on `http://localhost:9090/metrics`.

## Local Bootstrap Users

Local development instances are bootstrapped from `cli/chatto.toml` when the server is otherwise empty.

| Login   | Email               | Password    | Role  |
| ------- | ------------------- | ----------- | ----- |
| `alice` | `alice@example.com` | `foobar123` | owner |
| `bob`   | `bob@example.com`   | `foobar123` | user  |

Use `alice` when you need server administration access.
