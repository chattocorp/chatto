# Projection Inventory

Key files: [`cli/internal/core/projection_wiring.go`](../../cli/internal/core/projection_wiring.go), [`cli/internal/core/server_content_view.go`](../../cli/internal/core/server_content_view.go), [`pkg/events/projector.go`](../../pkg/events/projector.go), [`pkg/events/component_projection.go`](../../pkg/events/component_projection.go), [`pkg/events/projection_checkpoint.go`](../../pkg/events/projection_checkpoint.go), [`cli/internal/projectionsnapshot/cohort.go`](../../cli/internal/projectionsnapshot/cohort.go), [`cli/internal/search/bleve/projection.go`](../../cli/internal/search/bleve/projection.go), [`cli/internal/core/asset_processing_runtime.go`](../../cli/internal/core/asset_processing_runtime.go), [`cli/internal/core/projection_subjects_test.go`](../../cli/internal/core/projection_subjects_test.go), and [`proto/chatto/core/projection/v1`](../../proto/chatto/core/projection/v1)

Projections are derived read models rebuilt from an event log. Most live in
memory. Optional providers can own disposable locally checkpointed indexes.
`ServerContentView` coordinates the client-readable models that derive from
`EVT`. It has one ordered `evt.>` consumer, one apply barrier, one failure and
readiness state, and one exact applied EVT sequence. Focused models remain
separate components behind that barrier.

`initializeCoreProjections` registers each top-level projector once with a
stable machine-readable key and a human display name. `NewChattoCore` installs
that registry into the core runtime. The registry contains six projectors:
`server_content_view`, `notification_decisions`, `notifications`, `user_auth`,
`invitations`, and `oauth_clients`. Each registration also declares whether
that key is eligible for shared snapshots.

Core couples each projection pointer to its exact projector as one typed
`events.ProjectionHandle` from the independently versioned incubation module.
Focused content handles bind to the `ServerContentView` projector. Thus,
existing domain APIs keep narrow model types while all included state uses the
same wait and failure boundary. Chatto-specific keys, names, memory estimates,
diagnostics, and snapshot policy remain in the core registration layer. This
boundary follows [ADR-056](../adr/ADR-056-extractable-nats-event-sourcing-framework.md).

`ChattoCore.Run` starts one process-local ordered consumer for each registered
projector. `ServerContentView` and the four independent EVT projectors read
`EVT`. Notifications reads `NOTIFICATIONS`. Each projector owns its physical
filters, replay progress, failure state, and readiness. Chatto waits for all
six registered projectors before it completes boot.

Writers wait for the relevant projector sequence before returning
read-your-writes. Projection-aware domain models keep the projector references
needed for those waits; the `ChattoCore` facade does not mirror every registered
projector.

`InvitationModel` derives a process-local 16-character invite-token lookup
index from the cold-replayed Invitation projection. The index contains no
additional durable facts or stored bearer values and is rebuilt whenever the
append-only projected invitation identity count changes.

`CallModel`, `AssetModel`, and `UserModel` own their projection reads for domain
logic and API adapters. Call state, assets, user profiles, and content keys are
components of `ServerContentView`. Authentication remains an independent,
cold-replayed projection. Call token access material binds the call ID and E2EE
key to one revalidated content-view generation. Active-call and asset API
mapping use detached component snapshots.

Room timeline message hydration obtains deletion and channel-echo metadata as
one detached snapshot through `RoomTimelineReadModel`; ConnectAPI does not read
that component directly. `RoomModel` is the sole production owner of the Room
Directory, Room Group Layout, Room Timeline, Threads, and Reactions component
APIs. These components use the shared content-view projector. Threads also
derives channel message-to-root
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
supplies the unchanged `evtv1.Event` protobuf decoder, while
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
[ADR-066](../adr/ADR-066-durable-asset-processing-runtime-unit.md), and
[ADR-084](../adr/ADR-084-separate-internal-protobufs-by-storage-contract.md),
[ADR-088](../adr/ADR-088-componentized-projections-behind-one-apply-barrier.md),
and [ADR-089](../adr/ADR-089-server-content-view.md).

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

The 0.5 internal protobuf package split changes full protobuf names and selects
new snapshot contract IDs. A server ignores older snapshots, cold-replays EVT,
and writes new snapshots. It does not rewrite stored EVT or runtime-state data.

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

Snapshot loads and replay frontiers are projector-local. A successful restore
starts that projector's ordered consumer at one greater than its cutoff. A
missing, invalid, or unavailable scalar snapshot cold-replays only its owning
projector. A missing or invalid `ServerContentView` cohort cold-replays the
complete content view. Chatto does not combine restored and replayed content
components. Credential-bearing user state is owned by `UserAuthProjection` and
cold-replays from focused user event families.

The projector framework atomically captures a projection snapshot cohort and
its applied EVT sequence. Each component serializes its own explicit protobuf
state as one independent encrypted object. One encrypted manifest binds the
required component keys, contracts, object references, EVT stream identity,
and shared cutoff. The repository publishes one pointer only after all objects
are durable. It retains current and previous complete cohorts and uses KV
revision OCC for publication.

