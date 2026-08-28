# Instructions for Agents Working in `cli/`

This file applies to backend code: Go services, ConnectRPC, NATS/JetStream,
authorization, live events, backup and restore, and backend tests.

## Non-Negotiables

- Chatto can run in more than one replica. Never use process-local
  serialization for correctness.
- NATS JetStream/KV is the primary data store. Use JetStream OCC or KV
  `Create`/revision `Update` for uniqueness and cross-replica invariants.
- Durable domain state belongs in `EVT`; latest-value runtime state belongs in
  `RUNTIME_STATE` only when it is truly runtime/latest-value state.
- Services own their domain state and projections. Do not bypass service
  boundaries to access JetStream, KV, or projections from unrelated code.
- Call access is generation-bound. Use one call-state snapshot whenever an
  operation combines call identity and participants. For access credentials,
  capture the call ID and E2EE key reference from the same projected session,
  resolve that key reference, then revalidate both values before issuing
  access; fail or retry if the generation changed. Never assemble one response
  or credential from independent call-state reads.
- Do not log PII. Use opaque IDs, counts, Boolean values, event names, and safe
  hashes.
- Projections must not retain decrypted PII when encrypted source fields can be
  retained and hydrated at read boundaries. Keep derived lookup state
  non-plaintext, and never turn KMS or decryption failures into apparent
  absence, deletion, or a free uniqueness claim.
- Required encryption dependencies and projected key state must fail closed.
  Never treat unavailable key state as ordinary absence: generation,
  decryption, and shredding must return an error before mutating key material.

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
- For hot, high-fanout latest-value KV reads, let the owning model maintain one
  process-wide filtered watcher with an explicit initial-sync readiness
  barrier. Serve detached reads from that in-memory index, keep KV authoritative
  with `Create`/revision `Update`, and wait for the successful KV revision to
  reach the local watcher before returning when read-your-writes matters.
  Watchers belong to the process lifecycle, never to a request, user, or
  WebSocket goroutine.
- Projection-backed decisions need OCC tokens for the same event-log prefix as
  the projected state. Do not decide from a projection and publish against an
  unrelated stream tail.
- Treat OCC conflicts according to command semantics. For interactive
  replacement-style edits, do not replay precomputed events after a conflict.
  Either return a conflict so the client can preserve the draft and ask the
  user to reload, or re-read state and rerun authorization, validation,
  uniqueness checks, no-op detection, and event construction from the original
  command intent. Retry an unchanged event only when its meaning is proven to
  remain valid after intervening writes. Sparse patches avoid overwriting
  untouched fields, but detecting a stale edit to the same field requires a
  client-supplied revision.
- Defaults required for a newly created aggregate must commit with its creation
  facts in the same atomic EVT batch. Do not reconstruct creation-time defaults
  later by scanning projections during startup.
- When a committed EVT fact requires a KMS, LiveKit, object-store, or other
  external side effect, that fact must provide a durable recovery path. Verify
  crash recovery, multi-replica discovery, lease handover, and bounded
  request-path cost.
- Treat a named durable consumer as a persisted, deployment-wide resource
  contract. Required effect consumers must not use inactivity cleanup, delete
  themselves on worker shutdown, or be deleted by one replica. Keep consumer
  creation, versioned names, rollout, and retirement application-owned rather
  than adding them to `events.DurableWorker`.
- Removing or incompatibly changing a durable consumer requires ADR-069's
  staged migration: stop its producers or overlap an idempotent replacement,
  exclude old binaries that can recreate it, then prove a stable drain or
  replacement cutoff before idempotent deletion. Treat skipped work as an
  explicit abandonment decision and update the NATS inventory. Never
  garbage-collect unknown `chatto-*` consumers merely because the current
  binary does not declare them.
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
- Keep Chatto's EVT incarnation metadata key, generation, format validation,
  and lookup in `internal/evtstream` or application composition. Reusable
  projector restore mechanics receive an application resolver and treat its
  result as an opaque, non-empty value. Resolve identity from the same fresh
  `StreamInfo` as restore sequence bounds; framework code must not impose
  Chatto's metadata key or identity syntax. Bind the resolved identity to the
  projector run, capture it with snapshot state and cutoff, and publish that
  captured value rather than caching an identity separately in worker wiring.
  Check identity immediately before and after the capture barrier; never hold
  the projection apply barrier across NATS or other external I/O.
- Keep the package dependency direction application/core code ->
  `internal/evtstream` -> the `hmans.de/chatto/pkg/events` shared module.
  Chatto's `corev1.Event` codec,
  aggregate subjects, event tokens, typed publisher/projector constructors, and
  envelope-aware effect consumers belong in `internal/evtstream`.
  The framework lives in the independently versioned `../pkg/events` module.
  It remains an unstable incubation surface and must not import Chatto
  protobufs or `internal/evtstream`. Keep its production imports limited to
  the Go standard library and `github.com/nats-io/nats.go`; application-wide
  helpers must not become hidden extraction dependencies. Keep its tests
  portable too: test infrastructure may add `nats-server/v2`, but must not
  borrow other Chatto packages or unrelated third-party helpers.
- Drive reusable framework API changes from external-package consumer
  contracts with non-Chatto envelopes. Do not add generic framework surface
  merely to shorten Chatto wiring.
- Keep embedded NATS process mechanics behind the independently versioned
  `hmans.de/chatto/pkg/natsruntime` module from ADR-058. Chatto retains its
  configuration, listener, authentication, monitoring, logging, storage, and
  deployment policy in `internal/embedded_nats`; the shared module must not
  import Chatto packages.
