# ADR-070: Derive Invite-Link Capabilities from Durable EVT Identity

**Date:** 2026-08-11

**Status:** Accepted

## Context

Invite-only account creation needs shareable invite links that can be
copied again, limited by use count or expiry, revoked, and redeemed correctly
when several Chatto replicas serve signups concurrently. Invitation lifecycle
and usage also need durable audit history.

Persisting a raw invite-link token in `EVT` would put a live bearer capability in
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

An invite link uses `/invite/{token}`, where `token` is a fixed 16-character
URL-safe opaque capability. It is the first 96 bits of an HMAC over Chatto's
existing public invitation ID, encoded as 16 unpadded base64url characters.
Every character therefore contributes to the token's 96 bits of secret
unpredictability instead of carrying a separately visible identifier.

Chatto derives a purpose-specific signing key from `[core].secret_key` and a
fixed, versioned invite-link context before signing the invitation ID. The
token is never stored in `EVT`, `RUNTIME_STATE`, logs, or audit metadata.
Authorized administrators can reproduce the same full link from the invitation
ID.

Each replica maintains a process-local token-to-invitation index derived from
the invitation projection. Invitation identities are append-only, so the model
rebuilds the index when the projected identity count changes. Token resolution
then remains constant-time without turning the bearer capability into durable
state. A collision in the 96-bit truncated output fails validation closed
rather than selecting either invitation.

The `/invite/{token}` HTTP entry point validates the capability, stores only
the invitation ID in the signed browser session, and redirects immediately to
`/register?invited=1`. Invalid links redirect to the same generic registration
error. Responses are non-cacheable and suppress referrers, and request logging
records `/invite/:token` rather than the bearer value. Direct registration and
external-provider auto-provisioning consume the session-bound invitation ID;
there is no separate manual-entry flow.

The application cannot redact an upstream reverse proxy's or CDN's access
logs. Operators must configure those layers to replace the suffix of
`/invite/*` with a placeholder. This is the unavoidable operational tradeoff
of placing a directly usable bearer capability in a conventional link path.

Changing `[core].secret_key` intentionally invalidates all previously shared
invite-link capabilities, alongside the other server-secret-derived runtime
artifacts. It does not rewrite or revoke durable invitation aggregates; after
rotation, an administrator can copy a newly derived link for any invitation
that is otherwise still active.

## Consequences

Invite-link definitions and usage remain auditable, restorable domain history,
while backups do not contain directly usable links. The same link
can be recovered without separate encrypted secret storage.

Use limits remain correct across replicas, and failed account creation cannot
consume a use. The signup implementation is more coupled to the atomic EVT
batch boundary because it must commit user and invitation facts together.

The compact signing format fixes its current 96-bit length. Its versioned
derivation context allows a future implementation to derive and index more
than one format during a migration without spending path characters on an
explicit version. Operators must treat
`[core].secret_key` as stable shared deployment state and understand that its
rotation invalidates already-distributed links.

The shortened capability adds a small process-local lookup index and accepts
the birthday-bound collision risk of a 96-bit namespace. A detected collision
fails closed. Even at one million durable invitation records, the approximate
chance of any collision remains about 6 in a quintillion.

Conventional paths are easier to share than fragment-based handoffs, but every
HTTP intermediary sees the bearer path. Chatto redacts its own request and
internal-error logs and returns no-store, no-referrer, and noindex directives;
deployment-owned access logging needs equivalent redaction.

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
