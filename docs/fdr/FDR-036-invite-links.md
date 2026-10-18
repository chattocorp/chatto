# FDR-036: Server Invites

**Status:** Active
**Last reviewed:** 2026-10-18

## Overview

Invite links let operators restrict new account creation without tying
admission to email. Members with creation permission can create reusable links
with optional expiry and use limits, copy their links again, and revoke them. The
feature applies consistently to direct email/password registration and
external-provider auto-provisioning while leaving sign-in to existing accounts
unchanged.

## Behavior

- Operators choose an `open` or `invite_only` account-creation policy in server
  configuration. `open` is the default.
- In `invite_only` mode, every new self-service account requires a valid
  invite link. Direct registration still verifies the submitted email, and
  external providers still verify their own identity; an invite link proves
  admission rather than identity.
- Existing accounts can always sign in or link another sign-in method without
  an invite link. Accounts created through operator tooling also bypass
  invite-link admission.
- An invite link can have a maximum number of uses, an expiry time, both, or
  neither. A link with neither remains valid until revoked.
- The UI calls this feature **Server invites**. Its permissions have a separate
  section in permission matrices; their IDs remain `invite-link.create` and
  `invite-link.manage`.
- Members with `invite-link.create` use **Create server invite** at the top of
  the normal server sidebar or from the **Server invites** management page.
  Both actions open the same creation dialog outside management.
  The dialog defaults to one use and seven days. Creators can select other
  limits, unlimited uses, or no expiry. Creation does not require shield mode.
- The management page labels the list **Your server invites** or **All server
  invites**. If creation is not permitted, it explains the required permission.
  Without shield mode, members
  see only their own links. They can see status and use count, copy a link
  again, and revoke an active link without a permission.
- With effective `invite-link.manage` and shield mode, members can manage all
  links and see their creators. This permission does not permit creation.
- Removing creation permission stops new links but does not invalidate existing
  links or remove the creator's right to manage them.
- Revoked, expired, exhausted, malformed, and unknown invite links all fail
  with the same public-facing unavailable-link result.
- Opening an invite link while the server is open does not consume it.
- Invite-link use and account creation succeed together. A failed account
  creation does not consume a use, and concurrent signups cannot exceed a use
  limit.
- Changing the server secret invalidates previously shared invite links.
  Active link records remain visible, and authorized viewers can copy newly
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

### 2. Invite links are identity-provider neutral

**Decision:** One invite-link mechanism gates both direct registration and
external-provider auto-provisioning, including providers that do not supply an
email address.
**Why:** Admission and identity proof are separate concerns. A shareable link
works for every supported account-creation path and does not make email a
hidden requirement for SSO.
**Tradeoff:** Email-targeted delivery and recipient restrictions are deferred.

### 3. Invite links remain recoverable

**Decision:** Creators and callers with effective `invite-link.manage` can
copy an invite link again after creation.
**Why:** Invite links are routinely shared through several channels, and a
show-once secret creates unnecessary recovery and support friction.
**Tradeoff:** Global managers can obtain every active link. This permission
requires shield mode. Ownership alone exposes only the creator's links.

### 4. Constraints and redemption are durable facts

**Decision:** Creation, redemption, and revocation are retained as durable
history, with expiry and exhaustion derived from those facts and current time.
**Why:** Use limits are correctness rules across replicas, and operators need
an auditable explanation of an invite link's state. This follows ADR-033.
**Tradeoff:** Invite-link reads require an event projection rather than a simple
mutable row.

### 5. Links use compact deterministic capabilities

**Decision:** Each link uses a fixed 16-character URL-safe opaque capability
deterministically derived from its public invitation ID with a
purpose-separated server-secret key. The bearer token is not stored as an
event.
**Why:** Administrators can reconstruct the same link without persisting a
bearer capability in event history and backups. Purpose separation prevents
the root secret's other uses from sharing this signing domain.
**Tradeoff:** Rotating the server secret invalidates links that have already
been shared, even though their durable records remain active. Resolving the
opaque capability also requires a derived lookup over projected invitations.

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

**Decision:** Both invite-link permissions belong to the default administrator
role on fresh servers. Neither belongs to `everyone`. Existing roles are not
rewritten; operators must grant the new permissions explicitly. The retired
`user.invite` permission grants no authority and does not prevent role assignment
or revocation. Its historical decisions remain stored for upgrade detection.
**Why:** The split changes permission meaning and removes the shield requirement
from creation. Operators must review this change under ADR-113.
**Tradeoff:** A non-owner administrator on an upgraded server may need an owner
to grant the new permissions. Existing links and creator ownership survive.

## Permissions

- `invite-link.create` — create links without shield mode.
- `invite-link.manage` — read, copy, and revoke anyone's links with shield mode.
- Ownership — read, copy, and revoke your own links without either permission.

## Related

- **ADRs:** ADR-033, ADR-036, ADR-040, ADR-045, ADR-068, ADR-070, ADR-113
- **FDRs:** FDR-001, FDR-018, FDR-020, FDR-023, FDR-028, FDR-031
