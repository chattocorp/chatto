# Instructions for Agents Working in `authling/`

Read the repository-root [`AGENTS.md`](../AGENTS.md) first, then read this file
in full before any Authling task. The root instructions require this explicit
read because nested agent instructions and skills may not be discovered
automatically.

## Product Boundary

Authling is an independent, self-hostable identity provider. It lives in the
Chatto repository temporarily to make shared-framework extraction practical,
not because it is part of the Chatto product. Once the shared boundary is
stable enough for normal versioned consumption, Authling is intended to move
to its own repository.

- Authling has its own Go module, executable, configuration, HTTP surface,
  lifecycle, data model, documentation, version, changelog, and releases.
- Authling is not a Chatto runtime unit, optional Chatto feature, or special
  kind of Chatto server.
- The primary deployment model is a standalone Authling process. Keep future
  process-level embedding possible through dependency-injected composition, but
  do not build or couple to an embedding mode without a concrete requirement.
- Authling always uses credentials for its own NATS account. It must never
  share a Chatto application's NATS account, even if both runtimes eventually
  occupy one operating-system process.
- Keep Authling-owned files beneath `authling/` unless a root-level integration
  is strictly necessary for workspace, CI, release, instruction, or shared
  framework purposes. Do not add dependencies on Chatto repository layout that
  would obstruct moving this subtree to its own repository.

## Current Product Direction

- Start with standards-compliant OpenID Connect. Do not introduce a custom
  identity protocol while OIDC satisfies the requirement.
- Chatto server operators explicitly choose which OIDC issuers they trust.
  Authling must not imply a global issuer or automatic trust.
- Authling may store a user's server registrations and other light,
  user-controlled metadata. It must not create a "home server" concept or make
  one Chatto server authoritative for the user's identity or server list.
- `chatto.id` may run a convenient hosted Authling instance, while self-hosted
  issuers remain first-class.
- The current experimental runtime persists and replays structural accounts,
  but exposes no public account, authentication, metadata, or OIDC interface.
  Do not document planned identity-provider behavior as implemented.

## Code And Dependency Boundaries

- Authling must not import `hmans.de/chatto/internal/...`, Chatto domain
  packages, Chatto protobuf event envelopes, or Chatto application
  configuration.
- Do not copy Chatto subjects, stream or bucket names, event types, aggregate
  boundaries, runtime-state keys, or diagnostic identities into Authling.
- Reusable mechanics must move behind explicitly application-neutral shared
  package boundaries. The unstable `hmans.de/chatto/pkg/events` module owns
  generic event-sourcing mechanics,
  `hmans.de/chatto/pkg/natsruntime` owns embedded NATS lifecycle mechanics, and
  `hmans.de/chatto/pkg/datacrypto` owns raw XChaCha20-Poly1305 and 256-bit key
  wrapping primitives; Authling owns its associated data and key hierarchy.
  Authling should consume them only for concrete use cases. Each product owns
  its event vocabulary, storage coordinates, identity formats, configuration,
  policy, and composition.
- Changes that extract or modify shared framework code also fall under
  [`cli/AGENTS.md`](../cli/AGENTS.md),
  [ADR-056](../docs/adr/ADR-056-extractable-nats-event-sourcing-framework.md),
  and
  [ADR-057](../docs/adr/ADR-057-temporarily-incubate-authling.md). Embedded
  NATS runtime changes additionally follow
  [ADR-058](../docs/adr/ADR-058-application-neutral-embedded-nats-runtime.md).
  Data-cryptography changes additionally follow
  [ADR-060](../docs/adr/ADR-060-application-neutral-data-cryptography.md).
- Keep Authling independently buildable and testable from its module directory.
  The root `go.work` is a development convenience, not permission to blur module
  dependencies.

## Authling Documentation

Authling owns a complete documentation namespace:

- [`TODO.md`](TODO.md) — outstanding Authling product decisions and
  implementation work.
- [`docs/adr/INDEX.md`](docs/adr/INDEX.md) — Authling architecture decisions,
  numbered independently from Chatto ADRs.
- [`docs/fdr/INDEX.md`](docs/fdr/INDEX.md) — Authling feature behavior and
  rationale, numbered independently from Chatto FDRs.
- [`docs/architecture/INDEX.md`](docs/architecture/INDEX.md) — current Authling
  runtime inventory.
- [`docs/GLOSSARY.md`](docs/GLOSSARY.md) — canonical Authling terminology.

Keep `TODO.md` concise and current during Authling work. Remove completed tasks
instead of retaining a historical checklist. Use ADRs for accepted architecture
decisions, FDRs for implemented feature behavior, and the runtime architecture
inventory for the system that actually exists.

Never add Authling-specific records to the corresponding root `docs/` files.
Cross-product monorepo and shared-framework decisions are the narrow exception
described by the root instructions.

Repository-local skills must live in
[`../.agents/skills/`](../.agents/skills/). Do not create
`authling/.agents/`; agentic tools do not discover project skills there.
Authling-specific skills must use the path
`.agents/skills/authling-<name>/SKILL.md`, have a matching `authling-<name>`
skill name, and state that they operate on Authling's namespaces. Do not
substitute a similarly named Chatto skill. Global, plugin, and other configured
skills remain applicable when their trigger rules match. Release Please treats
repository skills as non-product infrastructure.

## Security

- Treat Authling as security-critical infrastructure. Authentication,
  authorization, consent, redirect handling, issuer metadata, signing keys,
  tokens, recovery, and account linking require explicit threat analysis and
  adversarial tests.
- Never log raw email addresses, login identifiers, provider subjects, tokens,
  authorization codes, passwords, recovery material, signing keys, raw IP
  addresses, or full query strings.
- Persist durable identity facts as events and short-lived credentials or
  workflow state as runtime state, once those stores exist. Do not infer
  Authling's exact subjects or resources from Chatto.
- Default to least privilege and fail closed when identity, key, issuer, or
  authorization state is unavailable.

## Releases And Compatibility

- Authling's Release Please component is `authling/`, its version source is
  `version.go`, its changelog is `CHANGELOG.md`, and its tags use
  `authling/v<version>`. The slash follows Go's nested-module tag convention.
- Authling releases are source-only during the initial scaffold. Add binary or
  container artifact workflows only when there is an implemented runtime worth
  distributing.
- Authling releases are independent from Chatto releases. An Authling-only
  commit must not require a Chatto release.
- Treat persisted identity data, signing-key references, issuer identifiers,
  OIDC subjects, and published protocol behavior as compatibility-sensitive.
  Add migration and mixed-version reasoning before changing them.

## Tooling And Verification

Use repository `mise` tasks:

```sh
mise test-authling
mise build-authling
```

These tasks run with `GOWORK=off` as well as in the repository workspace so
undeclared or unreleased cross-module dependencies cannot be hidden by
`go.work`.

Run the lowest test layer that can catch the failure, but add integration and
protocol tests when behavior crosses HTTP, OIDC, NATS, JetStream, cryptographic,
or process-lifecycle boundaries.
