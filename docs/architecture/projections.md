# Projection Inventory

Key files: [`cli/internal/core/core.go`](../../cli/internal/core/core.go), [`cli/internal/events/projector.go`](../../cli/internal/events/projector.go), [`cli/internal/core/projection_subjects_test.go`](../../cli/internal/core/projection_subjects_test.go)

Projections are in-memory read models rebuilt from `EVT`. `NewChattoCore`
registers each top-level projector once with a stable machine-readable key, such
as `content_keys`, and a human display name, such as `Content Keys`.

`ChattoCore.Run` replays `evt.>` through one process-local ordered consumer. It
decodes each event once, dispatches it to projections whose logical subject
filters match, records initial replay duration, and waits for projections to
become current at boot. Writers wait for the relevant projector sequence before
returning read-your-writes.

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

`core.projection_snapshots` enables the ADR-050 Thread snapshot canary. The
projector framework atomically captures projection state with its applied EVT
sequence. It can restore one projection while the shared `evt.>` consumer still
replays from the beginning for all others.

`ThreadProjection` uses the `threads-v1` protobuf codec. Its encrypted
generation bundle contains no message bodies or decrypted PII, and it rebuilds
derived indexes on restore. After boot, one replica is elected through a
`MEMORY_CACHE` lease to publish a generation whenever Threads has advanced.

Generations are compressed and authenticated with XChaCha20-Poly1305 under an
HKDF key derived from `core.secret_key`. They are stored under a secret-derived
opaque epoch in either the NATS `PROJECTION_SNAPSHOTS` Object Store or the
configured S3 bucket.

The encrypted current/previous pointer lives in `RUNTIME_STATE` and uses KV
revision OCC for either payload backend. It carries cutoff, EVT incarnation,
and projection compatibility metadata, so publication rejects both an obsolete
revision and a causally older capture.

A new secret uses a different generation epoch. This prevents its cleaner from
deleting generations still used by old-secret replicas during a rolling
change. Namespace `v1` is permanently limited to Threads, and the repository
rejects other keys. Adding another snapshotted projection requires a new
namespace version so older cleaners remain safe during mixed-version rollouts.

EVT carries a versioned opaque incarnation ID in stream metadata. Snapshot
compatibility therefore survives process reconstruction and backup restore but
changes when EVT is recreated. Missing IDs are deterministically derived once
from stream creation time so concurrent replicas converge, then persisted.
Runtime snapshot paths use the captured immutable value rather than refreshing
NATS client metadata concurrently.

A separately elected cleanup worker starts 5-10 minutes after boot and
inventories only its private key epoch every six hours. Each pass deletes at
most 100 objects or 1 GiB, and only when unreferenced generations are at least
24 hours old. The worker collects the bounded batch during one complete
read-only inventory, checks lease ownership once before deletion, and retries
failed or deletion-limited passes after 30 minutes.

Snapshot storage, EVT identity, projector configuration, load, validation,
pointer publication, lease, inventory, and cleanup failures are logged. They
disable only snapshot functionality where necessary and never affect core
readiness or EVT-backed reconstruction. Pre-epoch canary objects are outside
this cleaner and require provider lifecycle or later migration tooling.

| Projection | Namespace | Codec | Payload store | Pointer store | Publication |
| ---------- | --------- | ----- | ------------- | ------------- | ----------- |
| Threads | `v1` | `threads-v1` protobuf | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted `RUNTIME_STATE` pointer with KV revision OCC | Elected publisher after boot when the projection advances |

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
| Users              | Users                | `evt.user.>`                                               | Account/profile/custom-status/auth lookup state, verified emails, external identity links, encrypted user PII |
| Content keys       | Content Keys         | `evt.user.*.dek_generated`, `evt.user.*.user_key_shredded` | Active and shredded user DEK epochs for message bodies and user PII                        |
| RBAC               | RBAC                 | `evt.rbac.>`                                               | Roles, role order, assignments, scoped allow/deny decisions                                |
| Mentions           | Mentionables         | `evt.>`                                                    | Global mention-handle ownership across users, roles, `@all`, and `@here`                  |

Registered projector keys are used by metrics and automation. Registered names
match the admin projection diagnostics. Composite projections expose nested
read models, but only their parent projector is started by `ChattoCore.Run`.

The shared replay fanout reduces duplicate delivery and protobuf decoding while
keeping each projection's status, lag, failure, and read-your-writes waiters
independent. `Subjects()` is the logical consumption and readiness contract;
optional replay subjects are only the physical consumer filter.

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
