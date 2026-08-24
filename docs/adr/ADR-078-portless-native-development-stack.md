# ADR-078: Route the Native Development Stack with Portless

**Date:** 2026-08-21

## Context

ADR-075 replaced the containerized development stack with native processes
managed by Pitchfork. Native execution made warm startup substantially faster,
and Conductor's per-workspace port blocks kept the application listeners and
embedded data stores isolated.

Pitchfork nevertheless uses one machine-wide supervisor, IPC socket, reverse
proxy, route registry, and log database for every worktree. A stalled
supervisor consequently prevented unrelated Conductor workspaces from
starting, stopping, or archiving their local stacks. Combining project-session
autostop, explicit stop commands, route removal, attached log clients, and
Conductor's short shutdown window also created several competing lifecycle
owners.

The development stack needs stable HTTPS origins for Authling's issuer and
Chatto's CIMD client identifier, but it does not need a persistent daemon
supervisor or durable log store. The process running `mise dev` can own the
five child processes directly.

## Decision

`mise dev` builds the shared frontend inputs, then runs Mailpit, LiveKit,
Authling, the Chatto backend, and the Chatto Vite frontend as concurrent mise
tasks. The existing `tools/dev-supervisor.sh` process-group wrapper forwards
Conductor lifecycle signals and reaps the complete child tree. Stopping
`mise dev` is the single lifecycle operation for the stack.

Portless replaces Pitchfork only as the HTTPS routing layer. Each
browser-facing task runs its process through the mise-pinned Portless CLI. The
tasks use Portless's direct named mode to expose Chatto, Authling,
Mailpit, and LiveKit as `<service>.<workspace>.localhost` on shared proxy port
`42444`.
Portless owns a route for exactly as long as its child process runs.

The standalone `mise storybook` and `mise dev-docs-website` tasks use the same
proxy and expose `storybook.<workspace>.localhost` and
`docs.<workspace>.localhost`, respectively. Portless assigns their loopback
listener ports dynamically so both tasks can run alongside the regular stack
without competing for its Conductor port block. Conductor provides separate run
actions and preview links for both routes.

The application listeners retain ADR-075's Conductor port layout. With
`CONDUCTOR_PORT` as the base, Vite uses the base port, the Chatto backend uses
`+1`, Authling uses `+2`, Chatto's embedded NATS uses `+4`, LiveKit uses `+5`
through `+7`, and Mailpit uses `+8` and `+9`. Outside Conductor the base remains
`4000`. Chatto uses the worktree-local `cli/data/` directory. Authling retains
workspace-hostname-specific state beneath
`.context/dev-portless/<workspace>/nested/authling/`; the separate root prevents
reuse of Pitchfork-era state whose immutable HTTPS issuer used a different
proxy port.

Vite proxies Chatto authentication, OAuth, API, and realtime routes to the
backend without changing the browser-facing `Host` header. The browser
`Origin` and the backend request target therefore remain equal for same-origin
cookie authentication when a developer uses Vite's direct loopback URL.

Portless requires Node.js 24 or newer. Each Portless-backed mise task declares
Node.js 24 and `npm:portless@0.15.5` as task-specific tools, keeping Portless
out of the Node.js 22 pnpm workspace used by the rest of the repository.
Portless creates and trusts its
development CA once per machine. Its host-file synchronization is disabled
because `.localhost` resolves to loopback in the supported browser workflow.

The initial integration does not add a Go file watcher. Vite continues to
reload frontend changes, while Chatto and Authling Go changes require restarting
`mise dev`.

This decision supersedes ADR-075. It retains native processes, Conductor port
allocation, product-owned state, and the separation from production container
artifacts while removing Pitchfork.

## Consequences

- A failure in a persistent development-daemon supervisor can no longer block
  every worktree because no such supervisor owns the application processes.
- `mise dev` and its process tree are the single lifecycle boundary; no archive
  cleanup task or persistent route registry mutation is required.
- Concurrent workspaces keep separate listener ports, state, HTTPS origins,
  and Portless child registrations while sharing Portless's lightweight HTTPS
  proxy.
- Development, Storybook, and docs website URLs use the Conductor workspace
  name and port `42444`, allowing `.conductor/settings.toml` to provide working
  preview links.
- Developers may need to trust Portless's CA interactively on first use.
- Backend and Authling changes temporarily require restarting the stack until
  a simpler watch-and-rebuild mechanism is selected.
- Portless is pre-1.0 and pinned exactly so changes to its routing and lifecycle
  behavior are reviewed deliberately.
