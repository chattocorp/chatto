# Projection Inventory

Key files: [`cli/internal/core/core.go`](../../cli/internal/core/core.go), [`cli/internal/events/projector.go`](../../cli/internal/events/projector.go), [`cli/internal/core/projection_subjects_test.go`](../../cli/internal/core/projection_subjects_test.go)

Projections are in-memory read models rebuilt from `EVT`. `NewChattoCore`
registers each top-level projector once with a stable machine-readable key, such
as `content_keys`, and a human display name, such as `Content Keys`.

`ChattoCore.Run` starts one process-local ordered EVT consumer per registered
projection. Each projector owns its physical filters, replay progress, failure
state, and readiness. Chatto still waits for every registered projection to
become current before completing boot. Writers wait for the relevant projector
sequence before returning read-your-writes.

The projector framework owns JetStream message handling and passes stable
stream sequence numbers into `Projection.Apply`. Projection implementations do
not inspect consumer sequence numbers or raw JetStream metadata.

Projections that require event-envelope idempotency keep event-ID sets only
through the captured startup target. Clean histories then release those sets
and use the highest applied stream sequence as a constant-size steady-state
guard. If startup replay observes a duplicate ID, only that projection retains
its set and first-event-wins compatibility behaviour. Projection diagnostics
report both retained event-ID memory and whether compatibility mode is active.

Related decisions: [ADR-007](../adr/ADR-007-per-user-encryption-with-crypto-shredding.md),
[ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md), and
[ADR-050](../adr/ADR-050-ephemeral-encrypted-projection-snapshots.md).

## Snapshot support

`core.projection_snapshots` enables ADR-050 encrypted projection snapshots.
Every eligible projection owns one opaque, projection-scoped contract ID and
generation prefix. The contract covers serialized state, replay semantics,
consumed event families, and cutoff meaning. Most contracts currently use
`v1`; the user profile contract uses `v2`.

Snapshot loads and replay frontiers are projection-local. A successful restore
starts that projection's ordered consumer at one greater than its cutoff. A
missing, invalid, or unavailable snapshot cold-replays only its owning
projection. Projections without matching EVT history have no state to
accelerate and do not publish zero-cutoff generations. Credential-bearing user
state is owned by `UserAuthProjection` and cold-replays from eight focused user
event families.

The projector framework atomically captures each projection's explicit
protobuf state with its latest applied logical EVT sequence. Room Timeline
retains encrypted body envelopes and rebuilds derived indexes. Mentionables
retains encrypted login source events and wrapped DEK records rather than
plaintext handles or lookup digests. The Users codec retains encrypted login,
display-name, and verified-email values, lookup digests, wrapped DEK records,
and non-secret profile metadata. Its schema has no fields for password verifiers,
authentication generations, external identity subjects, or OAuth consent.

Every replica checks snapshot eligibility immediately after boot and hourly.
Each scheduled pass attempts the `MEMORY_CACHE` lease once; a winner runs jobs
sequentially and releases the lease before the hourly wait. The worker publishes
after cold or delta replay and refreshes unchanged generations once they reach
23 hours old. Repository OCC remains the correctness boundary for staggered or
stale writers.

S3 expiry uses a separate `MEMORY_CACHE` cooldown claim shared by all replicas.
The first elected pass after the cooldown expires runs bounded cleanup and keeps
the claim for 24 hours on success. Failures release it for an hourly retry.

Generations are compressed and authenticated with XChaCha20-Poly1305 under an
HKDF key derived from `core.secret_key`, then stored under
`internal/projection-snapshots/{projection}/{contract}/objects/{opaqueEpoch}/{generationId}`
in the dedicated NATS `PROJECTION_SNAPSHOTS` Object Store or configured S3
bucket. Their encrypted current/previous pointers live in `RUNTIME_STATE` and
use KV revision OCC regardless of payload backend. The opaque pointer locator
is scoped by projection and contract, so deployments using different contracts
cannot read, rotate, or compare each other's generations.

A new secret uses a different generation epoch and pointer locator. EVT carries
a versioned opaque incarnation ID so snapshot validation survives process
reconstruction and backup restore but changes when EVT is recreated.

`core.projection_snapshot_retention` defaults to seven days. NATS applies it as
the Object Store TTL. S3 uses a bounded age-expiry pass after daily publication
unless `core.projection_snapshot_s3_cleanup` is disabled for an external
lifecycle policy. S3 deletion requires the exact generation-key grammar,
expected snapshot content type, and private object-purpose marker. Snapshot and
expiry failures are logged and never affect core readiness or EVT-backed
reconstruction. Legacy cohort paths remain outside application S3 expiry.

