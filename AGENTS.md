# Instructions for Agents

Read this file first. It contains rules for the complete repository.

## Product Boundaries And Instruction Routing

This repository has two independent products and an incubating shared-framework
boundary:

- **Chatto** is the chat server, bundled client, CLI, and existing public
  protocols. Unless a path is explicitly Authling-owned or shared, existing
  repository content belongs to Chatto.
- **Authling** is the independent identity-provider product under `authling/`.
  It is not a Chatto component, runtime unit, feature, or deployment mode.
- **Shared framework code** is application-neutral event-sourcing, embedded
  NATS, data-cryptography, and configuration-loading machinery intended for
  consumption by both products. The independently versioned but unstable
  modules live under `pkg/events/`, `pkg/natsruntime/`, `pkg/datacrypto/`, and
  `pkg/appconfig/`.

Authling is in this repository temporarily. It provides the second application
needed to extract and validate the shared framework. Move Authling to its own
repository when the shared boundary is stable. Do not describe this repository
as its permanent home. Do not add coupling that makes this move more difficult.

## Prime Directives

- Use ASD-STE100 Simplified Technical English for all new or changed documentation (repository and public documentation!) Find the canonical vocabulary in [`docs/GLOSSARY.md`](docs/GLOSSARY.md).
- The nearest applicable `AGENTS.md` controls path-specific guidance. Root rules still apply when nested guidance is more specific.
- Add code documentation for public APIs and important fields, functions, types, invariants, and lifecycle behavior. Future maintainers must not have to infer this information from call sites.
- Keep tests and documentation up to date when changing behavior.
- Run verification that can find regressions in the changed area.
- Never claim full verification when only a partial signal was run.
- Never silence lint, type, vet, or Svelte warnings as a routine fix. Fix the cause; discuss rare scoped exceptions before adding them.
- Never log PII: no raw login names, display names, email addresses, submitted auth identifiers, OAuth/OIDC provider subjects, tokens, passwords, auth codes, reset links, raw IPs, or full query strings.

## Current Project Status

- Chatto is public, self-hosted, pre-1.0 software with real user data and mixed versions in use.
- Follow ADR-045 and `proto/AGENTS.md` for public and persisted protocol compatibility.
- We are working on version 0.5 of Chatto. 0.5's API already contains many breaking changes from previous versions, so keeping API and frontend compatibility is no longer a priority; but you must make sure that pre-0.5 Chatto servers can be upgraded cleanly, so all protocol buffers involved in persistence need to be backwards compatible where feasible.

## Additional Agent Rules & Context

- [README.md](README.md) — general project overview.
- [authling/AGENTS.md](authling/AGENTS.md) — mandatory Authling product,
  architecture, documentation, security, and testing rules.
- [authling/docs/README.md](authling/docs/README.md) — Authling-owned ADR, FDR,
  architecture, and glossary entry points.
- [pkg/events/AGENTS.md](pkg/events/AGENTS.md) — shared event-framework module
  boundary, compatibility, and verification rules.
- [pkg/natsruntime/AGENTS.md](pkg/natsruntime/AGENTS.md) — shared embedded-NATS
  lifecycle module boundary and verification rules.
- [pkg/datacrypto/AGENTS.md](pkg/datacrypto/AGENTS.md) — shared authenticated
  encryption and key-wrapping boundary and verification rules.
- [pkg/appconfig/AGENTS.md](pkg/appconfig/AGENTS.md) — shared TOML and
  environment configuration-loading boundary and verification rules.
