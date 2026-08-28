# Projection Inventory

Key files: [`cli/internal/core/projection_wiring.go`](../../cli/internal/core/projection_wiring.go), [`pkg/events/projector.go`](../../pkg/events/projector.go), [`pkg/events/projection_checkpoint.go`](../../pkg/events/projection_checkpoint.go), [`cli/internal/search/bleve/projection.go`](../../cli/internal/search/bleve/projection.go), [`cli/internal/core/asset_processing_runtime.go`](../../cli/internal/core/asset_processing_runtime.go), [`cli/internal/core/projection_subjects_test.go`](../../cli/internal/core/projection_subjects_test.go)

Projections are derived read models rebuilt from `EVT`. Most live in memory;
optional providers may own disposable locally checkpointed indexes.
`initializeCoreProjections` registers each top-level core projector once with a
stable machine-readable key, such as `content_keys`, and a human display name,
such as `Content Keys`. `NewChattoCore` installs that single registry into the
core runtime. Each registration also declares whether that same stable key is
eligible for shared snapshots. Snapshot configuration iterates the registry
directly rather than maintaining a parallel projector list.

Core couples each projection pointer to its exact projector as one typed
`events.ProjectionHandle` from the independently versioned incubation module.
Projection-aware domain models and the bundled search provider retain those
handles instead of parallel state/runner arguments. Chatto-specific keys,
names, memory estimates, diagnostics, and snapshot policy remain in the core
registration layer rather than becoming framework metadata. This boundary
follows [ADR-056](../adr/ADR-056-extractable-nats-event-sourcing-framework.md).

`ChattoCore.Run` starts one process-local ordered EVT consumer per registered
projection. Each projector owns its physical filters, replay progress, failure
state, and readiness. Chatto still waits for every registered projection to
become current before completing boot.

Writers wait for the relevant projector sequence before returning
read-your-writes. Projection-aware domain models keep the projector references
needed for those waits; the `ChattoCore` facade does not mirror every registered
projector.

`InvitationModel` derives a process-local 16-character invite-token lookup
index from the cold-replayed Invitation projection. The index contains no
additional durable facts or stored bearer values and is rebuilt whenever the
append-only projected invitation identity count changes.

`CallModel`, `AssetModel`, and `UserModel` own their projection reads and
readiness for domain logic and API adapters. `UserModel` keeps profile,
authentication, and content-key reads behind one boundary while their
independent projectors retain separate replay and snapshot policies. Call token
access material binds the call ID and E2EE key to one revalidated projection
generation. Active-call and asset API mapping use detached snapshots captured
under one projection lock.

Room timeline message hydration obtains deletion and channel-echo metadata as
one detached snapshot through `RoomTimelineReadModel`; ConnectAPI does not read
that projection directly. `RoomModel` is the sole production owner of the Room
Directory, Room Group Layout, Room Timeline, Threads, and Reactions
projections. The Threads projection also derives channel message-to-root
mappings and account-to-thread interaction relationships from message-post
facts. Membership, message, thread, reaction, asset, realtime, room-group OCC,
and sidebar-ordering paths use focused `RoomModel` operations instead of
projection fields on `ChattoCore`. Raw membership reads are named as explicit
membership so they remain distinct from policy-derived Universal-room access.

Any non-cancellation error from checkpoint or snapshot restore, consumer setup,
or event application moves the projector into its failed state before its run
loop returns. Readiness and provider status therefore cannot remain
healthy-looking after an incomplete startup.

Projection consumers use a five-minute inactivity cleanup threshold. Because
event application is synchronous and disk-backed commits can temporarily stop
the pull loop, a shorter broker threshold could delete a live consumer while
its projection is still applying a batch. Projector shutdown still stops the
pull subscription; NATS later removes its ephemeral consumer.

The projector framework owns JetStream message handling and passes stable
stream sequence numbers into `Projection.Apply`. Projection implementations do
not inspect consumer sequence numbers or raw JetStream metadata. An optional
startup-batch capability groups only the replay through the target captured at
startup; live events continue through individual `Apply` calls.

