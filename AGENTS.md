# Instructions for Agents

Read this file first. It contains repo-wide rules that should not be hidden in
path-specific guidance.

## Where Context Lives

- [README.md](README.md) — general project overview.
- [cli/AGENTS.md](cli/AGENTS.md) — Go backend, ConnectRPC, NATS/JetStream, authz, live events, backup/restore, and backend tests.
- [apps/frontend/AGENTS.md](apps/frontend/AGENTS.md) — SvelteKit frontend, Tailwind, i18n, browser verification, frontend tests, e2e, and Storybook.
- [proto/AGENTS.md](proto/AGENTS.md) — protobuf and generated public API reference guidance.
- [proto/chatto/api/v1/AGENTS.md](proto/chatto/api/v1/AGENTS.md) — public ConnectRPC API consistency rules for `chatto.api.v1`.
- [proto/chatto/admin/v1/AGENTS.md](proto/chatto/admin/v1/AGENTS.md) — administrative ConnectRPC API consistency rules for `chatto.admin.v1`.
- [proto/chatto/realtime/v1/AGENTS.md](proto/chatto/realtime/v1/AGENTS.md) — realtime WebSocket protobuf protocol rules for `chatto.realtime.v1`.
- [apps/docs-website/AGENTS.md](apps/docs-website/AGENTS.md) — public docs website guidance.
- `.agents/skills/**` — workflow skills. Use them when the task names one or clearly matches one, especially `chatto-architecture`, `glossary`, Svelte skills, ADR/FDR skills, and security/release workflows.
- `docs/fdr/INDEX.md` — feature behavior and rationale.
- `docs/adr/INDEX.md` — cross-cutting architecture decisions.
- `docs/ARCHITECTURE.md` — current inventory of services, streams, buckets, subjects, projections, realtime delivery, and ConnectRPC APIs.
- `docs/GLOSSARY.md` — canonical Chatto terminology.

## Project Status

- Chatto is public, self-hosted, and has real user data.
- The project is pre-1.0, so breaking changes can be acceptable, but storage,
  protobuf, discovery, and client compatibility still need an explicit plan.
- Some self-hosters track `:latest`; assume mixed deployed versions can exist.
- The ConnectRPC API is still settling. Prefer making `chatto.api.v1` a clean,
  broad base API and `chatto.admin.v1` a clearly administrative public API,
  with explicit compatibility notes over moving ordinary frontend-used features
  into an app-only namespace.
- As long as we haven't released 0.4.0, breaking changes to the ConnectRPC are not only okay, but even encouraged (if they are for cleanup/DRY/etc.) - after we've released 0.4.0, we'll want to be more careful.

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

## Tooling

Tools are managed by `mise`; prefer tasks when available.

```sh
mise test
mise test-cli
mise test-frontend
mise test-e2e
mise codegen
mise codegen-proto
mise codegen-types
```

For ad-hoc tool invocations, use `mise x -- ...` rather than assuming `go`,
`pnpm`, `node`, or related binaries are on `PATH`.

## Backend Principles

- Chatto can run multiple replicas. Correctness must not depend on process-local
  locks, single goroutines, or a single writer.
- NATS JetStream and KV are the primary data store. Use JetStream OCC or KV
  `Create`/revision `Update` for cross-replica invariants.
- Durable domain facts belong in `EVT`. `RUNTIME_STATE` is for persisted
  latest-value runtime records such as sessions, tokens, notification state,
  push subscriptions, cached previews, and wrapped DEK records.
- State interactions should go through the owning service/projection boundary.
  Avoid direct JetStream/KV/projection access from unrelated code.
- New public API surface should favor ConnectRPC/protobuf or the planned wire
  protocol.
- `ServerDiscoveryService.GetServer` is the high-compatibility discovery
  endpoint. Prefer additive changes and preserve public CORS and OAuth
  discovery semantics.

## Frontend Principles