Room Timeline retains one body-state entry per message: the current encrypted
envelope and EVT sequence are inline, while a sequence slice is allocated only
after an edit. Its component codec preserves the complete body-event sequence
history.

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
HKDF key derived from `core.secret_key`. Chatto stores them under the reserved
projection snapshot namespace in the dedicated NATS `PROJECTION_SNAPSHOTS`
Object Store or the configured S3 bucket. Encrypted current and previous
pointers live in `RUNTIME_STATE` and use KV revision OCC for both payload
backends. The opaque pointer locator is scoped by projector and contract.

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
| Server Content View | `v1` componentized-projection contract plus component contracts | `PROJECTION_SNAPSHOTS` or configured S3; one manifest and one object for each initial component | One encrypted `server_content_view` pointer in `RUNTIME_STATE` with KV revision OCC | Elected publisher checks hourly; cold/delta replay publishes immediately and unchanged state refreshes at 23 hours |
| Notification Decisions | `v2` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Elected publisher checks hourly; cold/delta replay publishes immediately and unchanged state refreshes at 23 hours |
| Notifications | `v2` | `PROJECTION_SNAPSHOTS` or configured S3 | Encrypted per-projection `RUNTIME_STATE` pointer with KV revision OCC | Binds snapshots to the independent `NOTIFICATIONS` stream identity and sequence |

## Registered projections

| Runtime area       | Registered projector | Consumes                                                   | Read models / primary readers                                                             |
| ------------------ | -------------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Client-readable EVT state | Server Content View | `evt.>` | Room directory, server configuration, room-group layout, room timeline, calls, assets, threads, reactions, user profiles, content keys, RBAC, and mentionable identities. Each focused component declares the event subjects that it reduces. |
| Notification derivation and privacy | Notification Decisions | Focused message/reaction sources plus account, room membership/kind, room-group placement, RBAC, server/room-group/room notification-policy, and thread-follow facts | Compact current decision state. The materializer waits until this projection includes its source fact, then uses all state that is current on that replica. Snapshots follow the standard projection lifecycle without a durable-worker cutoff |
| Notification list | Notifications | `notifications.signalled`, `notifications.read`, `notifications.removed`, `notifications.alert_resolved` | Current exact occurrence state, minimal anti-recreation tombstones, source-signal stream sequences for secure deletion, semantic expiry, list/count reads, realtime replacement, and push-delivery idempotency |
| User authentication | User Auth            | Focused account, password, external-identity, consent, deletion, and key-shredding user facts | `UserModel`; password verifiers, auth generations, external identity links, and OAuth consent keyed by stable client ID (legacy facts by redirect origin); always cold-replayed |
| OAuth clients        | OAuth Clients        | `evt.oauth_client.>`                                       | `OAuthClientModel`; validated metadata and callback origins for successfully authorized clients, distinct authorized-user counts, and administrative default/trusted/blocked policy; always cold-replayed |
| Invitations         | Invitations          | `evt.invitation.>`                                       | `InvitationModel`; immutable constraints, redemption count, revocation state, and administrator listings; always cold-replayed |

Registered projector keys are used by metrics and automation. Registered names
match the admin projection diagnostics. Composite projections expose nested
read models, but only their parent projector is started by `ChattoCore.Run`.

Independent projectors isolate snapshot availability, replay cost, status,
lag, failure, and read-your-writes waiters for state outside the content view.
`ServerContentView.Subjects()` declares `evt.>` as its readiness contract.
Each component has a focused subject declaration. The componentized projection
prepares an EVT record only for matching components. The projector advances
the shared sequence after all matching component mutations commit.

Permission resolution and permission explanation run inside one
`ServerContentView` read transaction. Account state, bot ownership, room
metadata, membership, bans, Room Group placement, and RBAC state therefore
come from one applied EVT generation. Request-time authorization still uses
the subject-tail validation and aggregate OCC procedure from ADR-087.

`AssetModel` is the sole production reader of every asset-derived index and
uses content-view readiness. Cross-package callers receive a detached
`AssetState` containing declaration, room, processing, and deletion state from
one projection generation. Explicit asset attachments establish immutable
message, room, and author ownership; message-body facts supply an uploader-
matched first-reference fallback for older histories plus public link-preview
references. Room Timeline retains only timeline rendering, body lifecycle,
tombstone, echo, and current
room-file indexes; it does not duplicate asset lifecycle state. Message-body
writers use the one content-view wait before they return.

`UserProjection` retains encrypted user fields and their AAD metadata. The user
and mentionable components decrypt login and email values during mutation
preparation. They commit in-memory lookup changes only after all matching
components prepare successfully. Neither plaintext nor the digests are
persisted in `EVT`. Read hydration decrypts profile PII with
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
the active bot API-key IDs, names, verifiers, and creation times from EVT.
Historical create and rotation events without key metadata project as the
synthetic `legacy` default key. A historical rotation replaces every active
verifier during replay. Current commands do not write replace-all rotations.
A revocation removes only the selected verifier. When either fact takes effect,
the projection closes process-local realtime watchers that used a removed
verifier. A rollback-visible key uses a rotation-shaped revocation fence so an
older binary cannot restore the raw revoked key after rollback. The projection
also replays the active incoming webhook IDs, names, verifiers, and creation
times. A
historical verifier-replacement fact from the unreleased
implementation replaces only the selected verifier during replay. Current
commands do not write this fact. Revocation removes only the selected webhook.
A webhook fact from the first unreleased implementation has no ID and projects
to the synthetic `legacy` ID.
The raw API key and incoming webhook credential are never projection values,
snapshot fields, or retrievable resources.
