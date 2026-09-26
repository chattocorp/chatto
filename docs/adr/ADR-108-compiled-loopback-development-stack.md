# ADR-108: Run a Compiled Development Stack on Loopback Hostnames

**Date:** 2026-09-26

## Status

Accepted. Supersedes [ADR-078](ADR-078-portless-native-development-stack.md).

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
when their sources change. Then it runs this binary, Authling, the Runling bot,
Mailpit, and LiveKit in the `tools/dev-supervisor.sh` process group. There is
no Vite process and no Portless route in the default stack. To see a change, the
developer restarts `mise dev`.

All services use plain HTTP on the Conductor port block, with base `4000`
outside Conductor. Chatto uses the base port, and Authling uses `+2`. The
browser-facing hostnames are `chatto.<workspace>.localhost` and
`authling.<workspace>.localhost`, where `<workspace>` is the Conductor
workspace name or `local`. Each workspace therefore has its own cookie scope.
Mailpit, LiveKit, and the Runling console use `localhost`, because they do not
keep browser sessions for this stack. The comment above the `dev` task in
`mise.toml` records the complete port layout.

The development Authling registers Chatto as the conventional confidential
client `chatto-dev`. The `dev-stack-authling` task copies Authling's development
configuration into the workspace state directory and adds this client with the
exact Chatto callback URL. Authling treats names beneath `.localhost` as
loopback hosts for its plain-HTTP public URL and redirect URI rules. Authling
state is in `.context/dev/<workspace>/authling/`. A new directory is necessary
because the issuer URL changed.

`mise dev-frontend` runs Vite at base port `+1`. Vite proxies API, realtime,
authentication, and OAuth requests to `CHATTO_BACKEND_URL`. The default value is
the Chatto server that `mise dev` runs in the same workspace. Storybook and the
documentation website use their own default ports and select the next free port.

This decision keeps the native processes, the Conductor port allocation, and
the archive cleanup from ADR-078.

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
