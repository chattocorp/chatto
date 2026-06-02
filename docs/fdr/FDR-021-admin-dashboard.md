# FDR-021: Admin Dashboard & System Monitoring

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

The admin section gives owners and admins visibility into the server's operational state: user counts, room counts, NATS/JetStream resource usage, and audit logs. It deliberately exposes only aggregate operational data — never message content, never per-user activity logs, never per-room conversation summaries.

## Behavior

- The admin UI lives under `/chat/admin/`. Non-admins see an "access denied" panel; the link is hidden from the chat header for them.
- **Users page** — paginated list of all server members with login, email, roles, verification status. Admins can edit profiles, assign roles, suspend, or delete users (subject to outranking the target — see FDR-001).
- **System Info page** — shows NATS connection status (server ID, version, round-trip latency), JetStream account limits and current usage (memory, storage, stream count, consumer count), and `ServerStats` (user count, channel room count, DM room count).
- **Audit log page** — chronological list of significant admin actions (user deletions, role changes, server config edits, etc.) for forensic review.

## Design Decisions

### 1. Tiered admin permissions

**Decision:** Admin access is gated by four permissions: `admin.access` (enter the admin UI at all), `admin.view-users`, `admin.view-system`, `admin.view-audit`.
**Why:** Some operators want a "read-only admin" role that can investigate without making changes; some want users-but-not-system access for a customer-support persona. Tiered permissions let those roles be expressed without inventing parallel role systems.
**Tradeoff:** Four permission strings instead of one. The default `admin` role grants all four, so out-of-the-box behavior matches expectations.

### 2. Aggregate metrics only — no per-stream / per-bucket breakdowns

**Decision:** The System Info page shows totals (overall stream count, overall memory usage, etc.) but not per-stream or per-bucket figures. Stream names, bucket names, and object-store identifiers are deliberately omitted.
**Why:** Stream and bucket names embed room IDs and user IDs in many places. Exposing them would leak structural metadata that operators don't need for capacity planning and that a malicious admin could correlate against. Aggregates serve the operational need without the leak.
**Tradeoff:** Debugging a specific bucket's growth requires direct NATS access via `nats` CLI. Acceptable for the operator persona that already has shell access for this kind of work.

### 3. Privacy boundary: admins see metadata, not content

**Decision:** Admins can see who exists, what rooms exist, who's a member, who has which roles. They cannot see message content, private messages, file contents, or user passwords.
**Why:** "Operate the system" and "read user conversations" are different jobs. Conflating them would mean every operator needs the trust level of a moderator with access to every conversation. Keeping the boundary explicit lets owners hire operators without granting message visibility.
**Tradeoff:** Moderation tools that need to read content (rare cases) would need a separate, auditable feature with explicit consent. None exists today.

### 4. Live data, not cached

**Decision:** System Info fetches fresh data from NATS on every page load. No caching layer.
**Why:** The data is fundamentally point-in-time ("how much storage are we using right now?"). Caching would mean stale numbers shown to operators making capacity decisions. The fetch cost is low because NATS already has the data internally.
**Tradeoff:** Refreshing the page hits NATS every time. Not a concern at admin-usage volume.

### 5. Nested `admin` resolver, single auth check at the root

**Decision:** Admin queries are a nested `Query.admin` type that returns `nil` for non-admins. Fields under it (`users`, `members`, `systemInfo`, `auditLog`) don't need individual auth checks.
**Why:** Without the nested shape, every admin field would need its own permission check — easy to forget when adding new ones, and easy to skew between fields. One gate at the root makes the boundary impossible to misplace.
**Tradeoff:** A non-admin querying `admin { systemInfo }` gets back `null` rather than a permission error. The frontend has to differentiate "no admin access" from "admin access but no data"; the convention is clear and documented.

## Permissions

- `admin.access` — gates entry to the admin UI and the `Query.admin` resolver.
- `admin.view-users` — gates `admin.users` and `admin.members` queries.
- `admin.view-system` — gates `admin.systemInfo` and `admin.stats`.
- `admin.view-audit` — gates `admin.auditLog`.
- `role.assign` — gates user edits and role changes via the `requireUserAdminTarget` helper (permission + outrank-target check).

## Related

- **ADRs:** ADR-006 (KV as source of truth, streams as audit logs)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle), FDR-020 (Server Branding & Configuration), FDR-022 (User Profile), FDR-024 (Permission Inspection Tool), FDR-025 (User Search & Member Directory)

## Open Questions

- An "operator-only" debugging surface that surfaces per-stream / per-bucket data behind a separate, more sensitive permission could help diagnose capacity issues without exposing structural metadata to all admins. Not currently planned.
