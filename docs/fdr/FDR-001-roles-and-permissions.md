# FDR-001: Roles & Permissions (RBAC)

**Status:** Active
**Last reviewed:** 2026-05-31

## Overview

Chatto controls who can do what through role-based access control. Every authenticated user holds one or more server roles; each role grants or denies specific permissions. Permissions can also be overridden per room-group and per room, giving operators fine-grained control without inventing parallel role systems.

## Behavior

- Every authenticated user belongs to the implicit `everyone` role and may additionally hold one or more named server roles.
- The system roles, highest rank first, are `owner`, `admin`, `moderator`, `everyone`. Custom roles can be created and positioned anywhere between `moderator` and `everyone`.
- A role grants or denies named permissions like `message.post`, `room.create`, `admin.view-users`.
- Permission grants/denies can be configured at three scopes: per-server (the role default), per room-group, and per room. The most specific scope wins.
- Permissions gate capabilities, not every form of visibility. For example, DM read access comes from room membership, while `message.post` gates starting DMs and sending root DM messages.
- Server admins can drag-and-drop to reorder custom roles. System roles are fixed in rank.
- Owners pass every permission check because the `owner` role is seeded with every server-scope permission — not because the resolver special-cases them. Owners are not above the rules; they hold the rules.
- Operators can designate owners via `owners.emails` in `chatto.toml`. Matching users are auto-assigned the `owner` role when their email is verified, and already-verified matching users are assigned the role on server boot.

## Design Decisions

### 1. Flat, single-tier role layout

**Decision:** One server-wide role layer. No separate "instance roles" vs "space roles".
**Why:** The earlier two-tier split duplicated concepts and made permission resolution unpredictable. Collapsing into one tier with per-room-group / per-room overrides gives equivalent flexibility with one mental model. See ADR-027 and ADR-030.
**Tradeoff:** Operators who liked per-space role ownership now configure that through room-group overrides instead.

### 2. Hierarchy-wins resolution

**Decision:** Roles are walked from highest rank to lowest. The first explicit allow/deny wins; lower-ranked roles aren't consulted further.
**Why:** This is the only model that makes patterns like "everyone denied `message.post`, moderator granted it" produce the intuitive result (announcement channels). A pure deny-wins or allow-wins model would force operators to invent workarounds. See ADR-005.
**Tradeoff:** Denying a permission on `everyone` does NOT block higher-ranked roles. Operators have to learn to attach denies at the right rank, and tests need to deny on the user's actual highest role to verify blocking.

### 3. Three permission scopes (server / group / room)

**Decision:** Permissions resolve room → group → server. The most specific scope wins.
**Why:** Operators want both "system-wide defaults" and "this one channel works differently" without modelling them as separate role systems. See ADR-031.
**Tradeoff:** A given permission decision now checks up to three scopes per role. This is acceptable because current RBAC state is kept in an in-memory projection.

### 4. Owner privileges materialize as role grants, not bypass

**Decision:** Owners aren't a special case in the resolver — the `owner` role just has every server-scope permission granted.
**Why:** Operators who deny a permission expect it to be denied uniformly. A "owners bypass everything" short-circuit would silently violate that expectation and complicate audit. See ADR-005.
**Tradeoff:** A misconfigured deny on the `owner` role can lock owners out. Mitigated by `chatto reset rbac`, which restores defaults.

### 5. Config-designated owners materialize as real role assignments

**Decision:** `owners.emails` in `chatto.toml` materializes durable `owner` role assignments, rather than being checked at permission time. Verification applies the role immediately for newly verified users; server boot applies it to already-verified matching users after config changes.
**Why:** Avoids a config-vs-role drift class of bug. Once assigned, the role is the source of truth. Fresh deployments work without restart because verification triggers the assignment, and retroactive config changes need only a process restart.
**Tradeoff:** Removing an email from `owners.emails` doesn't automatically demote that user — operators must revoke the role explicitly. This is intentional: removing the config shouldn't silently change live authorization.

### 6. Rank gates target-user mutations, in addition to permissions

**Decision:** Mutations that target another user (rename, role assignment, profile edits) require both the relevant permission **and** that the actor outrank the target.
**Why:** Otherwise a rogue moderator with `role.assign` could rename the owner. Permission asks "can this role do X at all?"; rank asks "does the actor outrank this specific target?". Both are needed.
**Tradeoff:** Two-step checks are more code than a single permission lookup, and easy to forget when adding new mutations. Helpers (`requireUserAdminTarget`, `requireUserPermissionTarget`) exist to keep call sites uniform.

### 7. RBAC state is event-sourced

**Decision:** Role definitions, role order, assignments, and explicit permission decisions are durable events, with reads served from an in-memory RBAC projection.
**Why:** This aligns RBAC with the rest of Chatto's event-sourced migration and makes authorization reads rebuildable from the deployment event log. See ADR-033 and ADR-035.
**Tradeoff:** Writes must append events and wait for local projection catch-up before returning, so mutation paths need optimistic concurrency handling instead of direct state writes.

## Permissions

The full permission catalog is in `cli/internal/core/permission.go`. Key permissions that gate RBAC management itself:

- `role.manage` — create, edit, delete roles and the permissions attached to them.
- `role.assign` — assign roles to users.
- `admin.access`, `admin.view-users`, `admin.view-system`, `admin.view-audit` — gate access to the admin UI and its sub-views.
- `message.post` — post root messages in rooms and start DMs. Reading DMs is not permission-gated; it follows room membership.

## Related

- **ADRs:** ADR-004 (authorization at API boundary), ADR-005 (hierarchy-wins RBAC), ADR-027 (instance/space consolidation), ADR-030 (space tier retirement), ADR-031 (room-group-centric ACL), ADR-033 (event-sourced state), ADR-035 (per-aggregate migration), ADR-037 (DM access via membership)
- **FDRs:** Every FDR that mentions a permission depends on this one.
