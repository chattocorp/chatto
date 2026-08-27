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

Before you change a file, classify the task as Chatto, Authling,
shared-framework, or repository-wide work. Follow these routing rules:

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

For a task that crosses these boundaries, state each product impact in code,
tests, documentation, and the final report. Do not use a cross-product task to
reorganize unrelated product code.

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

## Instruction Strength

- **Must** and **never** mark requirements, safety boundaries, and invariants.
- **Prefer** marks the default. Use another action only for a specific reason.
- **Consider** marks a review prompt, not a required action.
- The nearest applicable `AGENTS.md` controls path-specific guidance. Root
  rules still apply when nested guidance is more specific.

## Simplified Technical English

Use [ASD-STE100 Simplified Technical English, Issue 9
(January 2025)](https://www.asd-ste100.org/assets/files/ASD-STE100_ISSUE9.pdf),
including its controlled dictionary, for all new or changed documentation in
this repository. This scope includes the docs website.

Use canonical product, API, user-interface, and configuration terms as
technical terms. For Chatto terms, use the canonical vocabulary in
[`docs/GLOSSARY.md`](docs/GLOSSARY.md). Do not change code, commands, literal
names, or quotations.

Treat a violation in unchanged text as migration work. Apply the standard to a
complete page when you make a substantial page edit. Claim formal conformance
only after a complete review against the standard and its dictionary.

### Approved Exclusions

These pages can use a conversational product voice and vocabulary outside
ASD-STE100:

- `apps/docs-website/src/content/docs/index.mdx` — product home page.
- `apps/docs-website/src/content/docs/getting-started/introduction.mdx` —
  narrative product introduction.

ASD-STE100 still applies to technical instructions and safety information on an
excluded page. Add an exclusion only with explicit user approval. Record its
full path and reason here.

## Prime Directives

- Prefer simple, clear changes to complex abstractions.
- Add short code documentation for public APIs and important fields, functions,
  types, invariants, and lifecycle behavior. Future maintainers must not have
  to infer this information from call sites.
- Keep tests and documentation up to date when changing behavior.
- Run verification that can find regressions in the changed area.
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
- Treat optional operational telemetry as best effort. Its failure must not make
  other diagnostics unavailable. Preserve an explicit unavailable state across
  API and UI boundaries. Do not replace an unknown value with a zero, empty
  string, or timestamp that appears healthy.
- Chatto is public, self-hosted, pre-1.0 software with real user data and mixed
  versions in use. Follow ADR-045 and `proto/AGENTS.md` for public and persisted
  protocol compatibility. A breaking experimental API change requires explicit
  user approval and a compatibility plan; a release milestone does not waive
  that requirement.

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

- Use or update GitHub issues only when the user asks for issue or roadmap
  management, or when an explicitly invoked workflow requires it.
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
- Do not rename the current branch unless explicitly asked.

## Testing Judgment

- Pick the lowest test layer that exercises the change, but do not stop below
  the layer where the bug could occur.
- When testing an early rejection, use input that would fail a later check. The
  test should still return the early error.
- Choose additional integration or end-to-end coverage when the regression can
  occur only across component or process boundaries.
