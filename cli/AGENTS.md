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
- Projections must not retain decrypted PII when encrypted source fields can be
  retained and hydrated at read boundaries. Keep derived lookup state
  non-plaintext, and never turn KMS or decryption failures into apparent
  absence, deletion, or a free uniqueness claim.

## Architecture Touchpoints

- `cli/internal/core` is domain logic and service/projection code.
- `cli/internal/connectapi` is the protobuf/ConnectRPC API.
- `proto/chatto/core/v1` holds persisted/internal protobufs.
- `proto/chatto/api/v1` holds public ConnectRPC API protobufs.
- The relevant `docs/architecture/` inventories, FDRs, and ADRs should move
  with architectural changes.

## Public APIs

- Public RPC API surface lives in ConnectRPC/protobuf or the planned wire
  protocol.
- Keep ConnectRPC transport thin: authenticate, decode, map errors/responses,
  and delegate policy/domain work to shared services.
- Keep projected read hydration out of ConnectRPC handlers. Put per-response
  batching, bounded concurrency, include-map construction, and protobuf response
  assembly in small `*_assembler.go` helpers near the service that owns the
  response shape.
- Do not create a generic ConnectRPC loader package until multiple assemblers
  share the same non-trivial loading semantics. Prefer concrete assemblers plus
  small generic mechanics such as `internal/parallel`.
- Put operation-specific authorization in the core operation model for that
  behavior. Low-level `ChattoCore` helpers are not public transport entry
  points and may assume their caller already performed the appropriate gate.
- REST endpoints are acceptable for OAuth callbacks, webhooks, health checks,
  and uploaded assets. Public server discovery belongs to
  `ServerDiscoveryService.GetServer` in the ConnectRPC API.
- `ServerDiscoveryService.GetServer` is compatibility-sensitive. Preserve its
  public CORS behavior, required JSON/protobuf fields, and OAuth discovery
  fields unless there is a rollout plan.

## Event-Sourced State And NATS

- `EVT` is the durable event-sourced stream. `SERVER_EVENTS` is historical and
  should not receive new runtime writes or live delivery paths.
- `RUNTIME_STATE` stores sessions, auth/workflow tokens, notification state,
  push subscriptions, cached previews, wrapped DEK records, and similar
  latest-value runtime data.
- Projection-backed decisions need OCC tokens for the same event-log prefix as
  the projected state. Do not decide from a projection and publish against an
  unrelated stream tail.
- Defaults required for a newly created aggregate must commit with its creation
  facts in the same atomic EVT batch. Do not reconstruct creation-time defaults
  later by scanning projections during startup.
- When a committed EVT fact requires a KMS, LiveKit, object-store, or other
  external side effect, that fact must provide a durable recovery path. Verify
  crash recovery, multi-replica discovery, lease handover, and bounded
  request-path cost.
- Match distributed lease ownership to the work lifecycle. Continuous polling
  workers may hold and renew a lease while running; periodic workers should
  attempt one lease acquisition per pass and wait outside the lease. Treat the
  lease as duplicate-work reduction rather than fencing, and log ownership
  changes or failures rather than every successful renewal. Enforce a
  cluster-wide periodic rate with shared expiring state, not a per-replica
  timer; retain cooldowns only after successful work so failures can retry.
- Subject/key shapes are part of the storage contract. When changing them,
  update constructors, parsers, tests, architecture docs, and e2e coverage.
- For mixed records in one stream or KV bucket, encode discriminators in the key
  prefix so reads can filter by subject/prefix without deserializing everything.
- Projection snapshots are disposable acceleration data, never recovery data.
  Bind them to the durable EVT incarnation identity in stream metadata as well
  as the stream name and cutoff sequence; reject missing, corrupt,
  incompatible, or future snapshots by replaying EVT. Do not use
  `StreamInfo.Created` as a persisted identity.
- Snapshot restore codecs must be transactional on error and must account for
  compatibility state preloaded before projector startup. Privacy-review every
  persisted field: do not snapshot decrypted bodies, raw PII, credentials,
  unwrapped keys, or state that would weaken crypto-shredding.
- Give every snapshotted projection one opaque, projection-scoped contract ID.
  The contract covers serialized state, replay semantics, consumed event
  families, and cutoff meaning. If restoring an existing snapshot would no
  longer equal replaying EVT through its cutoff, bump the contract ID. Treat
  IDs only as bounded path-safe equality tokens, never as ordered versions.
  Scope both generation paths and pointer keys by projection and contract so
  different contracts never read or overwrite each other. Keep pointers on a
  durable revisioned store and publish them with OCC; a process lease is not
  fencing. Capture the contract once during projector configuration and use
  that same value for restore and publication; do not duplicate it in wiring.
  Carry cutoff, creation time, EVT incarnation, and contract metadata in
  the pointer.
  Allow same-cutoff refreshes for retention, but do not republish a fresh,
  unchanged generation merely because a process restarted. Reject regressing
  captures, and use pointer revision OCC to prevent concurrent writers from
  replacing newer history.
  Scope generation object paths by encryption-key epoch. NATS Object Store TTL
  and marker-verified S3 age expiry may remove referenced generations; loaders
  must treat absence as a normal cold-replay condition.
- Most current snapshot contracts use projection-local ID `v1`;
  the user profile projection uses `v2`. Keep password
  verifiers, auth generations, external identity subjects, and OAuth consent in
  the independently cold-replayed `UserAuthProjection`; never add them to a
  profile snapshot schema or codec.
