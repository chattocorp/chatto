# ADR-070: Derive Invitation Capabilities from Durable EVT Identity

**Date:** 2026-08-11

**Status:** Accepted

## Context

Invite-only account creation needs shareable invitation links that can be
copied again, limited by use count or expiry, revoked, and redeemed correctly
when several Chatto replicas serve signups concurrently. Invitation lifecycle
and usage also need durable audit history.

Persisting a raw invitation code in `EVT` would put a live bearer capability in
event history and backups. Persisting only a one-way hash avoids that exposure
but makes an existing link impossible to recover, forcing administrators into
a show-once workflow. A mutable latest-value record would make counters
convenient but would split the source of truth from the required audit history.

The admission policy itself has different ownership. It determines whether
self-service account creation is available at deployment time, before an
administrator can necessarily use the application, and resembles configured
authentication methods more than a mutable domain resource.

## Decision

Account admission policy is static server configuration with two values:
`open` and `invite_only`. `open` is the compatibility default. Every serving
replica must use the same configured value; operators upgrade all replicas
before enabling `invite_only` during a rolling deployment.

Invitation creation, constraints, redemption, and revocation are protobuf
facts in the `EVT` stream. A dedicated invitation projection derives active,
expired, exhausted, and revoked state and the current use count. Mutations use
the invitation aggregate's subject-filter OCC boundary. Account creation adds
the redemption fact to the same atomic EVT batch as the new user's durable
creation and verified sign-in-factor facts. The existing whole-`EVT`
account-uniqueness OCC boundary also covers the invitation tail, so any
concurrent redemption forces the complete admission decision to retry.

An invitation code is a versioned signed capability containing the public
invitation ID and an HMAC signature. Chatto derives a purpose-specific signing
key from `[core].secret_key` and a fixed invitation-code context before signing
the versioned payload. The code is verified in constant time and never stored
in `EVT`, `RUNTIME_STATE`, logs, or audit metadata. Authorized administrators
can reproduce the same code from the invitation ID.

Changing `[core].secret_key` intentionally invalidates all previously shared
invitation capabilities, alongside the other server-secret-derived runtime
artifacts. It does not rewrite or revoke durable invitation aggregates; after
rotation, an administrator can copy a newly derived link for any invitation
that is otherwise still active.

## Consequences

Invitation definitions and usage remain auditable, restorable domain history,
while backups do not contain directly usable invitation links. The same link
can be recovered without separate encrypted secret storage.

Use limits remain correct across replicas, and failed account creation cannot
consume a use. The signup implementation is more coupled to the atomic EVT
batch boundary because it must commit user and invitation facts together.

The signing format needs an explicit version and stable derivation context so
future formats or key separation changes can coexist. Operators must treat
`[core].secret_key` as stable shared deployment state and understand that its
rotation invalidates already-distributed links.

Static admission policy is simple to bootstrap and operate, but changing it
requires configuration rollout rather than an in-application toggle. Mixed
old/new server replicas must not serve traffic with invite-only enabled.

The public API additions are additive. New clients interpret an absent or
unknown discovery policy from older servers as `open`; older clients do not
understand invite-only registration and therefore cannot create accounts on a
server that enables it.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-068](ADR-068-selectable-event-mutation-consistency-boundaries.md)
