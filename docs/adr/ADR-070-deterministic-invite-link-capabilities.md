# ADR-070: Derive Invitation Capabilities from Durable EVT Identity

**Date:** 2026-08-11
**Amended:** 2026-10-29
**Status:** Accepted

## Context

Invite-only account creation needs capabilities that administrators can copy
again, revoke, and redeem correctly across replicas. Invitation lifecycle
needs durable audit history. FDR-036 specifies single-use codes.

Persisting raw capabilities in `EVT` would expose them in event history and
backups. A hash alone would prevent administrators from copying a code again.
A separate mutable record would split invitation state from its audit history.

## Decision

Account admission policy is the typed admin setting `registration.admission`:
`open` or `invite_only`, with `open` as the default. Changes are durable config
facts in EVT and apply to all replicas.

Creation, redemption, and revocation remain protobuf facts in `EVT`. The
invitation projection retains each code's creation, used state, expiry, and
revocation. Every code permits exactly one account. There are no reusable
invitations, use limits, or code-version fields.

Invitation mutations use the invitation aggregate's subject-filter OCC
boundary. Account creation adds the redemption fact to the same atomic EVT
batch as the user's creation and verified sign-in factors. The whole-`EVT`
account-uniqueness guard also covers invitation changes. A concurrent change
forces the complete admission decision to retry.

Codes contain the first 96 bits of an HMAC over the public invitation ID,
encoded as 20 unpadded base32 characters in five groups of four. Validation
ignores letter case, spaces, and hyphens. Both key derivation and the HMAC
input use versioned, purpose-specific contexts. Raw codes are not stored in
`EVT`, runtime records, or logs. Authorized administrators reconstruct them.

Each replica derives a code-to-ID index from the invitation projection. IDs
are append-only, so the model rebuilds the index when the identity count
changes. Resolution is then constant-time. A detected collision fails closed.

The browser submits a code in a JSON POST body to a same-origin endpoint.
The server validates it without consumption and stores a bounded list of
admission IDs, invitation IDs, code fingerprints, and expiry times in its
signed HttpOnly session cookie, encrypted when cookie encryption is configured.
A tab keeps only its random admission selector in session
storage. New email and SSO flows select an exact cookie entry. Started flows
retain their invitation ID in existing short-lived runtime records.

A new provider identity can reach the invitation gate after authentication.
Binding a code creates a new pending capability with the same verified
identity and original expiry. The original remains until expiry so a lost
response can be retried. Each copy keeps its invitation. Identity uniqueness
and atomic account creation prevent duplicate accounts or consuming two codes
for the same identity. Linking flows are not accepted by this gate.

Code handoff through URLs is deferred.

## Consequences

Invitation history remains auditable and restorable. Backups do not contain
raw codes. Failed signup does not consume a code, and one code cannot admit
two accounts across replicas.

Changing `core.secret_key` invalidates distributed codes and browser admission
grants through their code fingerprints. Active durable records remain, and administrators can copy new codes.
Operators must keep this secret consistent across replicas and restores.

The code index adds process-local memory and retains the 96-bit namespace's
collision risk. Detected collisions fail closed.

The public administration API replaces `AdminInviteLinkService` with
`AdminInvitationCodeService`. Integrations must use the new service. No
client connection floor increase is required.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-068](ADR-068-selectable-event-mutation-consistency-boundaries.md)
