# Instructions for Agents

Read this file first. It contains repo-wide rules that should not be hidden in
path-specific guidance.

## Product Boundaries And Instruction Routing

This repository contains two independent products plus an incubating shared
framework boundary:

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

Authling's presence in this repository is explicitly temporary. It is being
incubated here only while Authling provides the concrete second application
needed to extract and harden the shared framework. Once that boundary is
stable, Authling is intended to move to its own repository. Do not describe
this repository as Authling's permanent home, and do not introduce coupling
that would make the eventual extraction harder.

Before changing files, classify the task as Chatto, Authling, shared-framework,
or repository-wide work. Follow these routing rules:

1. Any task that concerns Authling or changes anything under `authling/` must
   read [`authling/AGENTS.md`](authling/AGENTS.md) in full before acting. Do
   this explicitly; do not assume nested instructions or skills were discovered
   automatically.
2. Authling behavior, architecture, features, vocabulary, and runtime inventory
   belong under `authling/docs/`. Do not put them in Chatto's `docs/adr/`,
   `docs/fdr/`, `docs/architecture/`, or `docs/GLOSSARY.md`.
3. Repository-local skills must live in the repository-root `.agents/skills/`
   directory. Agentic tools do not discover project skills under
   `authling/.agents/`. Authling skills must live under
   `.agents/skills/authling-<name>/`, use an `authling-` name, and state their
   Authling scope explicitly. These files are repository-level agent
   infrastructure, not product release inputs; Release Please excludes
   `.agents/skills/` from Chatto's root component. Global, plugin, and other
   configured skills remain applicable when their trigger rules match.
4. Existing Chatto documentation and skills are Chatto-specific unless their
   text explicitly says they are repository-wide or Authling-specific. Do not
   apply a Chatto workflow to Authling merely because it has the generic name
   `adr`, `fdr`, or `glossary`.
5. A shared-framework change must read both `cli/AGENTS.md` and
   `authling/AGENTS.md`, the target module's `AGENTS.md`, ADR-057, and the
   module-specific ADR: ADR-056 for `pkg/events`, ADR-058 for
   `pkg/natsruntime`, ADR-060 for `pkg/datacrypto`, or ADR-061 for
   `pkg/appconfig`. Shared packages must not import either product's domain,
   configuration, protobuf envelopes, subjects, resource names, or lifecycle
   policy.
6. Cross-product decisions may be recorded in root ADRs. Product-specific
   decisions must stay with their product. ADR-057 is repository-wide because
   it defines the monorepo boundary; that does not make other Authling ADRs
   Chatto ADRs.
7. Chatto and Authling product code and documentation have independent
   versions, changelogs, release pull requests, tags, binaries, and release
   notes. Never include one product in the other's release artifacts or
   documentation by default. Future artifact types such as container images
   also remain product-owned when introduced. Root-level CI, workspace,
   release, and agent-discovery files are repository infrastructure rather than
   either product's release payload.
8. Keep Authling-owned implementation and documentation beneath `authling/`
   except for the minimum repository-wide workspace, CI, release, instruction,
   and shared-framework integration points. Optimize those exceptions for
   deletion or relocation when Authling leaves this repository.

If a task crosses these boundaries, keep the product impacts explicit in code,
tests, documentation, and the final report. Do not use a cross-product task as
permission to reorganize unrelated product code.

## Where Context Lives

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
- [proto/chatto/realtime/v1/AGENTS.md](proto/chatto/realtime/v1/AGENTS.md) — realtime WebSocket protobuf protocol rules for `chatto.realtime.v1`.
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

## Chatto Project Status

- Chatto is public, self-hosted, and has real user data.
- The project is pre-1.0, but people are already self-hosting Chatto. The public API is experimental: compatibility is preferred, not guaranteed, and `v1` identifies the current wire namespace rather than a long-term stability promise. Prefer additive changes. Breaking public API changes are allowed when they materially improve the design, but discuss them with the user first and include an explicit compatibility plan, generated-client/docs updates, and release-note guidance. Changes to authoritative `core` protobuf messages used by persistence must never be breaking; disposable projection snapshot payloads are the exception described under Public API And Compatibility. Follow ADR-045.
- Assume that mixed versions are in use in the wider ecosystem; but self-hosters have been advised to track `:latest`, or upgrade to newly released versions quickly.
- The next planned version is `0.5.0`. Use the GitHub `0.5.0` milestone as the canonical roadmap and keep its issues current as work progresses. It significantly changes the API's realtime channel, so we are no longer trying to remain API compatible with 0.4.x versions; breaking API changes are OK for this release.
- Keep `CHATTO_DEVELOPMENT_VERSION` in `mise.toml` aligned with the next planned
  release. Local, E2E, compatibility-test, and main-branch snapshot builds must
  advertise this development version so the bundled client applies the same
  release compatibility policy everywhere.

## Prime Directives

