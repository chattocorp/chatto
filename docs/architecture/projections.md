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

With snapshots enabled, the eligible snapshot cohort and the cold
`UserAuthProjection` run as separate ordered replay groups.

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

`core.projection_snapshots` enables the ADR-050 encrypted snapshot cohort.
Threads remains alone in frozen namespace `v1`. Frozen namespace `v2` contains
Room Directory, Server Config, Room Group Layout, Room Timeline, Call State,
Assets, Reactions, Content Keys, RBAC, and Mentionables. Frozen namespace `v3`
contains only the user profile projection.

Snapshot loads run concurrently. If every projection with matching history
restores successfully, the cohort's shared `evt.>` consumer starts after its
lowest restored cutoff and each projector skips through its own cutoff. A
missing, invalid, or unavailable required snapshot moves that cohort's frontier
back to sequence 1. Credential-bearing user state is owned by
`UserAuthProjection` and cold-replays independently from eight focused user
event families.

The projector framework atomically captures each projection's explicit
protobuf state with its physical EVT watermark. Room Timeline retains encrypted
body envelopes and rebuilds derived indexes. Mentionables retains encrypted
login source events and wrapped DEK records rather than plaintext handles or
lookup digests. The Users codec retains encrypted login, display-name, and
verified-email values, lookup digests, wrapped DEK records, and non-secret
profile metadata. Its schema has no fields for password verifiers,
authentication generations, external identity subjects, or OAuth consent.

One replica is elected through a `MEMORY_CACHE` lease after boot to publish
advanced generations for all eligible projections. Generations are compressed
and authenticated with XChaCha20-Poly1305 under an HKDF key derived from
`core.secret_key`, then stored under a secret-derived opaque epoch in the
dedicated NATS `PROJECTION_SNAPSHOTS` Object Store or configured S3 bucket.
Their encrypted current/previous pointers live in `RUNTIME_STATE` and use KV
revision OCC regardless of payload backend.

A new secret uses a different generation epoch, preventing its cleaner from
deleting generations still used by old-secret replicas during a rolling
change. EVT carries a versioned opaque incarnation ID so snapshot compatibility
survives process reconstruction and backup restore but changes when EVT is
recreated.

A separately elected cleanup worker starts 5-10 minutes after boot, inventories
all three frozen namespaces every six hours, and deletes at most 100 objects or
1 GiB across one cleanup pass when unreferenced generations are at least 24
hours old. Snapshot failures are logged and never affect core readiness or
EVT-backed reconstruction. Pre-epoch canary objects remain outside this cleaner
and require provider lifecycle or explicit migration tooling.

| Projection cohort | Namespace | Payload store | Pointer store | Publication |
| ----------------- | --------- | ------------- | ------------- | ----------- |
| Threads | `v1` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted `RUNTIME_STATE` pointer with KV revision OCC | Elected publisher after boot when the projection advances |
| Room Directory, Server Config, Room Group Layout, Room Timeline, Call State, Assets, Reactions, Content Keys, RBAC, Mentionables | `v2` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointers with KV revision OCC | Same elected publisher, one generation per advanced projection |
| Users (profile state only) | `v3` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted `RUNTIME_STATE` pointer with KV revision OCC | Same elected publisher when the profile projection advances |

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
`UserAuthProjection` is independently locked, registered, and replay-guarded.
The `UserProjection` facade delegates credential and external-identity reads to
it so API callers keep one user boundary while snapshot serialization cannot
reach authentication state.
