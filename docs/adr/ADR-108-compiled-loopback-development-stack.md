# ADR-108: Run a Compiled Development Stack on Loopback Hostnames

**Date:** 2026-09-26

## Status

Accepted. Supersedes [ADR-078](ADR-078-portless-native-development-stack.md).

> **Amended 2026-10-05:** mise runs each service in its own process group and
> signals all groups on `SIGHUP`, `SIGINT`, or `SIGTERM`. `mise dev` therefore
> runs the services directly as parallel tasks, and the
> `tools/dev-supervisor.sh` wrapper is removed. The stack does not stop
> processes that it did not start: `mise stop`, the stop step before startup,
> and the archive cleanup are removed. Before startup, `mise dev` runs
> `tools/check-dev-ports.sh`. If a port is in use, the check lists the process
> and fails the start.

> **Amended 2026-10-05:** `mise dev` runs only Chatto. `mise dev-full` runs the
> complete stack that this record describes. `cli/chatto.toml` is the shared
> development configuration, with email and LiveKit disabled. The `chatto`
> task adds the values that depend on the workspace. `mise dev-full` enables
> email and LiveKit and adds the Authling login provider and TestBot through
> environment values. Most work needs only Chatto, so the default command
> builds and starts less.

## Context

ADR-078 ran the Chatto backend and a Vite development server as separate
processes. Portless gave each browser-facing process an HTTPS route on a shared
proxy port. When Vite or the backend restarted, the Portless route often
stopped working. A developer then had to restart the complete stack. This
removed the advantage of live reload.

A production frontend build takes approximately 8 seconds. An incremental Go
build and a server start add a few more seconds. Most changes do not need hot
module replacement. Frontend-only work sometimes needs Vite, for example to
test the frontend against a remote Chatto server.

Chatto's CIMD client identifier needed HTTPS because Authling accepts only
HTTPS CIMD client identifiers. Authling also accepts conventional, operator-
configured clients with plain-HTTP loopback redirect URIs when its own issuer
is loopback.

Browsers do not isolate cookies by port. If all workspaces use `localhost`,
their Chatto CSRF cookies, Chatto browser-session cookies, and Authling session
cookies share one cookie scope. Browsers, Go, and the macOS resolver resolve
names beneath `.localhost` to loopback without DNS or `/etc/hosts` changes.

## Decision

`mise dev` builds the embedded frontend and the bootstrap-enabled Chatto binary
when their sources change. Then it runs this binary. `mise dev-full` also runs
Authling, the Runling bot, Mailpit, and LiveKit as parallel tasks of one mise
process. There is no Vite process and no Portless route in either stack. To
see a change, the developer restarts the stack.

All services use plain HTTP on the Conductor port block, with base `4000`
outside Conductor. Chatto uses the base port, and Authling uses `+2`. The
browser-facing hostnames are `chatto.<workspace>.localhost` and
`authling.<workspace>.localhost`. In Conductor, `<workspace>` is `ws`
followed by the Conductor port, for example `ws55060`. Outside Conductor, it is
`local`. Conductor preview URLs can expand the port, but not the workspace name
or ID. The port is different for each concurrent workspace and does not change
when a workspace is renamed. Each workspace therefore has its own cookie scope.
Mailpit, LiveKit, and the Runling console use `localhost`, because they do not
keep browser sessions for this stack. The `[env]` section of `mise.toml`
names each port of the layout.

The development Authling registers Chatto as the conventional confidential
client `chatto-dev`. The `dev-stack-authling` task copies Authling's development
configuration into the workspace state directory and adds this client with the
exact Chatto callback URL. Authling treats names beneath `.localhost` as
loopback hosts for its plain-HTTP public URL and redirect URI rules. Authling
state is in `.context/dev/<workspace>/authling/port-<port>/`. Authling cannot
change its issuer URL, and Conductor can give a workspace a different port.
Thus, each issuer port has its own state directory. The task does not use the
earlier `authling/nats/` directory: its issuer port is unknown, and a different
port stops Authling at startup.

`mise dev-frontend` runs Vite at base port `+1`. Vite proxies API, realtime,
authentication, and OAuth requests to `CHATTO_BACKEND_URL`. The default value is
the Chatto server that `mise dev` runs in the same workspace. Storybook and the
documentation website use their own default ports and select the next free port.

This decision keeps the native processes and the Conductor port allocation
from ADR-078.

## Consequences

- A backend or frontend restart cannot break a proxy route, because no proxy is
  present.
- Frontend changes in the default stack require a restart and a production
  build. `mise dev-frontend` provides hot module replacement when necessary.
- The development stack has no development CA to trust.
- Concurrent workspaces keep separate cookies because their hostnames differ.
- The first start after this change creates new Authling state and a new Chatto
  OAuth client identity. Development Authling accounts from the Portless stack
  are not available.
- Storybook and the documentation website do not have fixed preview URLs. Their
  terminal output shows the selected port.
- Mailpit and LiveKit still run once for each workspace. A later decision can
  replace them with shared machine-wide services.
