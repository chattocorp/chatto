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
- Authling stores identity-provider state only: accounts, credentials, browser
  sessions, issuer and signing-key material, and short-lived OIDC protocol
  state. Application preferences, server catalogues, documents, and generic
  synchronization are outside the product boundary.
- `chatto.id` may run a convenient hosted Authling instance, while self-hosted
  issuers remain first-class.
- The current experimental runtime persists and replays local accounts,
  exposes server-rendered verified-email signup, password login, browser
  sessions and session management, password reset, signed-in password change,
  verified email change, and logout. It provides durable exact-client OIDC
  authorization grants and the narrow OpenID Connect surface recorded in
  FDR-004 and FDR-010. It has no public
  account-management, application-data, document, or synchronization API. Do
  not document other planned identity-provider behavior as implemented.

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
  wrapping primitives. `hmans.de/chatto/pkg/appconfig` owns TOML and
  environment loading mechanics. Authling owns its associated data, key
  hierarchy, configuration schema, environment names, defaults, and
  validation.
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
  Configuration-loading changes additionally follow
  [ADR-061](../docs/adr/ADR-061-application-neutral-configuration-loading.md).
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

## Identity Events And Recovery

- Treat `authling.core.v1.Event` and every reachable event payload as a
  persisted storage contract. Existing fields and oneof tags must never be
  removed, renumbered, reused, or incompatibly retyped. New event variants
  require historical-replay and mixed-version rollout reasoning.
- Durable, PII-free identity and security facts belong in `AUTHLING_EVT`.
  Expiring OTP digests, recovery bearers, attempt counters, delivery limits,
  and other workflow coordination belong in encrypted
  `AUTHLING_RUNTIME_STATE`, not permanent event history.
- Never put raw email addresses, login identifiers, provider subjects, IP
  addresses, user agents, OTPs, recovery bearers, reset links, or equivalent
  sensitive material in event envelopes, event payloads, subjects, runtime
  keys, URLs, or logs. Correlate related durable facts with opaque identifiers.
- When auditability requires an event before an external effect, commit the
  event before creating the dependent workflow or performing that effect.
  State precisely what the event proves, such as request acceptance rather
  than successful email delivery.
- Commands that append to an existing aggregate must use subject-level OCC.
  After a conflict, wait for and re-read the authoritative projection, then
  decide from the command's semantic preconditions. An audit-only event may
  advance an aggregate tail without changing its credential or identity state.
- A command whose semantic precondition can be advanced by events on multiple
  subjects must capture and wait for every relevant projection boundary before
  evaluating that precondition. If an atomic cross-subject batch materializes
  in stages, the command must also reject the interval while dependent state is
  staged but not yet active. Add an adversarial interleaving test that proves
  the command cannot commit history that fails ordered live projection or cold
  replay.
- Projectors must validate and deterministically replay every historical event.
  An audit-only event may intentionally leave the materialized account model
  unchanged, but it must not be silently ignored or weaken replay validation.
- Account recovery and identity mutations must explicitly define their effect
  on stable account IDs and OIDC `sub` values, verified identifiers, Authling
  browser sessions, issued OIDC tokens, and relying-party sessions.
- Enumeration resistance covers the complete observable flow, including HTTP
  status, browser copy, delivery behavior, storage failures, and timing where
  practical. Do not preserve attacker-controlled failures permanently merely
  to make them auditable; use bounded operational or runtime state instead.

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

### Browser UI And Tailwind

- Define reusable browser UI patterns as Tailwind component classes in
  `web/src/app.css` under `@layer components`. Compose a low-level base class
  with variants where appropriate, such as `button` plus `button-primary`,
  instead of repeating the same utility bundle across templ pages.
- Keep one-off layout and spacing utilities directly in templ markup. Extract a
  class when it represents a reusable component or interaction pattern, not
  merely to shorten a single class attribute.
- Shared interactive classes must cover the relevant pointer, hover,
  `focus-visible`, disabled, and light/dark states so links and native controls
  behave consistently. Regenerate committed templ output after changing templ
  source.

Run Authling's own `mise` tasks from the `authling/` directory:

```sh
cd authling
mise codegen
mise test
mise test-e2e
mise build
mise authling run
```

These tasks run with `GOWORK=off` as well as in the repository workspace so
undeclared or unreleased cross-module dependencies cannot be hidden by
`go.work`. Do not add Authling tasks to the repository-root `mise.toml`;
Authling's task catalog must remain movable with the product.

Run the lowest test layer that can catch the failure, but add integration and
protocol tests when behavior crosses HTTP, OIDC, NATS, JetStream, cryptographic,
or process-lifecycle boundaries. Browser end-to-end tests use a dedicated
Authling process, Mailpit process, port range, and temporary data directory per
test; do not point them at development state or share a process across tests.
Persisted-event and recovery changes require relevant malformed-event,
historical replay or restart, OCC conflict, enumeration-resistance, and
PII/recovery-material leakage tests. Regenerate and commit derived protobuf or
templ output after changing its source. Review visible Authling browser changes
with Chrome DevTools MCP in addition to running the relevant automated tests.