The ordered replay lifecycle receives decoded application events through
`events.EventDecoder[E]`. Chatto's `evtstream.NewProjector` constructor
supplies the unchanged `corev1.Event` protobuf decoder, while
`NewDecodedProjector`/`NewDecodedProjectionHandle` expose the envelope-neutral
construction path. Decode failures remain fatal at the stored record's stream
sequence and cannot advance readiness.

Projections that require event-envelope idempotency keep event-ID sets only
through the captured startup target. Clean histories then release those sets
and use the highest applied stream sequence as a constant-size steady-state
guard. If startup replay observes a duplicate ID, only that projection retains
its set and first-event-wins compatibility behaviour. Projection diagnostics
report both retained event-ID memory and whether compatibility mode is active.

Related decisions: [ADR-007](../adr/ADR-007-per-user-encryption-with-crypto-shredding.md),
[ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md),
[ADR-050](../adr/ADR-050-ephemeral-encrypted-projection-snapshots.md),
[ADR-054](../adr/ADR-054-optional-projection-persistence.md),
[ADR-055](../adr/ADR-055-pluggable-message-search-over-nats.md), and
[ADR-066](../adr/ADR-066-durable-asset-processing-runtime-unit.md).

The asset-processing runtime unit owns a private, non-snapshotted
`AssetProjection`. It uses the same canonical and legacy replay subjects as the
main core projection, reaches the queue delivery's stream sequence before
processing, and waits for terminal writes before acknowledging. It is not part
of the `ChattoCore` projector registry and does not run main-app boot mutations.

## Local checkpoint support

The projector framework also supports a projection-owned local checkpoint.
The checkpoint contract binds the derived state and its highest atomically
applied EVT sequence to a stable projection key, a projection contract ID, and
the current EVT stream incarnation and retained sequence bounds. Chatto
supplies the identity resolver; at restore time the projector invokes it with
the same fresh stream-info snapshot that supplies the sequence bounds, then
carries the result as an opaque value.

A valid checkpoint replays only the remaining EVT tail. Its global stream
cutoff may be newer than the last event matching the projection's current
filters; only a cutoff beyond the EVT stream tail is a future checkpoint.

A projection uses at most one restore authority: ADR-050 snapshots, a local
checkpoint, or neither. A projection without either starts empty and cold-replays
`EVT`. Missing, corrupt, incompatible, future, or retention-gapped checkpoints
are invalid; the projection may safely reset owned state or fail startup for
operator recovery. A successful individual `Apply` or startup batch must
atomically commit its derived changes and supplied final stream sequence.

The bundled search provider owns the first locally checkpointed projection. It
is registered by its runtime unit rather than by `ChattoCore`. It consumes only
message body, message posting, message retraction, room deletion, user DEK
generation, and user key shredding event families, and uses projector key
`message_search`.

During captured startup replay it commits up to 256 ordered events and the
final checkpoint in one Bleve transaction, including a smaller final batch;
once current, each relevant live event is committed immediately.
Its checkpoint contract starts with `bleve-message-index-v9-` and includes a
stable fingerprint of the configured language analyzer set, so changing that
set forces a cold EVT replay.

The index stores current decrypted message text plus its body-event revision and
message/room/author/filter metadata. The state needed to apply a later edit or
posting event is a stored, non-indexed field in that same Bleve document; it is
not duplicated as one internal Bolt key per message. Candidate revisions must
match current core state before hydration, fencing provider catch-up races.

Message bodies use BM25 scoring over a language-neutral field plus the
operator-selected subset of all 22 complete language analyzers available in
the bundled Bleve version. Omitting `search_provider.languages` selects all
analyzers; an explicit empty list selects none of the language-specific fields.
The index also stores non-plaintext DEK event metadata required to decrypt
later EVT tail records after restart. Retraction, room deletion, and user key
shredding remove matching documents in the same committed batch. Bleve's normal
background merger reclaims obsolete segments; Chatto does not use Scorch's
manual `ForceMerge` operation as part of projection correctness or startup
readiness.

