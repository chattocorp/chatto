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
> task adds the values that depend on the workspace and TestBot, because
> either command can do the first start that bootstraps the server.
> `mise dev-full` enables email and LiveKit and adds the Authling login
> provider through environment values. Most work needs only Chatto, so the default command
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

### Optional Paseo service proxy

Paseo's **dev** service runs `tools/paseo-dev.sh`. The launcher passes the
allocated `PASEO_PORT` and `PASEO_URL` to mise as the Chatto listener port and
public URL. It uses `exec` so Paseo can stop the complete mise process tree.
The launcher sets `CHATTO_WEBSERVER_BIND_ADDRESS=127.0.0.1` so Chatto binds
to loopback, and keeps data in the worktree's `cli/data/`.
The NATS TCP listener is disabled with port `0`; Chatto uses its existing
in-process NATS connection. No adjacent port reservation is required.

An optional machine-level Caddy wildcard route provides HTTPS over a VPN and
forwards requests to Paseo's loopback service proxy. Paseo owns route creation
and removal. Caddy needs no changes when workspaces start or stop. The machine's
Paseo configuration sets the HTTPS service base URL. Credentials, domain names,
and proxy configuration stay outside the repository.

Paseo names routes from the service, branch, and project. These names fit in
one DNS label and work with a wildcard certificate. Branch changes can change
the URL, and Paseo rejects concurrent routes with the same name. This setup
also applies to **dev-full**, which runs Chatto with Mailpit and LiveKit.
Authling and Runling are not part of **dev-full** in Paseo. Storybook, the docs
website, and Authling are separate, on-demand Paseo services. Each binds to its
allocated loopback port and gets its own proxy route. Authling's task stays in
its own task catalog and keeps separate state for each public issuer URL.
Normal terminal and Conductor starts keep their existing defaults.

The **dev-full** service runs a mise task graph. Preparation checks that the
workspace can start a new stack and writes private configuration. The main
task starts the two support services through Paseo's CLI, waits for Paseo to
report them healthy, and runs Chatto alongside a service-status check.
mise stops parallel tasks on failure or a signal. Its `depends_post` task
stops the two Paseo services and removes the configuration. Preparation is a
dependency, so rejected starts do not stop an existing stack. Each service
gets a separate route and port; Chatto uses LiveKit's public URL.
There are no custom process groups, PID watchdogs, or readiness files.
All Paseo commands use `exec` so process exit also ends the supervised terminal
and removes its route. **dev** and **dev-full** use the same worktree data and
must not run together.

Mailpit uses an OS-selected loopback SMTP port and its Paseo port for HTTP.
LiveKit uses its Paseo port for loopback HTTP and, in the separate UDP port
space, for media. Its built-in TURN/STUN server uses Mailpit's SMTP port number for
UDP and can allocate relay ports in `49152–65535`. These UDP listeners bind to
the local IPv4 address of the service hostname. An explicit
`CHATTO_DEV_LIVEKIT_NODE_IP` can select another local interface for split DNS.
TCP media is disabled because LiveKit does not restrict that listener to the
selected interface. The local TURN/STUN service replaces LiveKit's default
public STUN servers, so browser call setup does not need to contact Google.

The stack generates LiveKit credentials for each run. Private runtime state
stays in `.context/paseo-stack/<dev-full-port>/` and is removed on stop. Separate
run directories prevent old cleanup from deleting a new run's configuration.
After `SIGKILL`, developers must stop the services and remove stale configuration
manually; the stack does not provide a second supervisor to recover from forced
termination. Mailpit's inbox lasts
only for that run. Caddy needs no additional routes or configuration changes.

## Consequences

- Normal terminal and Conductor starts have no proxy route to maintain. Paseo
  starts depend on its service proxy; stop and restart through Paseo to keep
  the route and process lifecycle together.
- Frontend changes in the default stack require a restart and a production
  build. `mise dev-frontend` provides hot module replacement when necessary.
- The development stack has no development CA to trust.
- Concurrent workspaces keep separate cookies because their hostnames differ.
- The first start after this change creates new Authling state and a new Chatto
  OAuth client identity. Development Authling accounts from the Portless stack
  are not available.
- Storybook and the documentation website do not have fixed preview URLs. Their
  terminal output shows the selected port.
- With `mise dev-full`, Mailpit and LiveKit still run once for each workspace.
  A later decision can replace them with shared machine-wide services.