- Use Svelte 5, Tailwind 4 utilities, and established shared components.
- Avoid `$effect` unless synchronizing with the outside world. Prefer
  `$derived`, event handlers, context getters, and store methods for state flow.
- Review visible frontend changes in the browser using Chrome DevTools MCP.
- User-visible strings go through Paraglide message catalogs with both English
  and German entries. Follow ADR-043 and [apps/frontend/AGENTS.md](apps/frontend/AGENTS.md).
- In user-facing copy, do not prefix end-user accounts, users, members, or
  usernames with the product name. People belong to the community powered by
  Chatto; use "account", "user", "member", or "username" as appropriate.
- Use automatic "load more" pagination for frontend lists, not manual pages.
- Use Save buttons only for multi-field forms that submit together; disable them
  until something changed.
- Server Admin checkboxes and similar binary settings should save immediately
  and confirm via toast.
- Floating UI should reuse established menu/popover/dialog/toast patterns.

## Public API And Compatibility

- Public ConnectRPC services should live in `chatto.api.v1` for normal
  client/integration behavior and `chatto.admin.v1` for visibly administrative
  behavior. App-specific API should be exceptional, explicitly documented, and
  still stable enough for mixed bundled client/server versions.
- Public API surfaces should be resource-oriented, exhaustive for their
  resource/scope, and not shaped only around the current frontend. Prefer the
  repeatable `List`/`Get`/`BatchGet`/`Create`/`Update`/`Delete` pattern, with
  domain verbs only when CRUD names would hide important semantics.
- Prefer rich protobuf messages over scalar acknowledgements when returning the
  affected resource is cheap and does not change authorization. Provide batch
  hydration or include-map patterns where clients would otherwise need N+1
  reads.
- Reuse public protobuf shapes for repeated semantics. Offset list RPCs should
  use `PageRequest page` and return `PageInfo page`; singular lookups should
  return `NOT_FOUND` when absence is the error result, while batch/list RPCs can
  omit missing items or return empty lists.
- Reuse canonical API user shapes instead of adding service-local copies:
  `User` for lightweight render/cache references,
  `UserProfile` when presence/custom status is included, and
  `DirectoryMember` for directory/member rows with roles.
- Persisted protobuf messages in `EVT`, `RUNTIME_STATE`, `ENCRYPTION_KEYS`, and
  other JetStream resources are comparatively stable. Do not renumber fields or
  change field types; prefer additive evolution and migrations/repair code.
- Transient protobufs can change more freely, but still consider public API
  behavior and mixed-version clients.
- When changing room timeline event visibility, update ConnectRPC room timeline
  mapping or explicitly document why the event is hidden. Add tests so visible
  events cannot be silently dropped.

## Documentation Updates

- Use FDRs for feature behavior/rationale and ADRs for cross-cutting decisions.
- Update `docs/ARCHITECTURE.md` when changing core services, projections, EVT
  events or subjects, NATS resources, realtime delivery, or ConnectRPC APIs.
- Update `docs/GLOSSARY.md` when introducing, renaming, or clarifying canonical
  vocabulary.
- Update the docs website when changing user-facing features, config,
  deployment behavior, or public APIs.
- Keep `NOTICE` current when adding, removing, or materially changing bundled
  dependencies or shipped assets.

## Code Generation

- Public `.proto` or ConnectRPC changes require `mise codegen-proto` after
  rebasing onto the target branch, and generated Go/TS/docs outputs must be
  committed.
- New public ConnectRPC services also need `proto/buf.gen.yaml` and docs sidebar
  entries in `apps/docs-website/astro.config.mjs`.
- Shared Go types used by frontend TypeScript require `mise codegen-types`.

## Issues, Commits, And PRs

- Use GitHub Issues for planning.
- Use Conventional Commit format for commits and PR titles, for example
  `fix(api): ...` or `feat(frontend)!: ...`. Only mark breaking changes when
  they really are breaking.
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