The directory is a privileged, disposable local cache excluded from Chatto
backups. Chatto creates it only when the configured path does not exist and
never recursively deletes an unreadable or incompatible disk index. Those
conditions fail provider startup; an operator must move or delete the dedicated
directory explicitly before restarting it for a cold EVT replay.

## Snapshot support

`core.projection_snapshots` enables ADR-050 encrypted projection snapshots.
Every eligible projection owns one opaque, projection-scoped contract ID and
generation prefix. The contract covers serialized state, replay semantics,
consumed event families, and cutoff meaning. Each ID combines a manual semantic
token with a fingerprint of the codec's reachable protobuf schema, so a schema
change automatically starts a new contract namespace. Most contracts use
semantic token `v1`; Assets uses `v3`, user profile uses `v4`, and Room Timeline
uses `v7`.

Room Timeline `v3` keeps retraction tombstones authoritative when a legacy
writer appends a later body payload and retains that payload's sequence for
secure deletion. Version 0.4 replicas use the earlier projection behavior, so
the 0.4-to-0.5 release upgrade requires coordinated replacement of every Chatto
server replica rather than a rolling server deployment.

Room Timeline `v4` adds the current attachment-bearing-message index. `v5`
rebuilds a room-and-author latest-original-post index from retained timeline
entries so Slow Mode remains equivalent after restore. Echo rows are excluded;
edits and retractions do not erase the original successful-post timestamp.
`v6` retains call-started and call-ended facts as visible room timeline entries.
`v7` also retains Threading Mode changes as visible room timeline entries;
older snapshots omitted those rows and therefore cold-replay under the new
contract. Its current schema also stores active pinned-message associations by room.
Those associations reference canonical timeline messages instead of copying
message content; retraction removes the association during projection.

Snapshot loads and replay frontiers are projection-local. A successful restore
starts that projection's ordered consumer at one greater than its cutoff. A
missing, invalid, or unavailable snapshot cold-replays only its owning
projection. Projections without matching EVT history have no state to
accelerate and do not publish zero-cutoff generations. Credential-bearing user
state is owned by `UserAuthProjection` and cold-replays from focused user event
families.

The projector framework atomically captures each projection's explicit
protobuf state with its latest applied logical EVT sequence. Room Timeline
retains one body-state entry per message: the current encrypted envelope and
EVT sequence are inline, while a sequence slice is allocated only after an
edit. Its snapshot codec preserves the complete body-event sequence history.

Mentionables retains encrypted login source events and wrapped DEK records
rather than plaintext handles or lookup digests. The Users codec retains
encrypted login, display-name, and verified-email values, lookup digests,
wrapped DEK records, and non-secret profile metadata. Its schema has no fields
for password verifiers, authentication generations, external identity
subjects, or OAuth consent.

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
`internal/evtstream` owns Chatto's metadata key, format, generation, and
validation. Core composition passes its resolver into projector restore
configuration. The projector binds the resolved value to its run and captures
it with snapshot state and cutoff.

Capture checks the current incarnation immediately before and after the
projection barrier, performs no NATS I/O while holding that barrier, and
refuses publication if the identity differs. The worker publishes the captured
value. A transient lookup failure during best-effort restore falls back to the
identity validated at configuration, so publication can recover after cold
replay without accepting an actual stream recreation. Persistence mechanics
treat the identity as opaque.

`core.projection_snapshot_retention` defaults to seven days. NATS applies it as
the Object Store TTL. S3 uses a bounded age-expiry pass after daily publication
unless `core.projection_snapshot_s3_cleanup` is disabled for an external
lifecycle policy. S3 deletion requires the exact generation-key grammar,
expected snapshot content type, and private object-purpose marker. Snapshot and
expiry failures are logged and never affect core readiness or EVT-backed
reconstruction. Legacy cohort paths remain outside application S3 expiry.

