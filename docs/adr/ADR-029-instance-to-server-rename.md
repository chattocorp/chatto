# ADR-029: Rename `Instance` → `Server` across the codebase

- **Status**: Accepted
- **Date**: 2026-05-11
- **Related**: [ADR-025: Multi-instance Client Architecture](ADR-025-multi-instance-client-architecture.md), [ADR-027: Instance/Space/Server Consolidation](ADR-027-instance-space-server-consolidation.md)

## Context

Chatto's user-facing terminology has been "server" for a long time — admin UI strings, marketing pages, docs, and ADR-027 all use it. The codebase still says "instance" everywhere, a leftover from the very early days when each Chatto deployment was framed as an "instance" of the app.

The split has bitten us repeatedly:

- Code reviewers ask "is `Instance` the server, the deployment, or the multi-instance client concept?".
- New contributors mentally translate at every read.
- Documentation contradicts code (`docs/ARCHITECTURE.md` describes a "server" that the code calls `Instance`).
- ADR-027's narrative finished the conceptual collapse (instance/space → server); this PR finishes the naming.

## Decision

Rename `Instance` → `Server` in identifiers, types, GraphQL surface, proto messages/fields, frontend modules and routes, and user-facing strings. Persisted-shape names (JetStream stream names, KV bucket names) **stay as-is** for this PR — they're an implementation detail with real migration cost, and the in-code variables that reference them can be renamed independently.

### Naming rule

Drop the prefix entirely when there's nothing left to disambiguate against (single role tier, single admin list); keep `Server` when the prefix carries meaning by paralleling `Room`/`Space` at the same layer.

| Drop prefix | Keep `Server` prefix |
|---|---|
| `CreateInstanceRole` → `CreateRole` | `IsInstanceAdmin` → `IsServerAdmin` (parallels `IsRoomAdmin` etc.) |
| `AssignInstanceRole` → `AssignRole` | `HasInstancePermission` → `HasServerPermission` (parallels `HasSpacePermission`, `HasRoomPermission`) |
| `RevokeInstanceRole` → `RevokeRole` | `GrantInstancePermission` → `GrantServerPermission` (parallels `GrantRoomPermission`) |
| `ListInstanceRoles` → `ListRoles` | `InstanceEvent` → `ServerEvent` (later consolidated into GraphQL `Event`) |
| `GetInstanceRole` → `GetRole` | `InstanceConfig` → `ServerConfig` (top-level type) |
| `GetInstanceRolePermissions` → `GetRolePermissions` | |
| `ReorderInstanceRoles` → `ReorderRoles` | |
| `ListInstanceAdmins` → `ListAdmins` | |

### Scope decisions

1. **Persisted names stay**: JetStream streams (`SERVER_EVENTS`), KV buckets (`KV_INSTANCE*`), `chatto.toml` keys. In-code variables that reference them may be renamed independently. Real migration deferred to a separate PR.
2. **GraphQL hard rename**: schema breaks. Per early-stage policy (`.claude/rules/current-status.md`), external consumers re-codegen. Soft deprecation rejected as needless ceremony for this stage.
3. **Proto field-number stability**: message/field names rename; field numbers stay. Binary wire compat preserved — existing stored payloads decode unchanged. Verified at the end via a backup/restore round-trip.
4. **`live.*` NATS subjects in scope**: `live.instance.user.{userId}.*` → `live.server.user.{userId}.*`, `live.instance.space.{spaceId}.*` → `live.server.space.{spaceId}.*`. Live events don't persist, so subject rename is free.
5. **Frontend module path**: `$lib/instance/*` → `$lib/chatto/*` (not `$lib/server/*` — SvelteKit reserves that name for server-only modules).
6. **Routes**: the admin tree is already consolidated under `chat/[instanceId]/(chrome)/server-admin/*` (no `/admin/instance/*` exists). What remains is the `[instanceId]` route param everywhere under `chat/` and the top-level `/instances` and `/instances/callback` paths. These rename to `[serverId]` and `/servers` / `/servers/callback`, with SvelteKit redirects from the old paths for ~one release cycle.

## Consequences

### Positive

- One word for one concept across code, schema, and docs.
- New contributors don't have to learn the historical naming.
- Documentation can finally say what it means.
- Permission ops gain naming symmetry: `GrantServerPermission` / `GrantRoomPermission` / (future) `GrantSpacePermission` all read the same.

### Negative / risks

- Big PR — ~100+ files, several thousand lines moved. Mitigated by phased ordering (proto → GraphQL → Go → live subjects → frontend → docs) and re-running all codegen between phases.
- GraphQL break: any external playground user re-codegens. Acceptable per early-stage policy.
- urql cache churn from renamed operations across a deploy. Tabs open through deploy may see a brief refetch storm; transient and acceptable.
- Route redirects: `[instanceId]`-shaped bookmarks become `[serverId]`. SvelteKit redirects cover known shapes; deep links fixed by URL share.

### Deferred

- KV bucket and stream renames. They're a separate PR with a real migration command — track separately.
- `chatto.toml` config key renames (same reason).

## Alternatives considered

- **"Leave it; just docs"**: rejected. Every contributor pays the translation tax. The current `Instance` codename actively misleads new readers because the user-visible term has been "server" for many months.
- **Soft deprecation across two PRs**: rejected. Early-stage policy makes the ceremony pure waste; no third-party clients to coordinate with.
- **Bigger PR that also renames persisted shapes**: rejected. Mixing the cosmetic rename with the data-migration rename inflates blast radius without aiding either.
