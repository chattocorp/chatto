# FDR-021: Admin Dashboard & System Monitoring

**Status:** Active
**Last reviewed:** 2026-06-06

## Overview

The admin section gives owners and admins visibility into the server's operational state: user counts, room counts, NATS/JetStream resource usage, and audit logs. It deliberately exposes only aggregate operational data — never message content, never per-user activity logs, never per-room conversation summaries.

## Behavior

- The admin UI lives under `/chat/[serverId]/server-admin/`. Non-admins see an "access denied" panel; the link is hidden from the chat header for them.
- **Users page** — paginated list of all server members with login, email, roles, verification status. Admins can edit profiles, assign roles, suspend, or delete users (subject to outranking the target — see FDR-001).
- **System Info page** — shows NATS connection status (server ID, version, round-trip latency), JetStream account limits and current usage, stream/consumer health, projection health (lag, entry counts, and rough memory estimates), and `ServerStats` (user count, channel room count, DM room count).
- **Audit log page** — chronological list of significant admin actions (user deletions, role changes, server config edits, etc.) for forensic review.
- The audit/event-log GraphQL connection returns `totalCount` as `Int64` because it reflects retained stream message counts, which can exceed GraphQL's 32-bit `Int` range on long-running servers.

## Design Decisions

### 1. Tiered admin permissions

**Decision:** Admin access is gated by four permissions: `admin.access` (enter the admin UI at all), `admin.view-users`, `admin.view-system`, `admin.view-audit`.
**Why:** Some operators want a "read-only admin" role that can investigate without making changes; some want users-but-not-system access for a customer-support persona. Tiered permissions let those roles be expressed without inventing parallel role systems.
**Tradeoff:** Four permission strings instead of one. The default `admin` role grants all four, so out-of-the-box behavior matches expectations.

### 2. Operational metadata, not conversation content

**Decision:** The System Info page can expose operational metadata such as stream/consumer state and projection diagnostics, but not message bodies, file contents, per-user activity trails, or per-room conversation summaries.
**Why:** Operators need enough detail to diagnose lag, storage pressure, and projection growth. Those are system-health questions, not moderation questions. Keeping content and behavioral surveillance out of the admin dashboard preserves the privacy boundary while still making the server operable.
**Tradeoff:** Some identifiers and subject filters are visible to admins with system access. That is acceptable for the operator persona, but any future content-level moderation surface should be a separate, explicit feature.

### 3. Privacy boundary: admins see metadata, not content

**Decision:** Admins can see who exists, what rooms exist, who's a member, who has which roles. They cannot see message content, private messages, file contents, or user passwords.
**Why:** "Operate the system" and "read user conversations" are different jobs. Conflating them would mean every operator needs the trust level of a moderator with access to every conversation. Keeping the boundary explicit lets owners hire operators without granting message visibility.
**Tradeoff:** Moderation tools that need to read content (rare cases) would need a separate, auditable feature with explicit consent. None exists today.

### 4. Live data, not cached

**Decision:** System Info fetches fresh data from NATS and projection diagnostics on every page load. No caching layer.
**Why:** The data is fundamentally point-in-time ("how much storage are we using right now?"). Caching would mean stale numbers shown to operators making capacity decisions. The fetch cost is low because NATS already has the data internally.
**Tradeoff:** Refreshing the page hits NATS every time. Not a concern at admin-usage volume.

### 5. Nested `admin` resolver with field-specific capability gates

**Decision:** Admin queries are grouped under a nested `Query.admin` type gated by `admin.access`, while sensitive fields still check their narrower capabilities (`admin.view-users`, `admin.view-system`, `admin.view-audit`) before returning data.
**Why:** The nested shape gives the UI one obvious admin boundary, and the field-level checks let operators delegate user, system, and audit visibility independently.
**Tradeoff:** A user may be able to enter the admin area but see permission denials or empty panels for specific sections. The UI has to reflect that capability split clearly.

## Permissions

- `admin.access` — gates entry to the admin UI and the `Query.admin` resolver.
- `admin.view-users` — gates `admin.users` and `admin.members` queries.
- `admin.view-system` — gates `admin.systemInfo`, `admin.projections`, and `admin.stats`.
- `admin.view-audit` — gates `admin.auditLog`.
- `role.assign` — gates user edits and role changes via the `requireUserAdminTarget` helper (permission + outrank-target check).

## Related

- **ADRs:** ADR-001 (NATS JetStream as primary data store), ADR-033 (event-sourced state with projections), ADR-034 (single event stream), ADR-036 (runtime state in `RUNTIME_STATE`)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle), FDR-020 (Server Branding & Configuration), FDR-022 (User Profile), FDR-024 (Permission Inspection Tool), FDR-025 (User Search & Member Directory)

## Open Questions

- A more sensitive operator-only surface for raw storage inspection or content moderation would need its own permission and audit model. Not currently planned.
