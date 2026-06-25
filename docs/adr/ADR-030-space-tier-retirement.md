# ADR-030: Retire the Space tier

- **Status**: Accepted
- **Date**: 2026-05-11
- **Related**: [ADR-027: Instance/Space/Server Consolidation](ADR-027-instance-space-server-consolidation.md), [ADR-029: Rename `Instance` → `Server`](ADR-029-instance-to-server-rename.md), [ADR-015: DMs as a hidden space](ADR-015-dms-as-hidden-space.md)

## Context

ADR-027 collapsed the historical three-tier model (Instance → Space → Room) into two tiers (Server → Room) at the conceptual and user-facing layers. ADR-029 finished the cosmetic rename of `Instance` → `Server` across identifiers, GraphQL, NATS subjects, frontend modules, and docs.

The Space tier is therefore *behaviourally* retired, but its mechanical residue is still load-bearing in four places:

1. **A vestigial primary-space record with stale readers.** Every deployment has one `Space` proto stored in `INSTANCE` KV at key `space.{spaceId}` with fields `id`, `name`, `description`. The branding (logo, banner) and the canonical server name/description already live in `INSTANCE_CONFIG` (`ServerConfig` proto + separate `instance.logo`/`instance.banner` keys). Four code paths still read from the Space record anyway (line numbers omitted — these files have since shifted):
   - `cli/internal/core/dm.go` — bootstrap creates a synthetic DM Space record
   - `cli/internal/graph/mutation.resolvers.go` — explicit "until PR(c)" dual-write of name/description
   - `cli/internal/http_server/opengraph.go` — OG metadata reads `space.Name` / `space.Description` (stale; `ServerConfig` is the right source)
   - `cli/internal/graph/space_helpers.go` — thin GraphQL wrapper
   These are dead-end reads of stale data. Once they're removed, the persisted `space.{spaceId}` KV record becomes an orphan and can be left alone — one tiny entry per server, zero functional impact.

2. **`spaceID` plumbing on the core API.** Roughly 80 functions across `cli/internal/core/*.go` still take a `spaceID string` parameter. Every one of them either ignores the value or feeds it into a legacy compatibility mapping, which exists only to map the legacy wire value `space_id = "DM"` to `"dm"` and everything else to `"channel"`. The parameter is a one-bit DM flag dressed up as an ID.

3. **DMs still have legacy hidden-space residue at the wire boundary.** ADR-015's "hidden DM space" predates the room-`kind` discriminator. With the `kind` field now baked into KV keys and NATS subjects, the DM scope is determined by `kind == "dm"` directly; the old `space_id = "DM"` value only survives where persisted payloads or compatibility APIs still carry a `space_id` field.

4. **Proto layer is partially renamed.** The durable server-membership deletion event has been corrected to `ServerMemberDeletedEvent` while preserving field 320. Live deployment-scoped config changes use `ServerUpdatedEvent`; unused `ServerCreatedEvent` / `ServerDeletedEvent` live proto messages were removed during the 0.1 protobuf cleanup. `SpaceUserPreferences` (used by notification levels) and the bare `Space` / `SpaceMembership` messages share the remaining naming legacy.

The cost of leaving this in place is paid every time someone reads the code: a new contributor sees a `spaceID` parameter and assumes the codebase is multi-space, which it isn't. The cost of removing it is one focused refactor.