| Projection | Contract | Payload store | Pointer store | Publication |
| ---------- | -------- | ------------- | ------------- | ----------- |
| Threads, Room Directory, Server Config, Room Group Layout, Room Timeline, Call State, Assets, Reactions, Content Keys, RBAC, Mentionables | `v1` per projection | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Elected publisher checks hourly; cold/delta replay publishes immediately and unchanged state refreshes at 23 hours |
| Users (profile state only) | `v2` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Same elected age-aware publisher |

## Registered projections

| Runtime area       | Registered projector | Consumes                                                   | Read models / primary readers                                                             |
| ------------------ | -------------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Room directory     | Room Directory       | `evt.room.>`                                               | `RoomCatalogProjection`, `RoomMembershipProjection`, `RoomBanProjection`; room/member queries, room authorization, and Universal-room effective membership |
| Room organization  | Room Group Layout    | `evt.group.>`, `evt.layout.>`                              | `RoomGroupProjection`, `RoomLayoutProjection`; sidebar groups, sidebar links, and mixed sidebar item ordering |
| Room timeline      | Room Timeline        | `evt.room.>`                                               | Visible room timeline, latest message bodies, tombstone timestamps, hidden echoes, current attachment-bearing message index, direct message-post lookup, and message asset references |
| Assets             | Assets               | `evt.asset.>`, legacy `evt.room.*.asset_*`                 | Asset creation metadata, room scope, processing manifests, derivative graph, deletion state, and legacy room-asset compatibility |
| Threads            | Threads              | `evt.room.*.thread_created`, `evt.room.*.thread_followed`, `evt.room.*.thread_unfollowed`, `evt.room.*.message_posted`, `evt.room.*.message_edited`, `evt.room.*.message_retracted`, `evt.user.*.user_key_shredded` | Per-thread reply logs, summaries, participants, reply counts, and follow state             |
| Reactions          | Reactions            | `evt.room.>`                                               | Current canonical per-message reaction sets, echo-to-original reaction aliases, and room-scoped snapshot OCC positions; intentionally broad so reaction writes can OCC against the room tail |
| Voice calls        | Call State           | `evt.room.>`                                               | Current LiveKit call session, participants, active room IDs, and room-scoped snapshot OCC positions |
| Server/user config | Server Config        | `evt.config.>`, selected user cleanup/preference facts     | Server config, branding refs, user preferences, notification levels, blocked usernames     |
| Users              | Users                | `evt.user.>`                                               | Account/profile/custom-status state, verified emails, lookup digests, and encrypted user PII |
| User authentication | User Auth            | Focused account, password, external-identity, consent, deletion, and key-shredding user facts | Password verifiers, auth generations, external identity links, and OAuth consent; always cold-replayed |
| Content keys       | Content Keys         | `evt.user.*.dek_generated`, `evt.user.*.user_key_shredded` | Active and shredded user DEK epochs for message bodies and user PII                        |
| RBAC               | RBAC                 | `evt.rbac.>`                                               | Roles, role order, assignments, scoped allow/deny decisions                                |
| Mentions           | Mentionables         | `evt.>`                                                    | Global mention-handle ownership across users, roles, `@all`, and `@here`                  |

Registered projector keys are used by metrics and automation. Registered names
match the admin projection diagnostics. Composite projections expose nested
read models, but only their parent projector is started by `ChattoCore.Run`.

Independent consumers isolate snapshot availability, replay cost, status, lag,
failure, and read-your-writes waiters per projection. `Subjects()` is the
logical consumption and readiness contract; optional replay subjects are the
projection-owned physical consumer filters.

Focused logical filters suit stable derived indexes such as Threads. Broad
filters remain intentional for projections whose snapshots expose room-tail
OCC positions, such as Reactions and Call State. Threads reports the focused
logical subjects above for waits and diagnostics; non-thread room facts are
skipped before `Apply`.

`UserProjection` retains encrypted user fields and their AAD metadata. The user
and mentionable projections decrypt login and email values only transiently
while applying events to derive in-memory lookup digests; neither plaintext nor
the digests are persisted in `EVT`. Read hydration decrypts profile PII with
request-scoped DEK reuse. KMS and decryption failures remain operational errors
rather than appearing as missing or deleted users.
`UserAuthProjection` is independently locked, registered, and replay-guarded.
The `UserProjection` facade delegates credential and external-identity reads to
it so API callers keep one user boundary while snapshot serialization cannot
reach authentication state.
