# Contributing

Chatto is not accepting outside contributions at this time, but feedback, bug reports, and ideas are welcome by [email](mailto:hendrik@mans.de).

## Local Development with Conductor

[Conductor](https://conductor.build) workspaces build and run the bundled Chatto executable. The `run` script in `.conductor/settings.toml` wires Conductor's assigned `$CONDUCTOR_PORT` and the next port into the env vars read by the executable:

| Port              | Process                            |
| ----------------- | ---------------------------------- |
| `$CONDUCTOR_PORT` | Chatto webserver (user-facing URL) |
| `+1`              | Embedded NATS                      |
| `+2`              | Prometheus metrics                 |

The repository-level Conductor settings are shared in `.conductor/settings.toml`. The run command delegates to `mise run chatto`, which builds the frontend and development CLI, wires the per-workspace ports, and starts `bin/chatto run` without live reloads. Put machine-specific overrides in `.conductor/settings.local.toml`; that file is gitignored and wins over shared settings on your machine. Conductor also reads `.worktreeinclude` to copy gitignored local environment files, such as `.env` and `.env.*`, into new workspaces.

## Developing Outside of Conductor

Use `mise` for local tool versions and tasks:

```sh
mise trust
mise run setup
```

To run the bundled executable without live reloads using the same port wiring as Conductor:

```sh
mise run chatto
```

When `CONDUCTOR_PORT` is unset, `mise run chatto` uses `4000` for Chatto, `4001` for embedded NATS, and `4002` for Prometheus metrics. Pass explicit CLI arguments after the task name, for example `mise chatto version`.

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
