# ADR-021: Consolidate Instance and Space into a Single Server Concept

**Date:** 2026-05-04

## Context

Chatto today has a two-layer model:

- An **Instance** is a deployment — one binary, one embedded NATS, one set of users with accounts.
- An **Instance** can host many **Spaces**, each a workspace/team with its own RBAC, member list, rooms, branding, and per-space JetStream resources (`SPACE_{id}_EVENTS`, `SPACE_{id}_CONFIG`, `SPACE_{id}_RBAC`, etc.).
- **Direct messages** don't fit either layer cleanly, so they live in a synthetic hardcoded space with `spaceID = "DM"` (see ADR-015), with their own carve-outs in auth helpers, the DM RBAC engine, and the DM space lifecycle code.

This shape was inherited from an early ambition to support multi-tenant deployments where one operator could host many independent teams in a single process. In practice that lever is rarely pulled — almost every deployment runs a single space — and the cost of keeping the two-layer model is paid every day:

- **Hard to explain.** "Instance" is jargon nobody outside the project recognises. "Space" looks like Discord's "Server" but isn't quite the same thing. New users and operators have to learn a model that doesn't match anything else they've used.
- **Dual RBAC code.** Instance roles (`instance-owner`, `instance-admin`, `instance-moderator`, `everyone`) and space roles (`owner`, `admin`, `moderator`, `everyone`) run in parallel, with parallel permission scopes (`instance.*`, `space.*`, `room.*`) and parallel resolution paths. Two engines, two sets of permission constants, two GraphQL surfaces, two sets of admin UIs.
- **DM workaround leaks.** The synthetic DM space is one of the most visible places the two-layer model shows through: every space-membership check has to special-case `IsDMSpace`, DM permissions bypass the RBAC engine entirely, and the DM space appears in `ListSpaces` and gets filtered out at the GraphQL boundary.
- **Identity confusion.** A user belongs to an instance (where they have an account) but joins a space (where they get role assignments), and DMs live in a third hack-space they don't really "join". "Remove from space" vs "delete account" vs "leave DM" each behave differently.
- **Marketing weakness.** "Server" is the noun the rest of the chat space uses (Discord, Matrix homeservers, IRC). Calling our equivalent thing "Space" — a Discord-style server, but renamed — costs us recognition and gives us no compensating benefit.

The "many spaces per instance" capability has, we judge, more cost than value at the current point in the product. Operators who genuinely need to host multiple isolated communities are better served by running multiple processes (multiple servers, separate NATS, clean isolation) than by a soft-tenancy model inside one process.

## Decision

Collapse the Instance and Space layers into a single **Server** concept. The decision has three coupled parts — none of them is optional, because the consolidation only buys us the simplification we want if all three land together.

### 1. One process = one Server

The deployment is the server is the membership boundary. One binary, one embedded NATS, one set of users, one set of rooms. The "Space" concept disappears entirely from both the user-facing model and the codebase — there is no longer any nested-workspace layer to navigate, configure, or reason about.

"Server" is adopted as the user-facing noun everywhere — in URLs, UI copy, documentation, and marketing. It matches Discord's terminology, which is what the majority of the target audience already understands.

Operators who want to host multiple isolated communities run multiple servers — multiple processes, each with its own NATS, its own users, its own RBAC. We accept the small loss of the soft-tenancy lever in exchange for a sharply simpler model.

### 2. Single-layer RBAC

The dual `instance-*` / `space-*` role and permission systems collapse into one. There is one role namespace — `owner`, `admin`, `moderator`, `everyone` — and the permission scope shrinks from three (instance / space / room) to two (server / room).

The hierarchy-wins resolution model from ADR-005 carries over unchanged — it was already the right model, it just gets applied to a single role list instead of two parallel ones. `owners.emails` continues to designate config-driven server owners and continues to short-circuit the role hierarchy, exactly as it does for instance owners today.