| Projection | Contract | Payload store | Pointer store | Publication |
| ---------- | -------- | ------------- | ------------- | ----------- |
| Room Directory, Notification Decisions, Server Config, Room Group Layout, Call State, Reactions, Content Keys, RBAC | `v1` per projection | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Elected publisher checks hourly; cold/delta replay publishes immediately and unchanged state refreshes at 23 hours. Notification Decisions caps restore and publication at the notification worker's full acknowledged EVT floor |
| Notifications | `v2` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Binds snapshots to the independent `NOTIFICATIONS` stream identity and sequence |
| Threads, Mentionables | `v2` per projection | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Threads restores channel-room identity, canonical message-to-thread mappings, and post-time interaction causes. The key-shredding request boundary invalidates pre-request snapshot contracts |
| Room Timeline | `v7` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Restores call lifecycle and Threading Mode change rows plus active pin associations, and rebuilds Slow Mode's latest-original-post index; earlier contracts remain isolated |
| Assets | `v3` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Restores explicit exclusive attachments while retaining first uploader-authored message-reference ownership for older histories; earlier snapshots remain independently addressable during rollout and rollback |
| Users (profile state only) | `v4` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | The key-shredding request boundary invalidates `v2` and `v3` snapshots |

## Registered projections

| Runtime area       | Registered projector | Consumes                                                   | Read models / primary readers                                                             |
| ------------------ | -------------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Room directory     | Room Directory       | `evt.room.>`                                               | `RoomCatalogProjection`, `RoomMembershipProjection`, `RoomBanProjection`; room metadata including Slow Mode and Threading Mode, room/member queries, room authorization, and Universal-room effective membership |
| Notification derivation and privacy | Notification Decisions | Focused message/reaction sources plus account, room membership/kind, room-group placement, RBAC, server/room-group/room notification-policy, and thread-follow facts | Current decision state plus a second in-memory evaluator at the worker position; compact intervening event deltas preserve exact source and administrative boundaries without per-message full-state copies. It resolves a room's group at the exact source sequence. Snapshot restore/publication and delta release follow the shared consumer's confirmed acknowledgement floor |
| Notification list | Notifications | `notifications.signalled`, `notifications.read`, `notifications.removed`, `notifications.alert_resolved` | Current exact occurrence state, minimal anti-recreation tombstones, source-signal stream sequences for secure deletion, semantic expiry, list/count reads, realtime replacement, and push-delivery idempotency |
| Room organization  | Room Group Layout    | `evt.group.>`, `evt.layout.>`                              | `RoomGroupProjection`, `RoomLayoutProjection`; sidebar groups, sidebar links, and mixed sidebar item ordering |
| Room timeline      | Room Timeline        | `evt.room.>`, `evt.user.*.user_key_shredding_requested`, `evt.user.*.user_key_shredded` | Visible room timeline including call start/end and Threading Mode change facts, latest message bodies, tombstone timestamps, hidden echoes, current attachment-bearing message index, direct message-post lookup, active canonical pinned-message associations, the latest pin-fact marker per room, and latest original post by room and author |
| Assets             | Assets               | `evt.asset.>`, legacy `evt.room.*.asset_*`, `evt.room.*.message_body` | `AssetModel`; detached asset declaration/room/processing/deletion snapshots, derivative graph, exclusive message attachment/author references, public link-preview image references, and legacy uploader-matched first-reference compatibility |
| Threads            | Threads              | `evt.room.*.room_created`, `evt.room.*.room_deleted`, `evt.room.*.thread_created`, `evt.room.*.thread_followed`, `evt.room.*.thread_unfollowed`, `evt.room.*.message_posted`, `evt.room.*.message_edited`, `evt.room.*.message_retracted`, `evt.user.*.user_key_shredding_requested`, `evt.user.*.user_key_shredded` | Per-thread existence, reply logs, summaries, participants, reply counts, follow state, canonical message-to-thread mappings, and account-to-thread interaction relationships with typed post-time causes |
| Reactions          | Reactions            | `evt.room.>`                                               | Current canonical per-message reaction sets, echo-to-original reaction aliases, and room-scoped snapshot OCC positions; intentionally broad so reaction writes can OCC against the room tail |
| Voice calls        | Call State           | `evt.room.>`                                               | Current LiveKit call session, participants, active room IDs, and room-scoped snapshot OCC positions |
| Server/user config | Server Config        | `evt.config.>`, selected user cleanup/preference facts     | `ConfigModel`; server config, branding refs, user preferences, explicit per-signal-class server/room-group/room notification delivery modes, blocked usernames; group deletion leaves group override maps inert; historical coarse notification-level facts decode but have no projected effect |
| Users              | Users                | `evt.user.>`                                               | `UserModel`; account/profile/custom-status state, verified emails, lookup digests, and encrypted user PII |
| User authentication | User Auth            | Focused account, password, external-identity, consent, deletion, and key-shredding user facts | `UserModel`; password verifiers, auth generations, external identity links, and OAuth consent keyed by stable client ID (legacy facts by redirect origin); always cold-replayed |
| OAuth clients        | OAuth Clients        | `evt.oauth_client.>`                                       | `OAuthClientModel`; validated metadata and callback origins for successfully authorized clients, distinct authorized-user counts, and administrative default/trusted/blocked policy; always cold-replayed |
| Invitations         | Invitations          | `evt.invitation.>`                                       | `InvitationModel`; immutable constraints, redemption count, revocation state, and administrator listings; always cold-replayed |
| Content keys       | Content Keys         | `evt.user.*.dek_generated`, `evt.user.*.user_key_shredding_requested`, `evt.user.*.user_key_shredded` | `UserModel`; active and historical user DEK epochs, legacy-purpose fallback, and key references used by crypto-shredding |
| RBAC               | RBAC                 | `evt.rbac.>`                                               | `RBACModel`; roles, role order, assignments, and scoped allow/deny decisions                |
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
logical subjects above for waits and diagnostics. It applies channel room
lifecycle and message facts for interaction authorization and skips unrelated
room facts before `Apply`.

