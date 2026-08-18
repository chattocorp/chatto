# ADR-075: Run the Complete Development Stack Natively with Pitchfork

**Date:** 2026-08-17

## Status

Accepted

## Context

The repository's complete Conductor and Paseo development environment ran
Chatto, Authling, Storybook, and the documentation website in Docker Compose.
Ordinary source and dependency changes repeatedly invalidated development
images or container-local dependency state. Even after bind-mount and package
cache experiments, starting a workspace commonly took more than two minutes.

Chatto and Authling already run with embedded NATS, while Mailpit and LiveKit
both provide native macOS executables. The remaining browser-facing processes
are native Vite, Storybook, and Astro development servers. Containers therefore
added a build and filesystem-synchronisation layer without providing an
essential development dependency.

Multiple Conductor and Paseo worktrees must still run concurrently, use stable
browser origins, preserve product-owned local state, start in dependency order,
and stop without affecting another worktree.

## Decision

The complete repository development stack runs as native processes supervised
by Pitchfork. `mise` pins and installs Pitchfork, Caddy, Mailpit, LiveKit, Go,
Node.js, and the existing project toolchains. Pitchfork allocates worktree-local ports,
orders service startup, watches the Go services, and exposes Chatto, Authling,
Mailpit, LiveKit, Storybook, and the documentation website through one trusted
HTTPS proxy on port `42443`.

Each worktree registers globally unique `*-<workspace>.localhost` proxy slugs.
Conductor's workspace name is the public slug; outside Conductor the checkout
directory name is used. Chatto and Authling retain separate embedded-NATS and
search state beneath a matching gitignored `.context/dev/<workspace>/`
directory, preventing an Authling issuer created under one public workspace
identity from being reused under another. Vite, Storybook, and Astro consume
the one root pnpm installation rather than maintaining per-container stores.
Mailpit and LiveKit run from their mise-managed native binaries.

Pitchfork discovers a proxy target by inspecting a daemon's listening ports.
That is ambiguous for multi-port servers such as Mailpit and LiveKit and for
Node development servers that briefly open child-process ports. Pitchfork
therefore supervises a small native Caddy reverse-proxy process in front of
Mailpit's HTTP listener, LiveKit's HTTP/WebSocket listener, Storybook, and
Astro. Each Caddy process exposes exactly one listener, making Pitchfork's
public route deterministic while internal dependencies continue to use the
underlying service's allocated ports directly.

`mise dev`, the default Conductor run command, and Paseo's `dev` command all use
the same Pitchfork stack. The `mise dev` task registers the workspace's proxy
routes, attaches to Pitchfork's logs, and stops only that workspace's daemons
when the command exits; Pitchfork owns port allocation, readiness, dependencies,
watching, and process supervision. Pitchfork starts its machine-wide supervisor
automatically. The `mise dev-archive` task removes only that worktree's processes
and routes.

The obsolete root `compose.yml` and its development-only Docker build helpers
are removed. The independently maintained `examples/dockercompose/` deployment
example and all release container assets remain supported; this decision
changes local development orchestration, not Chatto's hosting options.

Authling may explicitly trust a development hostname whose CIMD document
resolves to loopback, and may explicitly consume proxy-sanitized forwarded
origin headers. These exceptions remain opt-in and narrowly scoped; production
CIMD SSRF protections and canonical-origin checks remain the default.

## Consequences

- A warm start brings the complete stack online in seconds and source changes
  no longer trigger container image rebuilds.
- Tool downloads and Go/pnpm caches are shared through mise and the host's
  normal package stores across worktrees.
- Developers need no Docker or OrbStack integration for the repository's
  complete development environment.
- The Pitchfork certificate authority must be trusted once on each development
  machine. Public development URLs include the non-privileged proxy port
  `42443`.
- Native processes share host CPU and memory limits rather than container
  limits, and developers must use the workspace command to stop or archive
  them cleanly.
- The root development workflow is macOS-oriented because Conductor, Paseo,
  `.localhost` routing, and the current native LiveKit installation target that
  environment. Production container and Docker Compose examples remain
  separate deployment artifacts.