- Keep raw XChaCha20-Poly1305 and 256-bit key-wrapping primitives behind the
  independently versioned `hmans.de/chatto/pkg/datacrypto` module from
  ADR-060. Chatto retains its associated-data formats, legacy cipher path,
  key references and hierarchy, envelope serialization, storage, KMS, cache,
  rotation, and cryptographic-erasure policy in `internal/encryption`; the
  shared module must not import Chatto packages.
- Keep TOML file loading and struct-tagged environment overrides behind the
  independently versioned `hmans.de/chatto/pkg/appconfig` module from ADR-061.
  Chatto retains its configuration schema, environment names, compatibility
  aliases, defaults, normalization, validation, generated examples, and CLI
  flag policy in `internal/config` and `cmd`; the shared module must not import
  Chatto packages or tighten existing configuration compatibility implicitly.
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
- Snapshot contract IDs combine a manual restore-semantics token with a
  fingerprint of the codec's reachable protobuf schema. Keep only the current
  snapshot message: a schema change automatically selects a new
  contract-scoped generation, while an old binary retains its own schema and
  namespace. Bump the manual token when `Apply`, replay, cutoff, or restore
  semantics change without a schema change.
- Most current snapshot contracts use semantic token `v1`; Assets and user
  profile use `v2`, while Room Timeline uses `v3`. Keep password
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
- Keep projection `Subjects()` precise as the logical application and readiness
  contract. Prefer one physical replay filter where practical. In particular,
  benchmark a broad wildcard combined with sparse extra families against one
  broader `ReplaySubjects()` filter on real EVT history: JetStream multi-filter
  scanning can cost more than delivering extra envelopes, and the projector
  rejects non-logical subjects before protobuf decoding.
- Projection snapshot methods are optional. Locally checkpointed projections
  own disposable derived state and must bind it to a stable projection key,
  contract ID, EVT stream incarnation, and retained sequence bounds. A
  successful `Apply` must atomically persist both its materialized changes and
  the supplied logical EVT sequence.
- Give each projection exactly one restore authority: shared snapshots or a
  local checkpoint, never both. Missing, corrupt, incompatible, future, or
  retention-gapped local state may be reset and replayed; transient filesystem
  and volume failures must fail startup without destructively resetting a
  potentially valid checkpoint. Define backup exclusion, deletion, and
  plaintext/privacy behavior for each checkpointed feature.

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
- For non-owners, each direct-user or explicitly assigned role contributes its
  nearest room/group/server decision. Denies win across those subjects. The
  implicit `everyone` role supplies the scoped baseline: a named allow overrides
  an everyone deny only at the same or a nearer scope. Effective owners bypass
  normal permission decisions.
- Effective owner means durable `owner` role or verified email matching
  `owners.emails`.
- DM rooms have an explicit privacy boundary; owners/admins/moderators do not
  get moderation visibility into DM contents.
- DM membership is the complete DM content-read boundary. `message.read`
  applies only to channel rooms. Do not add a second DM read gate.
- A bot must never start or fetch a DM through `RoomService.StartDM`, even when
  it has `message.post` or the DM already exists. A human must start the DM.
  After that, the bot can read it through membership and can use its normal
  message permissions inside it.
- Permission strings use two or more non-empty dot-separated components.
  Hyphens can stay inside a component: `room.ban-member`,
  `message.post-in-thread`, and `message.read.interactions` are valid.
- Each registered dotted prefix includes its registered descendants. A nested
  permission must have a registered immediate parent with the same category
  and scopes. Treat each permission name as an authorization contract.
  Currently, `message.read` includes `message.read.interactions`.
- Add permissions in Go first, regenerate frontend mirrors, and test scope and
  DM-boundary behavior.
- Targeted operations are permission-gated, not rank-gated: role assignment uses
  `role.assign`, direct user permissions use `user.manage-permissions`, room
  bans use `room.ban-member`. A non-owner's role assignment authority is bounded
  by the target role's explicit scoped permission decisions; assigning requires
  every allow, revoking requires every allow and deny, and the `owner` role is
  owner-only.
- Authorization-sensitive event writes must evaluate authorization inside the
  target aggregate's OCC retry. Request-time authorization is the default: a
  conflict-free command may finish after a concurrent cross-aggregate
  revocation. Commands that require strict commit-time revocation semantics
  must also guard the narrow authorization fence, keep its writer classification
  complete, and wait every projection consulted by authorization through the
  relevant captured subject tail inside the retry. Use ADR-068's whole-EVT
  boundary only for a genuine stream-wide invariant whose cost is worth
  contention with unrelated `evt.>` traffic, and record exceptional consistency
  choices in the relevant ADR/FDR.

## Admin Interface

- Owners/admins can see operational metadata, not user content. Message/file
  visibility for moderation must be an explicit audited feature.
- Management routes live under `/chat/[serverId]/manage/`, with server-only
  pages under `manage/server/` and delegated room/group pages beside it.
- The shared admin `Panel` component is used in both management and settings
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
- Treat fixture and setup errors as fatal before using returned values. Never
  discard an error from helpers such as `CreateRoom` or `CreateUser` and then
  dereference the result; fail the test at the setup call instead.
- `JoinRoom` is a self-service operation. The actor must be the account that
  joins. Use `AddMember` when an authorized account adds a different account
  during test setup.
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