Room Timeline, Threads, Assets, and Notification Decisions physically replay
through one `evt.>` filter. Their narrower logical subjects still determine
readiness and application. The projector rejects other subjects before
protobuf decoding. This avoids JetStream's expensive multi-filter scans while
preserving independent consumers and projection-local replay frontiers.

`AssetModel` is the sole production reader of every asset-derived index and
owns asset-projector readiness. Cross-package callers receive a detached
`AssetState` containing declaration, room, processing, and deletion state from
one projection generation. Explicit asset attachments establish immutable
message, room, and author ownership; message-body facts supply an uploader-matched
first-reference fallback for older histories plus public link-preview references. Room Timeline
retains only timeline rendering, body lifecycle, tombstone, echo, and current
room-file indexes; it does not duplicate asset lifecycle state. Message-body
writers wait for both projectors before returning.

`UserProjection` retains encrypted user fields and their AAD metadata. The user
and mentionable projections decrypt login and email values only transiently
while applying events to derive in-memory lookup digests; neither plaintext nor
the digests are persisted in `EVT`. Read hydration decrypts profile PII with
request-scoped DEK reuse. KMS and decryption failures remain operational errors
rather than appearing as missing or deleted users.
`UserAuthProjection` is independently locked, registered, and replay-guarded.
`UserModel` reads profile state from `UserProjection` and credential,
external-identity, consent, and auth-generation state from
`UserAuthProjection`, giving domain callers one user boundary while snapshot
serialization cannot reach authentication state.

Bot account kind and owner ID are durable user-aggregate fields projected by
`UserProjection`; it also maintains the current owner-to-bot index used for
management, reassignment, and cascade deletion. `UserAuthProjection` replays
the latest bot API-key verifier and creation/rotation timestamps from EVT. It
also closes process-local realtime watchers whose non-secret verifier generation
is superseded, so every replica terminates stale bot connections when it
observes the durable rotation.
The raw API key is never a projection value, snapshot field, or retrievable
resource.