- Prefer simple, clear changes over clever abstractions.
- Add concise code documentation for public APIs and for otherwise important
  fields, functions, types, invariants, and lifecycle behavior that future
  maintainers should not have to infer from call sites.
- Keep tests and documentation up to date when changing behavior.
- Run verification that would actually catch regressions in the area touched.
- Never claim full verification when only a partial signal was run.
- Never silence lint, type, vet, or Svelte warnings as a routine fix. Fix the
  cause; discuss rare scoped exceptions before adding them.
- Never log PII: no raw login names, display names, email addresses, submitted
  auth identifiers, OAuth/OIDC provider subjects, tokens, passwords, auth codes,
  reset links, raw IPs, or full query strings.
- Never expose NATS or JetStream storage coordinates through normal client or
  integration APIs. Public cursors and tokens must not reveal stream names or
  incarnations, subjects, sequence numbers, revisions, consumer positions, or
  equivalent internal facts, including through reversible encodings such as
  base64. Opaque coordinates must be integrity-protected and confidential;
  bind them to their viewer/resource scope where applicable, and reject or
  safely reset when validation fails. Explicit owner-only broker diagnostics
  and event-log inspection APIs are the sole exception: their operational
  purpose and fields must clearly identify the NATS/JetStream details exposed.
- Treat optional operational telemetry as best-effort: its failure must not make
  broader diagnostics unavailable. Preserve an explicit unavailable state across
  API and UI boundaries instead of replacing unknown values with healthy-looking
  zeroes, empty strings, or timestamps.

## Tooling

Tools are managed by `mise`; prefer tasks when available.

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

For ad-hoc tool invocations, use `mise x -- ...` rather than assuming `go`,
`pnpm`, `node`, or related binaries are on `PATH`.

When an agent needs the long-running development stack, launch it as
`exec tools/dev-supervisor.sh mise dev` so lifecycle signals reach the dev
supervisor directly, and stop it before handing control back to the user. Never
leave a dev stack running in a detached or yielded terminal session.

## Chatto Backend Principles

- Chatto can run multiple replicas. Correctness must not depend on process-local
  locks, single goroutines, or a single writer.
- NATS JetStream and KV are the primary data store. Use JetStream OCC or KV
  `Create`/revision `Update` for cross-replica invariants.
- Durable domain facts belong in `EVT`. `RUNTIME_STATE` is for persisted
  latest-value runtime records such as sessions, tokens, notification state,
  push subscriptions, cached previews, and wrapped DEK records.
- For hot, high-fanout latest-value KV reads, prefer one process-wide filtered
  watcher and an owning model's in-memory index. Keep KV authoritative, retain
  OCC on writes, and wait for the written revision to reach the local index
  before returning when read-your-writes matters. Do not attach a watcher to
  each request, user, or WebSocket.
- State interactions should go through the owning service/projection boundary.
  Avoid direct JetStream/KV/projection access from unrelated code.
- New public API surface should favor ConnectRPC/protobuf or the planned wire
  protocol.
- A realtime resume cursor must never advance beyond the projection state used
  to authorize and assemble its public operations. Capture a durable boundary,
  wait for the serving projections through it, and fail the catch-up instead of
  publishing stale state at a newer cursor.
- Treat projected authorization loss as a persistent privacy boundary. Purge
  every copied content-bearing or room-sensitive mirror, reject older async
  responses, and reopen the resource only after an explicit positive grant.
- `ServerDiscoveryService.GetServer` is the high-compatibility discovery
  endpoint. Prefer additive changes and preserve public CORS and OAuth
  discovery semantics.

## Chatto Frontend Principles

- Use Svelte 5, Tailwind 4 utilities, and established shared components.
- Avoid `$effect` unless synchronizing with the outside world. Prefer
  `$derived`, event handlers, context getters, and store methods for state flow.
- Review visible frontend changes in the browser using Chrome DevTools MCP.
- User-visible strings go through the British English (`en-GB`) source and all
  complete translated Paraglide catalogs, with sparse US English (`en-US`)
  overrides where wording differs. Preserve message structure and placeholders.
  Follow ADR-043 and
  [apps/frontend/AGENTS.md](apps/frontend/AGENTS.md).
- In user-facing copy, do not prefix end-user accounts, users, members, or
  usernames with the product name. People belong to the community powered by
  Chatto; use "account", "user", "member", or "username" as appropriate.
- Use automatic "load more" pagination for frontend lists, not manual pages.
- Use Save buttons only for multi-field forms that submit together; disable them
  until something changed.
- Server Admin checkboxes and similar binary settings should save immediately
  and confirm via toast.
- Floating UI should reuse established menu/popover/dialog/toast patterns.

## Chatto Public API And Compatibility

- Treat `chatto.auth.v1`, `chatto.discovery.v1`, `chatto.api.v1`,
  `chatto.admin.v1`, and `chatto.realtime.v1` as experimental public contracts.
  Prefer compatibility, but do not preserve a materially worse pre-1.0 design
  solely to avoid a break. Classify every public API change as additive,
  behavioural, deprecated, or breaking and document client migration impact.
