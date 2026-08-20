# ADR-075: Run the Regular Development Stack Natively with Pitchfork

**Date:** 2026-08-17

**Updated:** 2026-08-20

## Status

Accepted

## Context

The repository's complete worktree development environment originally ran
Chatto, Authling, Storybook, and the documentation website in Docker Compose.
Ordinary source and dependency changes repeatedly invalidated development
images or container-local dependency state. Even after bind-mount and package
cache experiments, starting a workspace commonly took more than two minutes.

Chatto and Authling already run with embedded NATS, while Mailpit and LiveKit
both provide native macOS executables. The remaining browser-facing processes
are native Vite, Storybook, and Astro development servers. Containers therefore
added a build and filesystem-synchronisation layer without providing an
essential development dependency.

Multiple Conductor worktrees must still run concurrently, preserve
product-owned local state, start in dependency order, and stop without
affecting another worktree. Conductor already allocates a stable,
non-overlapping block of ten ports to each local workspace. Chatto's CIMD
client identifier still requires HTTPS, and stable HTTPS origins are useful for
the other browser-facing services when Pitchfork can proxy them directly.

## Decision

The regular development stack runs as native processes supervised by
Pitchfork. `mise` pins and installs Pitchfork, Mailpit, LiveKit, Go, Node.js,
and the existing project toolchains. The default stack contains five daemons:
Mailpit, LiveKit, Authling, the Chatto backend, and the Chatto Vite frontend.
Pitchfork orders their startup, watches the Go services, and stops only the
current worktree's daemon namespace.

`mise dev` records `CONDUCTOR_PORT`, or `4000` outside Conductor, in the
gitignored `.context/dev/port-base` file before starting Pitchfork. Daemons read
that file instead of inheriting an environment variable from Pitchfork's
machine-wide supervisor. Chatto's browser-facing Vite server uses the base
port, its backend uses offset `+1`, and Authling uses `+2`. LiveKit uses offsets
`+5` through `+7`; Mailpit SMTP and UI use `+8` and `+9`. The known layout also
lets LiveKit address Chatto's webhook while Chatto addresses LiveKit without a
cyclic Pitchfork dependency. Internal listener, SMTP, and webhook connections
use plain-HTTP loopback where applicable. Pitchfork's own reverse proxy exposes
Chatto, Authling, Mailpit, and LiveKit at workspace-specific HTTPS hostnames on
port `42443`. Pitchfork generates and trusts the certificate and proxies each
hostname directly to its daemon's declared HTTP listener; no adapter process is
required. For daemons with several listeners, `mise dev` declares the intended
HTTP listener when starting the daemon so Pitchfork does not infer an SMTP,
RTC, or ephemeral listener as the proxy target. Mailpit explicitly allows its
workspace HTTPS origin for same-origin API requests. The HTTPS Chatto origin
satisfies CIMD's client-identifier requirement, while the HTTPS Authling origin
is the development issuer. The development issuer has no configured
conventional client; Chatto is discovered exclusively through CIMD.

Pitchfork derives its daemon namespace from the checkout directory, so each
Conductor worktree remains isolated. Chatto and Authling retain separate
embedded-NATS and search state beneath `.context/dev/<workspace>/` in that
worktree. The workspace component is also part of the public HTTPS origins and
Authling's immutable issuer identity, so changing the workspace name selects a
fresh state namespace instead of trying to reuse state created for another
issuer. The `mise dev` task registers the four workspace-named service routes,
enters a Pitchfork project session tied to the run process, starts the `dev`
group, and attaches to its logs. Every daemon is marked for auto-stop, so
Pitchfork stops the namespace when the project session leaves or its host
process disappears. The `mise dev-archive` task explicitly stops the namespace
and removes its service routes.

The shared API types and Lingua packages are built by `mise setup`; the regular
stack does not keep separate TypeScript package watchers running. Storybook and
the documentation website remain available through their explicit mise tasks
but are not part of `mise dev`. Mailpit and LiveKit run from their mise-managed
native binaries. The Pitchfork reverse proxy, local certificate authority, and
global slug registry provide the four stable development origins. Caddy
adapters are not used.

The obsolete root `compose.yml` and its development-only Docker build helpers
are removed. The independently maintained `examples/dockercompose/` deployment
example and all release container assets remain supported; this decision
changes local development orchestration, not Chatto's hosting options.

Authling explicitly trusts the one workspace-specific Chatto development
hostname to resolve to loopback. Production CIMD rules continue to require
HTTPS and reject private and special-use destinations by default. Chatto keeps
the same separation for frontend CIMD SSRF protections, and canonical-origin
checks remain the default.

## Consequences

- A warm start brings the regular stack online in seconds and source changes
  no longer trigger container image rebuilds.
- Tool downloads and Go/pnpm caches are shared through mise and the host's
  normal package stores across worktrees.
- Developers need no Docker or OrbStack integration for the regular
  development environment.
- Concurrent Conductor workspaces use their assigned ten-port blocks. Outside
  Conductor, only one default-base stack can run at a time unless the caller
  supplies `CONDUCTOR_PORT`.
- Browser-facing services use Pitchfork-managed HTTPS hostnames; their native
  listeners and internal service connections remain isolated on the
  workspace's allocated loopback ports.
- Pitchfork's certificate authority must be trusted once per machine, and the
  four service proxy routes must be removed when a workspace is archived.
- Existing plain-HTTP, unnamespaced development state is not reused because an
  Authling issuer cannot change its canonical URL in place.
- Renaming a Conductor workspace preserves its old state directory but starts
  the renamed workspace with fresh state matching its new HTTPS issuer.
- Storybook, the documentation website, and package watch builds must be
  started explicitly when work requires them.
- Native processes share host CPU and memory limits rather than container
  limits, and developers must use the workspace command to stop or archive
  them cleanly.
- The root development workflow is macOS-oriented because Conductor,
  `.localhost` routing, and the current native LiveKit installation target that
  environment. Production container and Docker Compose examples remain
  separate deployment artifacts.
