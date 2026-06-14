# FDR-028: User Suspension

**Status:** Active
**Last reviewed:** 2026-06-08

## Overview

User suspension is a server-level moderation tool that temporarily prevents a user from interacting with the server and other users without deleting their account or ending their sessions. A suspended user can still authenticate and view their own suspension status, but permission-gated interaction and administration actions are denied through RBAC.

## Behavior

- Moderators with `user.suspend` can suspend or unsuspend lower-ranked users. Self-suspension is denied.
- A suspension has a required moderator reason, a creation time, the moderator actor, the target user, and an optional expiry.
- Expired suspensions stop applying automatically. No separate unsuspend fact is written for natural expiry.
- Unsuspending writes an explicit moderation fact and clears the active suspension projection.
- Suspended users remain able to log in. Existing sessions are not terminated.
- Suspended users see a banner that shows whether the suspension is indefinite or when it expires. The moderator reason is not shown to the suspended user.
- Admin moderation surfaces show active suspensions with target, moderator, reason, creation time, and expiry.
- Suspension preserves roles, room memberships, DMs, settings, messages, and account data.
- v1 only blocks behavior that is already permission-gated. Self-service actions without an RBAC gate remain available unless they depend on a denied permission.

## Design Decisions

### 1. Suspension is durable moderation history

**Decision:** Suspension and unsuspension are event-sourced moderation facts, with reads served from an active-suspension projection.
**Why:** Operators need an auditable record of who suspended whom, why, and for how long. Projection-based expiry gives the runtime a current answer without writing synthetic events when time passes.
**Tradeoff:** Expired suspensions remain in the event log as history but are absent from active moderation lists.

### 2. RBAC-shaped enforcement via a protected virtual role

**Decision:** An active suspension applies a protected system role internally named `suspended`, rendered as `@suspended`.
**Why:** Suspension is a policy state: "deny these interaction permissions while active." Representing it as a role keeps enforcement understandable in the same vocabulary as the rest of authorization.
**Tradeoff:** The role is not a normal assignment. UI must show it as protected and implicit, and assignment APIs must reject manual assignment.

### 3. Suspension denies run before ordinary grants

**Decision:** Explicit denies from the active `@suspended` role are checked before user-level grants and normal role grants.
**Why:** Chatto's RBAC resolver intentionally uses hierarchy-wins, and user-level grants normally outrank roles. Suspension needs to be stronger than an old ad-hoc grant or an owner/admin role grant, otherwise it would be unreliable.
**Tradeoff:** `@suspended` is a special preflight only for explicit denies. Permissions not denied by the suspended role continue through normal RBAC.

### 4. Suspension is not account deletion

**Decision:** Suspension does not shred keys, remove content, revoke all credentials, or erase user data.
**Why:** Suspension is reversible moderation. Deletion is irreversible account lifecycle with crypto-shredding concerns and needs a separate feature design.
**Tradeoff:** Suspended users may still access non-interaction self-service features unless those features are permission-gated.

### 5. User-facing privacy boundary

**Decision:** The suspended viewer sees expiry only; the moderation reason is shown only in admin/moderation surfaces.
**Why:** Reasons can include operator notes or reports. The user needs to know the account state and duration, not the internal audit text.
**Tradeoff:** Operators may need separate communication channels for user-facing explanation.

## Permissions

- `user.suspend` — suspend or unsuspend a lower-ranked user. Granted to owner and admin by default.

The default `@suspended` role explicitly denies current interaction/admin permissions such as room create/list/join/manage/ban, message post/thread/react/echo/manage, server/manage, role/manage, role/assign, admin view permissions, `user.suspend`, and `user.delete-any`.

## Related

- **ADRs:** ADR-004 (authorization at API boundary), ADR-005 (hierarchy-wins RBAC), ADR-033 (event-sourced state), ADR-035 (per-aggregate migration)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle), FDR-021 (Admin Dashboard), FDR-023 (Authentication & Sessions), FDR-025 (User Search & Member Directory)