- Every projection owns its ordered EVT consumer, snapshot restore, and replay
  frontier. A usable snapshot starts only that consumer after its cutoff; a
  missing snapshot cold-replays only its owning projection. Keep global boot
  readiness gated on every required projection becoming current. Release
  boot-time sequence waiters when installing a restored cutoff, and test
  all-restored, partial, corrupt, future, tail-replay, and restore-in-flight
  waiter interleavings.

## Live Events

- Durable facts publish to `evt.>` through `EventPublisher`; JetStream republish
  exposes committed facts on `live.evt.>`.
- Transient UI sync publishes `corev1.LiveEvent` on `live.sync.>` through
  `publishLiveEvent`.
- Pick one delivery path per conceptual update. Do not double-publish both a
  durable event and a transient live event for the same UI change.
- Do not publish from projector `Apply` methods; every replica runs projectors.
- Do not use a locally published NATS message as a global ordering fence for
  JetStream republish or messages from other replicas. Tie projection snapshots
  and stale-event suppression to authoritative EVT stream sequences.
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

- `/assets/server/*` is unauthenticated and may serve only positively
  classified public server assets: current/historical avatars, server branding,
  and server-fetched link-preview images. Classification must happen before
  transform-signature parsing, resize-cache lookup, object reads, or transforms.
- New NATS public objects and URLs use `public/{assetId}`. Keep canonical
  `{assetId}` aliases and the positive compatibility classifier for historical
  flat-key public objects; never migrate content during an unauthenticated read.
- `SERVER_ASSETS` is a mixed store. Never treat an opaque key, missing private
  metadata, or a valid transform signature as proof that an object is public.
  Deny room-asset declarations and tombstones, `Room-Id`/`Upload-Id` metadata,
  reserved namespaces, and unknown object classes with the same 404 response.
- Stable asset URLs use `/assets/files/{assetId}` and image transform variants.
- Browser-facing ConnectRPC attachment URL fields append a signed per-user
  `access` ticket and expose expiry in the API asset URL object.
- The ticket is the browser capability: it carries asset/user/expiry/transform
  claims and is accepted without cookies or bearer headers.
- Asset serving still checks that the signed user remains a member of the asset's
  room, so kick/leave revokes future fetches.
- URLs are per-user and intentionally not shared/CDN-cacheable. Treat leaked URLs
  as usable until expiry or membership loss.
- Chatto streams protected asset bytes by default. It may redirect heavy passive
  originals such as video, audio, and large files to short-lived presigned S3
  URLs after the same authorization check.
- The legacy `/assets/attachments/{signedLocator}` route has been removed; do
  not add new callers for signed locator URLs.

## Backup, Restore, And Keys

- `chatto backup` snapshots JetStream streams/KV and writes a manifested
  `.tar.gz`, optionally age-encrypted.
- `chatto restore` restores snapshots into embedded or external NATS and supports
  conflict modes `error`, `skip`, and `overwrite`.
- `KV_ENCRYPTION_KEYS`/KEK material is intentionally separate from data backups.
  Use `chatto keys export`/`import` for built-in KMS key records.
- When adding streams, KV buckets, or Object Stores, decide whether backup should
  include or skip them and update `skipReason()` if needed.

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
- Tests that mutate a projection wired into a running `ChattoCore` must append
  the fact through `EventPublisher` and wait for the owning projector. Reserve
  direct `Apply` calls for isolated projection tests, using monotonically
  increasing stream sequences.
- Permission tests need positive and negative cases.
- DM behavior needs explicit coverage when touching room/message/permission logic.
- Endpoint tests for `/auth/test/*` or `/webhooks/test/*` require
  `//go:build test_endpoints`.
- Use `go-smtp-mock` with `MultipleMessageReceiving: true` and
  `WaitForMessages` to avoid email-test races.

## Local Profiling

- Store local benchmark/profiling artifacts under `.context/bench/`.
- Run `mise bench-projections` for projection replay throughput, allocations,
  and retained Go heap. It uses a versioned deterministic protobuf fixture and
  reports room timeline, threads, and combined results at multiple history
  sizes. Benchmarks are explicit and do not run as part of `mise test`;
  ordinary tests only run the small fixture-validation test.
- Compare projection changes with repeated before/after runs on the same
  machine and Go version. Keep the fixture version constant, check both 10,000
  and 50,000-message retained-heap results, and reject memory improvements that
  cause an unacceptable replay-throughput regression.
- Run `mise bench-projections-profile` for exact retained-heap attribution.
  Profiles are written under `.context/bench/projections/`; exact allocation
  sampling is intentionally slower than the normal benchmark.
- Treat the synthetic projection benchmark as a regression and attribution
  tool, not a production RSS model. Confirm meaningful wins against a restored
  real EVT history before shipping them. If the fixture event mix changes,
  bump its version so results from different workloads are not compared.
- For realtime connection-memory work, negotiate production WebSocket
  compression and use an external load generator so client allocations do not
  enter the server profile.
- Validate connection-scaled memory with server RSS/runtime `Sys` deltas and
  active-connection heap profiles. Treat in-process `HeapAlloc` benchmarks as
  regression signals, not production RSS models.
- Startup CPU profile:
  `CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE=.context/bench/startup.pprof`.
- Runtime pprof: set `CHATTO_METRICS_ENABLED=true`,
  `CHATTO_METRICS_PPROF=true`, and bind metrics to localhost.