The full per-space KV bucket family (`SPACE_{spaceId}_BODIES`, `SPACE_{spaceId}_REACTIONS`, etc.) referenced in some proto comments **does not exist** in current code — those buckets were already collapsed into `SERVER_*` by the Phase-4 migration (#354). So this ADR is not gated on a data migration.

## Decision

Retire the Space tier as a code-level concept. The persisted `space.{spaceId}` KV record stays where it is as an orphan (one tiny entry per deployment — nothing reads it after this ADR's Phase 1).

### Naming rule

When a function previously took `spaceID` and the only thing it actually needed was the room's kind, the new signature drops `spaceID` and either:

- takes a `kind RoomKind` (or `kind string` for now, pending a typed enum), if the kind is what the caller wants to express; or
- drops the parameter entirely if the function only needs the room ID (the kind is derivable from the room record).

The DM sentinel is retired as a product/storage concept in favour of an explicit `kind == "dm"` check at call sites. A small `RoomKindFromLegacySpaceID` / `LegacySpaceIDForRoomKind` mapping may remain only at wire-format boundaries where persisted payloads or compatibility APIs still expose `space_id`.

### Scope

In-scope for this ADR:

1. **Drop the four readers of the `space.{spaceId}` KV record.** Replace each with the canonical `ServerConfig` read (server name / description) or eliminate the call:
   - `dm.go` — DM bootstrap no longer needs a Space record.
   - `mutation.resolvers.go` — the explicit "dual-write until PR(c)" comment is now PR(c); drop the dual-write.
   - `opengraph.go` — read `ConfigManager().GetEffectiveInstanceName` / `GetEffectiveDescription` instead.
   - `space_helpers.go` GraphQL wrapper — delete with the rest of the surface.
2. **Collapse `spaceID` → `kind` (or drop) across `cli/internal/core/*.go`.** Mechanical refactor; behaviour-preserving. Tests update at the same time.
3. **Delete `cli/internal/core/spaces.go`, the `Space` Go type, `SpaceMembership` proto message, and `Server.primarySpaceId` GraphQL bridge field** once nothing reads them. The 1070-line `spaces.go` disappears.
4. **Retire legacy live deployment-scoped proto residue.** Durable `corev1.Event` tags 1030–1032 are reserved as retired live-only variants; the current live envelope keeps only the emitted `ServerUpdatedEvent` and reserves removed lifecycle names/tags.
5. **Rename `live.server.space.{spaceId}.>` NATS subjects** to `live.server.{eventType}` (or another deployment-scoped pattern — to be decided in Phase 1). Live subjects have no persistence; rename freely.
6. **Rename `SpaceUserPreferences` → `UserPreferences`** in proto + storage key naming. Same wire-format-safe argument (preferences are a small KV-stored proto, not in JetStream).
7. **Frontend rename `$lib/state/space/*` → `$lib/state/server/*` (or merge in).** Cosmetic; the store is "the active server's room/permissions state", not "a space's".
8. **Update `docs/fdr/`, `docs/ARCHITECTURE.md`, and the relevant `AGENTS.md` files to drop residual "space" prose** that survived ADR-029's docs sweep (those described what the code looked like at the time; this ADR makes them stale).

Out of scope (deferred):

- **`space.{spaceId}` KV record**: stays as orphan. One small entry per deployment, no readers post-Phase-1.
- **`ServerMemberDeletedEvent` proto message** (`corev1.Event` field 320): renamed during the 0.1 protobuf cleanup while preserving the field number. No 0.1 servers had been deployed yet, and 0.0 import remains field-number based, so the source name could still be corrected before beta.
- **`space_id` fields on other persisted event payloads** (e.g. message events): stay for the same reason — wire format must decode existing stored events.
- **`KV_INSTANCE*` bucket names, `instance.logo`/`instance.banner` KV keys, `/api/instance` REST endpoint**: stay. Already covered by ADR-029. The public LiveKit config key was later renamed from `livekit.instance_id` to `livekit.server_id`, with the old name retained as a deprecated alias for existing configs.

### Phases (PR boundaries)

| Phase | Scope | Risk | Approx. size |
|---|---|---|---|
| 1 | Drop the four readers of `space.{spaceId}` KV (opengraph, mutation dual-write, dm init, GraphQL wrapper). Replace with `ServerConfig` reads where needed. | Low | ~4 files |
| 2 | Collapse `spaceID` → `kind` (or drop) across core + tests. | Medium (mechanical but wide) | ~80 signatures, ~15 files |
| 3 | Delete `spaces.go`, `Space` / `SpaceMembership` proto messages, `Server.primarySpaceId` GraphQL field. Retire legacy live deployment-scoped proto residue and rename `SpaceUserPreferences` to its un-prefixed counterpart. Rename `live.server.space.>` subjects. | Low-medium | ~1100 line net deletion + targeted proto/subject renames |
| 4 | Frontend `$lib/state/space/` → `$lib/state/server/` import sweep. | Low | ~5 files + ~15 importers |
| 5 | Docs and rules cleanup (stale "space" prose). | Trivial | Small targeted edits |

Each phase is shippable independently. Phases 1 and 2 are good candidates to combine. Phase 3 is the biggest single landing because deleting `spaces.go` cascades into the proto + subject renames cleanly.

## Consequences

### Positive

- The core Go API stops lying about its shape: signatures reflect what data they actually depend on.
- New contributors don't have to learn the Space tier just to find it's not real.
- `spaces.go` (1070 lines) goes away. The `Space` Go type and `Server.primarySpaceId` GraphQL bridge field go away.
- The DM mechanism gets simpler: normal room code uses `kind == "dm"` directly, while the legacy `space_id` conversion is boxed into explicitly named compatibility helpers.
- ADR-015 ("DMs as a hidden space") was already marked superseded by ADR-027 at the storage layer. This ADR finishes the job at the API layer.

### Negative / risks

- Phase 2 touches ~80 function signatures in core. Mechanical, but caller fan-out is wide; the PR will be large. Mitigated by holding the diff to one mechanical pattern per call site (rename or drop, no semantic changes).
- The persisted `space.{spaceId}` KV record remains as orphan data on every deployment. Cost: ~a hundred bytes per server. Accepted.
- Tests that construct primary spaces via `CreateSpace(...)` need to be migrated to the new bootstrap path. Risk of test churn proportional to the spread of the helper.
- Retiring live event proto residue is safe at the wire level, but every Go callsite that constructs the event needs updating in the same PR. The emitted set is small (`ServerUpdatedEvent` publishers in `server_branding.go`).

### Deferred

- Persisted shapes that can't be renamed without a JetStream migration: `space_id` fields on persisted event payloads and the orphan `space.{spaceId}` KV record. Same reasoning as ADR-029: cosmetic gain not worth a stream rewrite. The event variant name was corrected to `ServerMemberDeletedEvent` before 0.1 deployment while keeping proto field 320 stable; unused live server lifecycle messages were removed from `LiveEvent`.

## Alternatives considered

- **Leave it alone.** Defensible — nothing is actively broken. Rejected because the gap between "what the code shape suggests" and "what the code actually does" is the largest remaining one in the codebase, and every contributor pays the translation tax.
- **Rip out the persisted Space record too.** Rejected: requires a data migration with no functional payoff. The orphaned KV record is one tiny entry per server.
- **Rename `space_id` proto fields and `live.server.space.>` subjects.** Rejected: persisted bytes are stored under those names. Field-number stability and subject naming are documented carve-outs from ADR-029.
- **Keep `spaceID` in signatures, rename to `_legacySpaceID` with a comment.** Rejected as worst of both worlds — same translation tax, more visible eyesore.
