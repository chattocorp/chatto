# FDR-021: Admin Dashboard & System Monitoring

**Status:** Active
**Last reviewed:** 2026-06-20

## Overview

The admin section gives owners and admins visibility into the server's operational state: user counts, room counts, storage resource usage, projection health, and audit/event-log diagnostics. It deliberately exposes operational metadata — never message content, never per-user activity logs, never per-room conversation summaries.

## Behavior

- The admin UI lives under `/chat/[serverId]/server-admin/`. Non-admins see an "access denied" panel; the server-header gear entry point is hidden from them.
- Admin-capable users enter through the gear icon in the server name pane header. Once inside server-admin, the server sidebar switches from room navigation to the admin section navigation with a Back to Server affordance.
- **Users page** — paginated list of all server members with login, email, roles, verification status. Admins can edit profiles, assign roles, suspend, or delete users when they hold the relevant permission.
- **System Info page** — owner-only page showing backing message-broker connection status, storage account limits and current usage, stream/consumer health, projection health (lag, entry counts, and rough memory estimates), and `admin.systemInfo.stats` (user count, channel room count, DM room count).
- **Audit log page** — chronological diagnostic event-log view for forensic review, grouped by event creation date. The list view uses `admin.eventLog`; the detail view uses `admin.eventLogEntry` to show the raw payload JSON for human inspection.
- The audit log UI can be filtered by exact event type and exact actor ID. Event type suggestions come from `admin.eventLogEventTypes`; the actor field reuses the server member lookup but still accepts synthetic actor IDs such as `system:bootstrap`. The GraphQL filter also supports inclusive created-at bounds for API callers, but the server-admin page does not expose time-range controls.
- The audit/event-log GraphQL connection returns `totalCount` as `Int64` because it reflects retained stream message counts, which can exceed GraphQL's 32-bit `Int` range on long-running servers.
- Filtered audit-log browsing is a bounded diagnostic scan over retained EVT rows, not an indexed analytics query. The connection reports `scannedCount`, `scanLimit`, and `scanLimited` so the UI can tell operators when older matches may exist beyond the inspected window.

## Design Decisions

### 1. Capability-based admin entry

**Decision:** There is no separate `admin.access` permission. The admin UI is visible when the viewer has at least one concrete admin capability, while child routes and GraphQL fields enforce their own narrower gates such as `server.manage`, `admin.view-users`, `admin.view-audit`, `role.manage`, and owner-only diagnostics.
**Why:** Some operators want a "read-only admin" role that can investigate without making changes; some want users-but-not-system access for a customer-support persona. Tiered permissions let those roles be expressed without inventing parallel role systems.
**Tradeoff:** There is no standalone "can see the admin dashboard" bit. The dashboard is a capability index, so the UI derives visibility from the concrete permissions the viewer holds.

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

### 5. Diagnostic values are operator tooling, not product contracts

**Decision:** Raw storage subjects, stream/consumer names, payload JSON, projection metric names, and memory estimates are documented as diagnostic values. The GraphQL fields are intentional operator APIs, but clients should not parse those raw values as stable product-domain data.
**Why:** Operators need visibility into what the runtime is doing, especially during the 0.1 stabilization lane. At the same time, these values reflect storage and projection implementation details that may evolve as the event-sourcing model settles.
**Tradeoff:** Third-party admin clients can display diagnostics but should treat raw strings and JSON as best-effort inspection data. If a future integration needs a stable audit export format, it should get a dedicated schema instead of depending on diagnostic payloads.

### 6. Event-log filters are bounded diagnostic scans

**Decision:** `admin.eventLog(filter:)` supports exact event-type and actor-ID matching plus inclusive created-at bounds, but filtered reads scan at most 5,000 retained EVT rows per request. The server-admin UI currently exposes event-type and actor filters and groups the newest-first table by creation date.
**Why:** EVT is the source of truth, not an indexed analytics store. The filters make the admin page useful for common investigations without adding a second durable index or allowing one request to walk an unbounded stream.
**Tradeoff:** A sparse filter on a large server may report `scanLimited: true` before finding every historical match. Operators can narrow the time range or inspect older windows explicitly; a future export/analytics feature should get a dedicated read model.

### 7. Nested `admin` resolver with field-specific capability gates

**Decision:** Admin queries are grouped under a nested `Query.admin` type that returns for authenticated viewers, while sensitive fields check their own capabilities (`server.manage`, `admin.view-users`, `admin.view-system`, `admin.view-audit`, `role.manage`, owner-only diagnostics) before returning data.
**Why:** The nested shape gives the API one obvious admin-tooling namespace, and the field-level checks let operators delegate user, system, audit, and RBAC-editor visibility independently.
**Tradeoff:** A user may be able to enter the admin area but see permission denials or empty panels for specific sections. The UI has to reflect that capability split clearly.

## Permissions

- `admin.view-users` — gates user-management views, admin-only affordances, and user-sensitive fields such as other users' verified email addresses and login cooldowns. The underlying `server.members` directory query remains authenticated-user visible; see FDR-025.
- `admin.view-system` — gates `admin.projections`; `admin.systemInfo` is owner-only for now.
- `admin.view-audit` — gates `admin.eventLog`, `admin.eventLogEventTypes`, and `admin.eventLogEntry`.
- `role.assign` — gates user edits and role changes via the `requireUserAdminTarget` helper.

## Related

- **ADRs:** ADR-001 (NATS JetStream as primary data store), ADR-033 (event-sourced state with projections), ADR-034 (single event stream), ADR-036 (runtime state in `RUNTIME_STATE`)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle), FDR-020 (Server Branding & Configuration), FDR-022 (User Profile), FDR-024 (Permission Inspection Tool), FDR-025 (User Search & Member Directory)

## Open Questions

- A more sensitive operator-only surface for raw storage inspection or content moderation would need its own permission and audit model. Not currently planned.