- Use the bundled client's internal feature-to-minimum-server-version table for
  version-skew gates. Keep protocol support separate from server configuration
  and authenticated viewer permissions. `ServerDiscoveryService.GetServer`
  reports the server software version; it does not declare client requirements.
- Public ConnectRPC services should live in `chatto.api.v1` for normal
  client/integration behavior and `chatto.admin.v1` for visibly administrative
  behavior. App-specific API should be exceptional, explicitly documented, and
  still stable enough for mixed bundled client/server versions.
- Public API surfaces should be resource-oriented, exhaustive for their
  resource/scope, and not shaped only around the current frontend. Prefer the
  repeatable `List`/`Get`/`BatchGet`/`Create`/`Update`/`Delete` pattern, with
  domain verbs only when CRUD names would hide important semantics.
- Prefer rich protobuf messages over scalar acknowledgements when returning the
  affected resource is cheap and does not change authorization. Prefer explicit
  `BatchGet*` hydration over `includes` maps. Add `includes`-style properties
  only for proven hot paths where many rows repeatedly reference the same
  related render data and follow-up batch hydration would be materially worse.
- Reuse public protobuf shapes for repeated semantics. Offset list RPCs should
  use `PageRequest page` and return `PageInfo page`; singular lookups should
  return `NOT_FOUND` when absence is the error result, while batch/list RPCs can
  omit missing items or return empty lists.
- Reuse canonical API user shapes instead of adding service-local copies:
  `User` for lightweight render/cache references,
  `UserProfile` when presence/custom status is included, and
  `DirectoryMember` for directory/member rows with roles.
- Persisted protobuf messages in `EVT`, `RUNTIME_STATE`, `ENCRYPTION_KEYS`, and
  other JetStream resources are comparatively stable. Do not remove or
  renumber fields or change field types; prefer additive evolution and
  migrations/repair code. Reserving a removed field is not sufficient for
  these storage contracts.
- Projection snapshot payloads are disposable caches and may change
  incompatibly because a missing snapshot cold-replays from EVT. Keep only the
  current codec schema in `projection_snapshots.proto`; old binaries retain
  their own schema and contract namespace. Prior generations remain isolated
  until normal retention removes them, after which that version cold-replays
  EVT. Derive every snapshot contract ID with the shared reachable-schema
  fingerprint helper, and bump its manual semantics token whenever restore
  equivalence changes without a protobuf schema change.
- Transient protobufs can change more freely, but still consider public API
  behavior and mixed-version clients.
- When changing room timeline event visibility, update ConnectRPC room timeline
  mapping or explicitly document why the event is hidden. Add tests so visible
  events cannot be silently dropped.

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
  `pkg/appconfig/`, plus explicit integration and documentation surfaces such
  as the standalone frontend source and image, public protocol/API definitions,
  generated TypeScript API clients, documentation, and examples.
- The Chatto server, CLI, and bundled server release artifacts should stay
  AGPL-3.0-or-later unless the license boundary is deliberately changed.

## Chatto Code Generation

- Public `.proto` or ConnectRPC changes require `mise codegen-proto` after
  rebasing onto the target branch, and generated Go/TS/docs outputs must be
  committed.
- New public ConnectRPC services also need `proto/buf.gen.yaml` and docs sidebar
  entries in `apps/docs-website/astro.config.mjs`.
## Issues, Commits, And PRs

- Use GitHub Issues for planning.
- Use Conventional Commit format for commits and PR titles, for example
  `fix(api): ...` or `feat(frontend)!: ...`. Only mark breaking changes when
  they really are breaking.
- Always create pull requests as full, ready-for-review PRs. Create a draft PR
  only when the user explicitly asks for a draft.
- PR bodies should summarize changes and link relevant FDRs, ADRs, glossary
  terms, and issues.
- If a PR closes an issue, include a GitHub closing keyword such as
  `Closes #123.` in the body.
- When using `gh` for multiline PR/issue bodies, write Markdown to a file/stdin
  and use `--body-file`; do not pass escaped `\n` to `--body`.
- After creating or editing a PR, verify the stored body and issue-closing
  wiring with `gh pr view --json body,baseRefName,closingIssuesReferences`.
- After creating a PR, check CI and fix failures that are regressions from
  `main`.
- Do not rename the current branch unless explicitly asked.

## Testing Judgment

- Pick the lowest test layer that exercises the change, but do not stop below
  the layer where the bug could occur.
- Svelte runtime errors, hydration issues, missing context, and `$effect` loops
  require mounting a component or browser verification.
- Backend refactors that touch subjects, streams, projections, authorization, or
  live delivery usually need targeted Go tests plus relevant e2e coverage.
- E2E tests run locally without Docker/Tilt/OrbStack; Playwright starts its own
  embedded-NATS Chatto binary.
