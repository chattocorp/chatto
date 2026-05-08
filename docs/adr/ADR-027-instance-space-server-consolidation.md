# ADR-027: Consolidate Instance + Space into a Single "Server" Concept

**Date:** 2026-05-04

**Status:** Accepted (phase 4 complete; phase 5 — RBAC unification — pending under #357)

**Tracking issue:** [#330](https://github.com/chattocorp/chatto/issues/330)

## Context

Chatto currently has a two-layer top-level hierarchy: an **Instance** is the deployment-wide container (one per process / NATS account), and a **Space** is a membership group within an instance. Most concerns are duplicated across both layers:

- Two parallel RBAC systems with overlapping role names (`instance-owner` vs `owner`, `instance-admin` vs `admin`, etc.) — see ADR-005 and `instance_rbac.go` / `space_rbac.go`.
- Two membership models: a user belongs to an instance _and_ separately joins each space.
- Two permission check paths in resolvers; helpers like `isConfigOwner` need to reason about both layers.
- DMs are modelled as a hidden space (ADR-015), bolted on top of the space machinery.

This is operationally redundant. In practice almost every deployment runs a single space per instance — the multi-space-per-instance flexibility is rarely exercised, but its cost is paid everywhere in the code and in the user's mental model. The terminology is also user-hostile: "instance" is alien outside the project, and "space" looks like Discord's "server" but isn't quite.

The end state in #330 is a single **Server** concept where deployment = membership boundary = RBAC root, with one unified role hierarchy (`owner` / `admin` / `moderator` / `everyone`), DMs as rooms with `type: dm`, and a single auth/permission path through the codebase.

We have public instances running with real user data, so this cannot be a flag-day rewrite. The migration must be incremental, with each step independently shippable and reversible.

## Decision

Migrate toward the unified **Server** model in small, independently mergeable PRs, using a **config-designated primary space** as the load-bearing bridge.

### The primary space bridge

Introduce a server-side configuration value (env var / `chatto.toml` setting) that names exactly one space within the instance as the **primary space**. Conceptually this primary space _is_ the future Server; everything else is legacy.

- New deployments auto-create a primary space at bootstrap and the config points at it.
- Existing deployments set the config to their existing single space (or, for the rare multi-space instance, pick one and treat the others as legacy until manually consolidated or dropped).
- The config is **temporary** — its only purpose is to let us route code paths through "the one space that matters" without committing to schema changes yet. It will be removed at the end of the migration when Instance and Space have collapsed into a single Server entity.

This bridge lets every subsequent step be a small, local refactor: each resolver, store, or UI surface that currently reasons about "the instance" or "all spaces" can be rewritten to reason about "the primary space," one at a time, with the legacy code paths kept alive until the last consumer is gone.

### Migration phases (indicative, not contractual)

Sequencing is decided per-PR; this is the rough order. The guiding principle is **behaviour-first, data-last**: change how the API, frontend, and product behave through the bridge config while the underlying schema, streams, and KV buckets stay exactly as they are today. Only once the codebase has settled into the Server shape do we touch the data layout, and only after _that_ do we simplify RBAC/membership.

1. **Bridge in place.** Add the primary-space config (env var + `chatto.toml`), bootstrap behaviour for new instances (auto-create one space and point at it), and backfill behaviour for existing ones (point at the existing single space; for multi-space deployments, pick one and leave the rest as legacy). No semantic changes yet — every existing code path still works.
2. **Rework API and frontend UX around the Server concept.** Anything user-facing or API-facing that currently splits instance from space gets reframed as a single layer routed through the primary space: GraphQL schema additions/aliases, frontend stores, navigation, settings pages, admin views, "this deployment" surfaces. Underlying storage and the legacy multi-space code paths stay intact behind the new surface.
3. **DMs as rooms in the API/frontend.** Reframe DMs (currently a hidden space, ADR-015) as rooms with `type: dm` inside the Server at the API and UI level. The DM space's storage stays untouched at this stage — only how the API exposes them and how the frontend renders them changes.
4. **Schema/data migration.** With the API and UX already behaving as if Server exists, rename streams and KV buckets to their Server-shaped equivalents and write a one-shot migration that copies data from the primary space's streams/buckets (and the DM hidden space) into the new layout. Legacy spaces that aren't the primary either get dropped, archived, or merged per a documented operator policy. The primary-space config is removed at the end of this phase — its job is done.
5. **Consolidate and simplify membership and RBAC.** With a single Server and a single set of streams/buckets, collapse the instance-level and space-level role/membership systems into one. Remove the compatibility shims, the dual permission paths, and the `instance_*` vs `space_*` split in core. (Hierarchy-wins resolution from ADR-005 stays; only the two-layer split goes away.)

Every PR contributing to this work references #330 and updates this ADR if it invalidates an assumption.

### What we're explicitly _not_ doing

- **No big-bang migration.** No single PR rewrites both layers at once.
- **No data-layout changes until the API and UX have already settled into the Server shape.** Stream names, KV bucket names, and storage layout stay exactly as they are today through phases 1–3. This keeps the early phases trivially revertible and means we only run the data migration once we know the target shape is correct.
- **No early RBAC/membership simplification.** RBAC and membership stay dual-layer through phases 1–4 even though it's tempting to clean them up sooner. Doing it before the data migration would mean re-doing it after, and the dual-layer code is the safety net that keeps the legacy paths working during migration.
- **No removal of the multi-space-per-instance code paths until they have zero callers.** The legacy paths stay alive and tested until the data migration in phase 4 retires them.
- **No support for multi-primary or pooled-tenant deployments.** One process = one server stays a hard invariant (per #330's "what stays the same").

## Consequences

- **Phases 1–3 are trivially revertible.** They only add new surfaces and reroute reads/writes through the bridge; the underlying streams, KV buckets, and legacy code paths stay byte-identical to today. Any PR in this range can be reverted without data loss or migration.
- **Phase 4 is the one-way door.** Renaming streams/buckets and copying data is the irreversible step. By the time we run it, the API and UX have already proven out the Server shape end-to-end on real data, so the migration is mechanical rather than exploratory.
- **Operators get a clear migration story.** "Set this one config value, point it at your existing space, keep running. When phase 4 ships, run `chatto migrate` once on upgrade." Multi-space operators get an explicit decision point at phase 4 (pick one, archive, or merge per the documented policy).
- **The primary-space config is debt by design.** It exists only to enable the migration and is deleted at the end of phase 4. Leaving it in would re-introduce a two-layer model under a different name.
- **Sustained duplication during phases 1–4.** Both instance-level and space-level code paths live in the tree simultaneously, and RBAC/membership stay dual-layer until phase 5. This is accepted; the alternative is doing the data migration and the RBAC simplification at the same time, which is exactly the flag-day risk we're trying to avoid.
- **DMs cross the bridge twice.** Phase 3 reframes them at the API/UX level while their storage stays as the hidden DM space; phase 4 migrates that storage into the unified Server layout. Splitting it this way means the disruptive UX change and the disruptive data change never land in the same PR.
- **Documentation churn.** `.claude/rules/admin.md`, `authorization.md`, and several feature docs reference "instance" and "space" as distinct concepts. They will be updated phase-by-phase as the underlying behaviour changes, not pre-emptively.
- **#330 supersedes parts of ADR-015 in the long run.** ADR-015's "DMs as a hidden space" decision is retired once phase 4 completes; the consequences listed there (hardcoded permissions, `IsDMSpace` special cases) dissolve into normal room logic during phase 5's RBAC simplification.