- [cli/AGENTS.md](cli/AGENTS.md) — Go backend, ConnectRPC, NATS/JetStream, authz, live events, backup/restore, and backend tests.
- [apps/frontend/AGENTS.md](apps/frontend/AGENTS.md) — SvelteKit frontend, Tailwind, i18n, browser verification, frontend tests, e2e, and Storybook.
- [proto/AGENTS.md](proto/AGENTS.md) — protobuf and generated public API reference guidance.
- [proto/chatto/api/v1/AGENTS.md](proto/chatto/api/v1/AGENTS.md) — public ConnectRPC API consistency rules for `chatto.api.v1`.
- [proto/chatto/admin/v1/AGENTS.md](proto/chatto/admin/v1/AGENTS.md) — administrative ConnectRPC API consistency rules for `chatto.admin.v1`.
- [proto/chatto/auth/v1/AGENTS.md](proto/chatto/auth/v1/AGENTS.md) — public authentication and capability-token API consistency rules.
- [proto/chatto/discovery/v1/AGENTS.md](proto/chatto/discovery/v1/AGENTS.md) — unauthenticated discovery and bootstrap API consistency rules.
- [proto/chatto/realtime/v1/AGENTS.md](proto/chatto/realtime/v1/AGENTS.md) — realtime WebSocket protobuf protocol rules for `chatto.realtime.v1`.
- [apps/desktop/AGENTS.md](apps/desktop/AGENTS.md) — desktop integration and native-helper testing guidance.
- [apps/docs-website/AGENTS.md](apps/docs-website/AGENTS.md) — public docs website guidance.
- `.agents/skills/**` — discoverable workflow skills. Skills prefixed
  `authling-` are Authling-specific; existing generic and `chatto-` skills are
  Chatto-specific unless their text explicitly says otherwise.
- `docs/fdr/INDEX.md` — Chatto feature behavior and rationale.
- `docs/adr/INDEX.md` — Chatto and explicitly repository-wide architecture
  decisions.
- `docs/architecture/INDEX.md` — current Chatto runtime inventory, split by
  components, projections, NATS resources, subjects, runtime state, effects,
  interfaces, and realtime delivery.
- `docs/GLOSSARY.md` — canonical Chatto terminology.

## Tooling

`mise` manages tools. Prefer its tasks when they are available.

Use Chrome DevTools MCP only to inspect and verify Chatto or Authling browser
behavior. Do not use it for general web research or public documentation
research. Use the available web or document research tools for those tasks.

```sh
mise test
mise test-cli
mise test-events
mise test-natsruntime
mise test-datacrypto
mise test-appconfig
mise test-frontend
mise test-e2e
mise codegen
mise codegen-proto
(cd authling && mise test)
(cd authling && mise test-e2e)
(cd authling && mise build)
```

Run Authling's unprefixed tasks from `authling/`; its nested `mise.toml` owns
the Authling toolchain and workflow.

For an ad-hoc tool command, use `mise x -- ...`. Do not assume that `go`,
`pnpm`, `node`, or related binaries are on `PATH`.

When an agent needs the long-running development stack, launch `mise dev`; the
task runs the child processes through `tools/dev-supervisor.sh` so lifecycle
signals reach them directly. Stop it before handing control back to the user.
Never leave a dev stack running in a detached or yielded terminal session.

## Chatto Documentation Updates

- Use FDRs for feature behavior/rationale and ADRs for cross-cutting decisions.
- Update the relevant file in `docs/architecture/` when changing runtime
  components, projections, EVT events or subjects, NATS resources, runtime
  state, durable effects, realtime delivery, or mounted ConnectRPC services.
- Update `docs/GLOSSARY.md` when introducing, renaming, or clarifying canonical
  vocabulary.
- Update the docs website when changing user-facing features, config,
  deployment behavior, or public APIs.
- Keep `NOTICE` current when adding, removing, or materially changing bundled
  dependencies or shipped assets.

## License Metadata

- Chatto uses REUSE/SPDX license metadata. Keep `mise license-check` passing
  when adding files or changing license boundaries.
- Files are AGPL-3.0-or-later by default unless `REUSE.toml`, an SPDX header,
  or an adjacent `.license` file says otherwise.
- Apache-2.0 applies to the independently versioned shared framework modules
  under `pkg/events/`, `pkg/natsruntime/`, `pkg/datacrypto/`, and
  `pkg/appconfig/`, the framework-neutral `packages/lingua` runtime, plus
  explicit integration and documentation surfaces such as the standalone
  frontend source and image, public protocol/API definitions, generated
  TypeScript API clients, documentation, and examples.
- The Chatto server, CLI, and bundled server release artifacts should stay
  AGPL-3.0-or-later unless the license boundary is deliberately changed.

## Issues, Commits, And PRs

- Use Conventional Commit format for commits and PR titles, for example `fix(api): ...` or `feat(frontend)!: ...`. Only mark breaking changes when they really are breaking.
- Always create pull requests as full, ready-for-review PRs. Create a draft PR only when the user explicitly asks for a draft.
- PR bodies should use clean Markup and summarize changes and link relevant FDRs, ADRs, glossary terms, and issues.
- If a PR closes an issue, include a GitHub closing keyword such as `Closes #123.` in the body.
