# FDR-024: Permission Inspection Tool

**Status:** Active
**Last reviewed:** 2026-08-27

## Overview

Administrative API clients can inspect why a specific user has or does not
have each permission at server or room scope. The response surfaces the
direct-user, named-role, and `everyone` decisions that contribute to the check,
with the winning decision highlighted. Chatto does not currently include a
bundled permission-explainer UI.

## Behavior

- `AdminPermissionService.ExplainPermissions` accepts a target user and an
  optional room. It returns every permission that applies at that scope.
- The ConnectRPC permission-inspection API keeps inspection in the RBAC
  administration namespace and requires `role.manage`.
- The trace lists the nearest applicable (subject, scope) entry for the direct user and each assigned named role. It also includes the nearest `everyone` baseline entry.
- Denies win across direct-user and named-role entries. A named/direct allow wins over an `everyone` deny only at the same or a nearer scope; otherwise the nearer baseline row is marked as winning.
- Each trace entry shows: the subject (a role name, or "user" for user-level overrides), the scope (server / room group / room / user), the decided state (allow / deny / none), and whether this is the entry that won.
- If no role or override produced a decision, the resulting state is "none" — which the API boundary treats as deny by default.

## Design Decisions

### 1. Trace, not just final decision

**Decision:** The tool returns the complete set of effective subject entries, not just the boolean outcome. Less-specific entries shadowed by the same subject's nearer decision are omitted; the `everyone` baseline is retained so operators can see whether its scope applied.
**Why:** "Did the resolver allow this?" is a question the resolver itself answers. "Why?" requires showing the decision path so operators can spot misconfigurations — e.g., "this user gets `message.post` because their custom role has it granted at server scope, even though we denied it on `everyone`". A boolean wouldn't help debug a misconfig.
**Tradeoff:** Bigger response payloads. Acceptable for an admin tool that's used sparingly.

### 2. Admin-only, no self-inspection

**Decision:** Every explanation requires RBAC-editor authority (`role.manage`).
The target cannot inspect itself through this administrative operation.
**Why:** The trace would leak which roles a user holds and the structure of the permission tree. Useful information to a malicious actor probing what they're up against. Restricting to admins keeps that surface inside the trust boundary.
**Tradeoff:** A delegated user-permission editor who lacks `role.manage` cannot
inspect the full resolver trace. Users who need an explanation must ask an
RBAC editor.

### 3. Probe-resistant error responses

**Decision:** When inspecting a room scope, a missing or inaccessible room returns `ErrPermissionDenied`, not a 404 "room not found".
**Why:** A 404 leaks the existence-or-not of room IDs to anyone with access to the inspector. The permission-denied response is identical for "no such room" and "you can't see this room", so admins can't accidentally enumerate rooms via the tool.
**Tradeoff:** API clients cannot distinguish a mistyped room ID from an
inaccessible room. They should select room IDs from an authorized room list.

### 4. Same resolver code path as runtime permission checks

**Decision:** The explainer calls `core.PermResolver().ExplainAllPermissions(...)` — the same resolver used in production permission checks, but in a mode that records each step instead of short-circuiting.
**Why:** A separate "documentation" version of the resolver would drift from the real one. Anytime the real resolver gets a new short-circuit, optimization, or scope, the documentation version would be wrong until someone remembered to update it. Shared code = the trace is always the truth.
**Tradeoff:** The resolver has to support a "trace mode" that adds branching. The branching is small and well-isolated.

### 5. Trace surfaces stable scope tokens

**Decision:** Trace entries label scopes as `SERVER`, `GROUP`, and `ROOM`,
matching the public API's permission-level vocabulary.
**Why:** API clients should not have to translate internal resolver phases or
storage identifiers into operator-facing scope names.
**Tradeoff:** A future UI must use the same vocabulary or translate these
stable API values explicitly.

## Permissions

- `role.manage` — server-scope and room-scope inspection.

## Related

- **ADRs:** ADR-031 (room-group-centric ACL), ADR-040 (permission-only RBAC with owner override), ADR-052 (subject-specific RBAC with an everyone baseline)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-017 (Room Groups & Sidebar Layout), FDR-021 (Admin Dashboard & System Monitoring)
