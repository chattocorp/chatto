# FDR-024: Permission Inspection Tool

**Status:** Active
**Last reviewed:** 2026-05-19 (scope label rename to SERVER/GROUP/ROOM)

## Overview

Admins can inspect why a specific user has (or doesn't have) a permission, at server or room scope. The tool surfaces a trace of every grant and deny the resolver considered, in the order it considered them, with the winning decision highlighted. It's the only way to debug "why can this user post in #announcements?" without reading code.

## Behavior

- The admin UI exposes a "Permission Explainer" tool: pick a user, optionally pick a room, pick a permission, and see the full resolution trace.
- The trace lists every (subject, scope) entry the resolver evaluated. The first entry that produced an allow or deny is marked as the winning decision; subsequent entries show what the resolver *would* have looked at next if the winning entry hadn't fired.
- Each trace entry shows: the subject (a role name, or "user" for user-level overrides), the scope (server / room group / room / user), the decided state (allow / deny / none), and whether this is the entry that won.
- If no role or override produced a decision, the resulting state is "none" — which the API boundary treats as deny by default.

## Design Decisions

### 1. Trace, not just final decision

**Decision:** The tool returns the entire ordered list of considered entries, not just the boolean outcome.
**Why:** "Did the resolver allow this?" is a question the resolver itself answers. "Why?" requires showing the decision path so operators can spot misconfigurations — e.g., "this user gets `message.post` because their custom role has it granted at server scope, even though we denied it on `everyone`". A boolean wouldn't help debug a misconfig.
**Tradeoff:** Bigger response payloads. Acceptable for an admin tool that's used sparingly.

### 2. Admin-only, no self-inspection

**Decision:** The query requires `admin.access` (and `role.manage` at the room/group scope when inspecting per-room permissions). Regular users can't run the inspector even against themselves.
**Why:** The trace would leak which roles a user holds and the structure of the permission tree. Useful information to a malicious actor probing what they're up against. Restricting to admins keeps that surface inside the trust boundary.
**Tradeoff:** Users who legitimately wonder "why can't I post here?" have to ask an admin. Acceptable; the failure mode is rare and the leak is real.

### 3. Probe-resistant error responses

**Decision:** When inspecting a room scope, a missing or inaccessible room returns `ErrPermissionDenied`, not a 404 "room not found".
**Why:** A 404 leaks the existence-or-not of room IDs to anyone with access to the inspector. The permission-denied response is identical for "no such room" and "you can't see this room", so admins can't accidentally enumerate rooms via the tool.
**Tradeoff:** Slightly more confusing UX in the rare case where the admin actually mistyped a room ID. The admin UI suppresses this by populating the room ID from a dropdown of rooms they can see.

### 4. Same resolver code path as runtime permission checks

**Decision:** The explainer calls `core.PermResolver().ExplainAllPermissions(...)` — the same resolver used in production permission checks, but in a mode that records each step instead of short-circuiting.
**Why:** A separate "documentation" version of the resolver would drift from the real one. Anytime the real resolver gets a new short-circuit, optimization, or scope, the documentation version would be wrong until someone remembered to update it. Shared code = the trace is always the truth.
**Tradeoff:** The resolver has to support a "trace mode" that adds branching. The branching is small and well-isolated.

### 5. Trace surfaces scope tokens that match the UI

**Decision:** Trace entries label scopes as `SERVER`, `GROUP`, and `ROOM`, mirroring what the admin UI displays and the `PermissionScope` constants used by the resolver.
**Why:** Operators reading the trace shouldn't have to translate between internal vocabulary (e.g., "phase 2 of the resolver", "object ID 'any'") and what they see in the admin UI. The labels match the buttons.
**Tradeoff:** None — the GraphQL `PermissionLevel` enum, the Go `Level*` constants, and the inspector UI use the same three tokens.

## Permissions

- `admin.access` — server-scope inspection.
- `admin.access` or `role.manage` — room-scope inspection.

## Related

- **ADRs:** ADR-005 (hierarchy-wins RBAC), ADR-031 (room-group-centric ACL)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-017 (Room Groups & Sidebar Layout), FDR-021 (Admin Dashboard & System Monitoring)
