# Instructions for Agents Working in `cli/`

This file covers backend code: Go services, ConnectRPC, NATS/JetStream,
authorization, live events, backup/restore, and backend tests.

## Non-Negotiables

- Chatto is multi-replica software. Never rely on process-local serialization
  for correctness.
- NATS JetStream/KV is the primary data store. Use JetStream OCC or KV
  `Create`/revision `Update` for uniqueness and cross-replica invariants.
- Durable domain state belongs in `EVT`; latest-value runtime state belongs in
  `RUNTIME_STATE` only when it is truly runtime/latest-value state.
- Services own their domain state and projections. Do not bypass service
  boundaries to poke JetStream, KV, or projections from unrelated code.
- Do not log PII. Use opaque IDs, counts, booleans, event names, and safe hashes.

## Architecture Touchpoints

- `cli/internal/core` is domain logic and service/projection code.
- `cli/internal/connectapi` is the protobuf/ConnectRPC API.
- `proto/chatto/core/v1` holds persisted/internal protobufs.
- `proto/chatto/api/v1` holds public ConnectRPC API protobufs.
- `docs/ARCHITECTURE.md`, FDRs, and ADRs should move with architectural changes.

## Public APIs

- Public RPC API surface lives in ConnectRPC/protobuf or the planned wire
  protocol.
- Keep ConnectRPC transport thin: authenticate, decode, map errors/responses,
  and delegate policy/domain work to shared services.
- Put operation-specific authorization in the core operation model for that
  behavior. Low-level `ChattoCore` helpers are not public transport entry
  points and may assume their caller already performed the appropriate gate.
- REST endpoints are acceptable for OAuth callbacks, webhooks, health checks,
  uploaded assets, and unauthenticated discovery such as `GET /api/server`.
- `GET /api/server` is compatibility-sensitive. Preserve URL, CORS behavior,
  required JSON fields, and OAuth discovery fields unless there is a rollout
  plan.

## Event-Sourced State And NATS

- `EVT` is the durable event-sourced stream. `SERVER_EVENTS` is historical and
  should not receive new runtime writes or live delivery paths.
- `RUNTIME_STATE` stores sessions, auth/workflow tokens, notification state,
  push subscriptions, cached previews, wrapped DEK records, and similar
  latest-value runtime data.
- Projection-backed decisions need OCC tokens for the same event-log prefix as
  the projected state. Do not decide from a projection and publish against an
  unrelated stream tail.
- Subject/key shapes are part of the storage contract. When changing them,
  update constructors, parsers, tests, architecture docs, and e2e coverage.
- For mixed records in one stream or KV bucket, encode discriminators in the key
  prefix so reads can filter by subject/prefix without deserializing everything.

## Live Events

- Durable facts publish to `evt.>` through `EventPublisher`; JetStream republish
  exposes committed facts on `live.evt.>`.
- Transient UI sync publishes `corev1.LiveEvent` on `live.sync.>` through
  `publishLiveEvent`.
- Pick one delivery path per conceptual update. Do not double-publish both a
  durable event and a transient live event for the same UI change.
- Do not publish from projector `Apply` methods; every replica runs projectors.
- `StreamMyEvents` is the authorized gate for realtime delivery. It waits for
  projection readiness and filters per subscriber before publishing events.
- New live event types usually require protobuf, publishing, authorization,
  realtime mapping, frontend subscription handling, and tests. If a visible room
  timeline event is added, update the Connect timeline assembler and mapping
  tests.

## Authorization And RBAC

- Core authorization source of truth lives around `cli/internal/core/permissions.go`,
  `permission_resolver.go`, `can.go`, and FDR-001/ADR-040.
- Users are server-scoped. Spaces and rooms may be discoverable, but room
  message access requires room membership.
- Permission resolution for non-owners is deny-wins, then allow-if-any, then
  default deny. Effective owners bypass normal permission decisions.
- Effective owner means durable `owner` role or verified email matching
  `owners.emails`.
- DM rooms have an explicit privacy boundary; owners/admins/moderators do not
  get moderation visibility into DM contents.
- Permission strings use exactly `{object}.{verb}` with hyphenated verbs:
  `room.ban-member`, `message.post-in-thread`, `admin.view-users`.
- Add permissions in Go first, regenerate frontend mirrors, and test scope and
  DM-boundary behavior.
- Targeted operations are permission-gated, not rank-gated: role assignment uses
  `role.assign`, direct user permissions use `user.manage-permissions`, room
  bans use `room.ban-member`.

## Admin Interface

- Owners/admins can see operational metadata, not user content. Message/file
  visibility for moderation must be an explicit audited feature.
- Server admin routes live under `/chat/[serverId]/server-admin/`.
- The shared admin `Panel` component is used in both server-admin and settings
  surfaces; changes affect both.
- Implicit roles such as `everyone` must not be editable as normal assignments.

## Attachment URL Authorization

- Stable asset URLs use `/assets/files/{assetId}` and image transform variants.
- Browser-facing ConnectRPC attachment URL fields append a signed per-user
  `access` ticket and expose expiry in the API asset URL object.
- The ticket is the browser capability: it carries asset/user/expiry/transform
  claims and is accepted without cookies or bearer headers.
- Asset serving still checks that the signed user remains a member of the asset's
  room, so kick/leave revokes future fetches.
- URLs are per-user and intentionally not shared/CDN-cacheable. Treat leaked URLs
  as usable until expiry or membership loss.
- Legacy locator URLs remain for compatibility; new browser fields should prefer
  stable `/assets/files/...` plus `access`.

## Backup, Restore, And Keys

- `chatto backup` snapshots JetStream streams/KV and writes a manifested
  `.tar.gz`, optionally age-encrypted.
- `chatto restore` restores snapshots into embedded or external NATS and supports
  conflict modes `error`, `skip`, and `overwrite`.
- `KV_ENCRYPTION_KEYS`/KEK material is intentionally separate from data backups.
  Use `chatto keys export`/`import` for built-in KMS key records.
- When adding streams or KV buckets, decide whether backup should include or skip
  them and update `skipReason()` if needed.

## Backend Tests

- Use `mise test-cli` for full backend checkpoints. It includes the
  `test_endpoints` build tag.
- Iterate with targeted tests:

```sh
mise x -- go test ./internal/core -run TestName -timeout 30s
mise x -- go test -tags test_endpoints ./internal/http_server -run TestName -timeout 30s
```

- Always set a timeout for targeted Go tests.
- Use table-driven tests where practical.
- Permission tests need positive and negative cases.
- DM behavior needs explicit coverage when touching room/message/permission logic.
- Endpoint tests for `/auth/test/*` or `/webhooks/test/*` require
  `//go:build test_endpoints`.
- Use `go-smtp-mock` with `MultipleMessageReceiving: true` and
  `WaitForMessages` to avoid email-test races.

## Local Profiling

- Store local benchmark/profiling artifacts under `.context/bench/`.
- Startup CPU profile:
  `CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE=.context/bench/startup.pprof`.
- Runtime pprof: set `CHATTO_METRICS_ENABLED=true`,
  `CHATTO_METRICS_PPROF=true`, and bind metrics to localhost.
