# FDR-036: Server Invitations

**Status:** Active
**Last reviewed:** 2026-08-11

## Overview

Server invitations let operators restrict new account creation without tying
admission to email. Administrators can create reusable invitation links with
optional expiry and use limits, copy them again later, and revoke them. The
feature applies consistently to direct email/password registration and
external-provider auto-provisioning while leaving sign-in to existing accounts
unchanged.

## Behavior

- Operators choose an `open` or `invite_only` account-creation policy in server
  configuration. `open` is the default.
- In `invite_only` mode, every new self-service account requires a valid
  invitation. Direct registration still verifies the submitted email, and
  external providers still verify their own identity; an invitation proves
  admission rather than identity.
- Existing accounts can always sign in or link another sign-in method without
  an invitation. Accounts created through operator tooling also bypass
  invitation admission.
- An invitation can have a maximum number of uses, an expiry time, both, or
  neither. An invitation with neither remains valid until revoked.
- The administration interface defaults new invitations to one use and seven
  days. Administrators can deliberately select unlimited uses or no expiry.
- Administrators can list invitations, see their current status and use count,
  copy the same invitation link again, and revoke an active invitation.
- Revoked, expired, exhausted, malformed, and unknown invitations all fail with
  the same public-facing invalid-invitation result.
- Supplying an invitation while the server is open does not consume it.
- Invitation use and account creation succeed together. A failed account
  creation does not consume a use, and concurrent signups cannot exceed a use
  limit.
- Changing the server secret invalidates previously shared invitation links.
  Active invitation records remain visible, and administrators can copy newly
  derived links for them.

## Design Decisions

### 1. Admission policy is static operator configuration

**Decision:** The initial policy is configured as `open` or `invite_only` by
the server operator and takes effect when replicas load configuration.
**Why:** Admission policy is deployment posture, like enabled authentication
methods, and operators must be able to establish it before any administrator
account or UI is available.
**Tradeoff:** Administrators cannot switch policy from the application, and
operators must keep the value consistent across replicas.

### 2. Invitations are identity-provider neutral

**Decision:** One invitation mechanism gates both direct registration and
external-provider auto-provisioning, including providers that do not supply an
email address.
**Why:** Admission and identity proof are separate concerns. A generic code
works for every supported account-creation path and does not make email a
hidden requirement for SSO.
**Tradeoff:** Email-targeted delivery and recipient restrictions are deferred.

### 3. Invitation links remain recoverable

**Decision:** Administrators with `invite.manage` can copy an invitation link
again after creation.
**Why:** Invitation links are routinely shared through several channels, and a
show-once secret creates unnecessary recovery and support friction.
**Tradeoff:** Anyone who gains invitation-management access can obtain every
active invitation link, so the permission is intentionally broad and
security-sensitive.

### 4. Constraints and redemption are durable facts

**Decision:** Creation, redemption, and revocation are retained as durable
history, with expiry and exhaustion derived from those facts and current time.
**Why:** Use limits are correctness rules across replicas, and operators need
an auditable explanation of an invitation's state. This follows ADR-033.
**Tradeoff:** Invitation reads require an event projection rather than a simple
mutable row.

### 5. Codes are deterministic signed capabilities

**Decision:** A link carries a public invitation ID plus a versioned,
purpose-separated signature derived from the server secret. The raw code is
not stored as an event.
**Why:** Administrators can reconstruct the same link without persisting a
bearer capability in event history and backups. Purpose separation prevents
the root secret's other uses from sharing this signing domain.
**Tradeoff:** Rotating the server secret invalidates links that have already
been shared, even though their invitation records remain active.

### 6. Redemption shares the account-creation commit

**Decision:** The invitation redemption fact and the new account's durable
creation facts are committed atomically under the existing whole-`EVT`
account-uniqueness guard, which also covers concurrent invitation changes.
**Why:** Reserving or consuming an invitation earlier can burn a use when a
later signup step fails. Consuming it later can exceed a use limit under
concurrent replicas. The shared commit satisfies both invariants.
**Tradeoff:** Account-creation paths must assemble their durable verified
factor before publishing rather than adding it through a later best-effort
write.

### 7. Existing installations do not receive a permission backfill

**Decision:** `invite.manage` belongs to the default administrator role on new
servers. Existing custom/default roles are not rewritten automatically;
effective owners retain their normal override and may grant it.
**Why:** Chatto does not mutate operator-managed RBAC policy when new
permissions are introduced. This matches FDR-001.
**Tradeoff:** A non-owner administrator on an upgraded server may need an owner
to grant the new permission.

## Permissions

- `invite.manage` — list, create, copy, and revoke server invitations.

## Related

- **ADRs:** ADR-033, ADR-036, ADR-040, ADR-045, ADR-068, ADR-070
- **FDRs:** FDR-001, FDR-018, FDR-020, FDR-023, FDR-028, FDR-031