The full table of which permissions are renamed, kept, or dropped is deferred to a follow-up ADR ([#292](https://github.com/chattocorp/chatto/issues/292)) so this ADR doesn't try to settle every individual permission decision in a single document. The shape — one namespace, two scopes, hierarchy-wins — is locked here.

### 3. DMs are regular rooms

A `type` field on rooms distinguishes `channel` (named, browseable in the room list) from `dm` (hidden from browse, listed via a DM-specific query). DMs are stored in the same buckets as channels, flow through the same event stream, use the same body / reaction / thread storage, and resolve permissions through the same RBAC engine. There is no synthetic DM space, no `SPACE_DM_*` carve-out, no parallel permission code path, no `IsDMSpace` checks scattered through auth.

`dm.view` and `dm.write` remain as global server-scoped permissions — they're toggles for whether a user can use the DM feature at all, not per-room permissions. The actual mechanics of DM rooms (deterministic IDs from sorted participants, group-DM rules, find-or-create semantics) carry over from today's model and are documented in detail in a follow-up ADR ([#293](https://github.com/chattocorp/chatto/issues/293)).

## Consequences

- **User-facing copy** changes everywhere: URLs, UI labels, settings pages, error messages, admin sections, marketing site, documentation. "Instance" and "Space" stop appearing as user-visible nouns.
- **Backend types and packages** rename: `Instance` becomes `Server`; `Space` types and operations either collapse into the server itself or are removed. Per-space iteration disappears (`ListSpaces`, `Query.spaces`, the `spaceId` argument on every room query).
- **One unified RBAC engine** replaces two. The permission resolver, the role-management code, the GraphQL `viewerCan*` resolvers, and the admin UI for roles all collapse to a single surface. We expect roughly half the code in the RBAC area to disappear.
- **DM logic merges into the regular room code path.** Read state, mention tracking, member lists, event delivery, and subscription wiring stop having a DM special case. The result is a strictly more uniform codebase.
- **`owners.emails` config stays**, with unchanged matching semantics (verified emails only). It just becomes implicitly per-server, since there's only one server per process.
- **Per-space JetStream resources collapse to per-server resources.** The `SPACE_{id}_*` proliferation goes away; the exact destination layout is decided in [#291](https://github.com/chattocorp/chatto/issues/291).
- **Backup / restore, migration tooling, and the Chatto Community cutover** are downstream consequences — Phase 4 builds the migration command and Phase 5 executes it. Existing self-hosted instances accept data loss as part of the breaking-change announcement; the Chatto Community deployment is the only one with a built migration path.
- **Soft tenancy is no longer offered.** Operators who want isolation between communities run multiple servers. We accept this; per-process isolation is simpler to reason about, simpler to scale, and stronger as a security boundary.
- **Marketing benefits**: a slightly stronger competitive moat (operators can no longer trivially spin up a single instance hosting hundreds of communities to capture a market) and a friendlier explanation story ("it's like Discord, but you can self-host one").

## Alternatives Considered

- **Keep the two layers, just rename "Space" → "Server" in UI copy.** Rejected: it fixes the marketing problem but leaves all the code, RBAC, and DM-workaround complexity in place — and arguably makes the codebase *more* confusing because the user-visible name no longer matches the internal type.
- **Keep both layers, but auto-create a single default space per instance.** Rejected: every seam in the codebase still exists, every per-space iteration still runs (with a list of one), every dual-RBAC check still happens. Pure cost, no simplification.
- **Build proper multi-tenancy (separate NATS accounts, real isolation between spaces).** Rejected: large engineering investment for a market we're not currently chasing. Per-process isolation is simpler, more secure, and operationally well-understood.
- **Consolidate Instance + Space but keep the DM hack-space.** Rejected: the synthetic DM space is one of the most visible places the two-layer model leaks. Consolidating without folding DMs into the regular room model leaves the codebase asymmetric — a DM is still "a room in a not-quite-a-space" — and undoes a meaningful share of the simplification gain.

## References

- Epic [#284](https://github.com/chattocorp/chatto/issues/284) — Consolidate Instance + Space → Server.
- Phase 1 epic [#285](https://github.com/chattocorp/chatto/issues/285) — Design & ADRs.
- Follow-up detail ADRs: [#290](https://github.com/chattocorp/chatto/issues/290) (identity & OIDC binding), [#291](https://github.com/chattocorp/chatto/issues/291) (destination schema), [#292](https://github.com/chattocorp/chatto/issues/292) (permission model details), [#293](https://github.com/chattocorp/chatto/issues/293) (DM-as-room mechanics).
- Supersedes [ADR-015](ADR-015-dms-as-hidden-space.md) (DMs as a hidden space).
- Builds on [ADR-005](ADR-005-hierarchy-wins-rbac.md) (Hierarchy-wins RBAC), whose resolution model carries over unchanged.
