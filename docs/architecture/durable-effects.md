# Durable Effect Inventory

Key files: [`pkg/events/durable_worker.go`](../../pkg/events/durable_worker.go),
[`cli/internal/evtstream/effects.go`](../../cli/internal/evtstream/effects.go),
[`cli/internal/core/durable_delivery.go`](../../cli/internal/core/durable_delivery.go),
[`cli/internal/core/notification_materializer.go`](../../cli/internal/core/notification_materializer.go),
[`cli/internal/video/unit.go`](../../cli/internal/video/unit.go)

Related decisions: [ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md),
[ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md), and
[ADR-066](../adr/ADR-066-durable-asset-processing-runtime-unit.md),
[ADR-069](../adr/ADR-069-explicit-durable-consumer-lifecycle.md), and
[ADR-076](../adr/ADR-076-deterministic-notification-occurrences.md).

Some durable facts require work in a different storage system or external
service. The table records the current execution and recovery contract. A
durable trigger means unfinished work can be rediscovered after a crash; it
does not by itself guarantee that every implementation currently performs that
recovery.

`events.DurableWorker` provides application-neutral bounded pull-consumer
execution with progress heartbeats, delayed retry, confirmed acknowledgements,
poison termination, and reconnect-safe fetching. Chatto owns each consumer's
durable name, filters, ack policy, event decoding, projection barrier,
idempotency, and terminal facts. Video processing and notification
materialization use application-owned durable consumers through this boundary.

`internal/evtstream` standardizes consumer creation and worker wiring. Each
application site still owns its name, filters, handler, retry policy, and
lifecycle.

Transient fetch failures retry in place. A deleted consumer stops its worker so
the owning core process or supervised runtime unit can recreate the declared
consumer instead of polling a stale handle indefinitely. Retry failures are
logged on the first and exponentially sparse later attempts; terminated poison
deliveries are always logged. Shutdown cancels outstanding pulls before active
handlers and schedules redelivery beyond the maximum pull lifetime, preventing
an orphaned server-side pull from reclaiming its own handoff.

Owner-only admin diagnostics classify the seven known Chatto durable queues from
their JetStream consumer state without adding process-local health as a source
of truth. Waiting pulls demonstrate availability. Ack-pending deliveries
without a waiting pull are unconfirmed because they may be actively handled or
awaiting crash recovery; a present queue with neither is stalled. Unresolved
redelivery counts remain informational rather than a current failure flag.

| Effect | Durable fact or invariant | Immediate execution | Restart and multi-replica behavior | Current status |
| ------ | ------------------------- | ------------------- | ---------------------------------- | -------------- |
| Ended-call E2EE key shredding | `CallEndedEvent`; the call ID deterministically identifies the KMS key | The committing request attempts to shred only the ended call's key | Shared `chatto-call-key-cleanup-v1` pull-consumer replicas retry idempotent shredding, including facts committed by other replicas | Recoverable; failure, restart, and late-replica commit paths are covered by focused tests |
| LiveKit participant eviction after membership loss | `UserLeftRoomEvent` plus paired call leave/end facts for current writers | The membership mutation best-effort calls LiveKit `RemoveParticipant` after projection catch-up | The elected reconciler compares LiveKit rooms with current call projection state; unmatched historical calls get a durable reconciliation `CallEndedEvent` before eviction | Recoverable while LiveKit remains observable; room-not-found is treated as successful cleanup |
| Call-key creation compensation | A successful `CallStartedEvent` retains the newly created key; an append conflict means the pre-created key is unused | The call mutation creates the key before EVT append and shreds it after a failed/conflicting append | Failed compensation is logged; no durable fact identifies a key that was created but never committed | Best-effort compensation with an orphan-key gap |
| User DEK creation compensation | A successful `UserDEKGeneratedEvent` declares the KEK and wrapped content-key references | Initial DEK generation creates both key records before EVT append and attempts to shred both after append failure or conflict | Compensation errors are discarded; no durable fact identifies key records that were created but never committed | Best-effort compensation with an orphan-key gap |
| Video derivative processing | `AssetProcessingStartedEvent`, committed atomically with the owning message; `AssetProcessingSucceededEvent` or `AssetProcessingFailedEvent` is terminal | An `asset-processing` runtime unit receives the Started fact through the shared `chatto-asset-processing-v1` pull consumer, runs bounded ffmpeg work, uploads a thumbnail and HLS segments, then publishes the terminal manifest; animated GIF loops upload one MP4 derivative | Explicit ack follows projected terminal state. Crashes and shutdown before terminal state cause redelivery; replicas share deliveries. Existing terminal state makes redelivery an ack-only no-op. A startup compatibility pass backfills only pre-queue messages with no Started marker. Exceeding the fixed 30-minute processing budget records a terminal failure instead of redelivering forever | Work discovery and ownership are durable and at least once. Terminal OCC prevents manifest replacement, but interrupted or losing attempts, unconfirmed success, and uncommitted derivative creation can still leave orphaned storage |
| Asset and branding binary creation compensation | `AssetCreatedEvent` or a server logo/banner event declares the stored object and its owner | Completed uploads and branding uploads write NATS/S3 bytes before the durable event or pointer update; attachment upload failure attempts immediate deletion | Attachment cleanup failure is ignored, and a branding upload abandoned before `SetServerLogo`/`SetServerBanner` has no durable owner or discovery path | Best-effort compensation with orphan-object gaps |
| Asset binary and transform-cache deletion | `AssetAttachedEvent` binds an upload to one exact room/message; `AssetDeletedEvent` makes projected reads and signed asset resolution reject the asset; the asset ID locates the canonical aggregate's durable creation metadata | Message deletion and attachment removal verify the exact durable attachment before tombstoning or touching bytes; pending-upload expiry atomically verifies that no attachment won; account and derivative cleanup record deletion before removing NATS/S3 bytes and cached transforms | Shared `chatto-asset-cleanup-v1` pull-consumer replicas load storage metadata from creation facts and retry idempotent binary/cache deletion. A source-video tombstone also re-reads its durable HLS manifest and tombstones any still-live HLS children, repairing deletion by an older HLS-unaware replica; beta room-scoped facts without a canonical creation aggregate are skipped | Recoverable for canonical message-owned asset deletion facts and mixed-version HLS source cleanup; duplicate references from before explicit attachment events cannot delete the first owner's asset; beta room-scoped cleanup and failed-generation derivatives without a deletion fact remain best-effort |
| Obsolete or retracted message-body erasure | `MessageEditedEvent`, `MessageRetractedEvent`, and hidden echo state make prior `MessageBodyEvent` payloads obsolete | The mutation calls JetStream `SecureDeleteMsg` for projected obsolete body sequences | After projections catch up at boot, every replica derives all obsolete body sequences and repeats idempotent secure deletion | Recoverable from EVT projection state; boot work is not lease-owned |
| User content-key and KEK shredding | `UserKeyShreddingRequestedEvent` is committed under the exact user-aggregate OCC tail and is the logical tombstone boundary; immutable `UserDEKGeneratedEvent` facts plus surviving runtime DEK records identify the deletion set; `UserKeyShreddedEvent` records physical completion | Account deletion aborts unless the request is durable; the command waits for privacy-sensitive projections through it, shreds every discovered wrapping key before deleting any DEK record, and appends completion | Shared `chatto-user-key-shredding-v1` pull-consumer replicas reconstruct targets and redeliver the request until deletion and completion succeed; KEK-first ordering preserves discovery across partial attempts, and existing completion is an ack-only no-op | Crash-safe, recoverable, at-least-once effect with deterministic failure-window and concurrent-key-generation coverage |
| Runtime credential cleanup after security changes | Password, account-deletion, and external-identity events advance durable user/auth state before stored sessions and tokens are deleted | The request scans and deletes matching `RUNTIME_STATE` credentials and publishes transient session termination | Credential generation prevents stale credentials from authenticating new requests or reconnects; stale records remain cleanup debt, and an already-open realtime connection depends on best-effort session termination | New authentication is durably revoked; physical cleanup and immediate live disconnect are best-effort |
| Push-credential cleanup after account deletion | The existing `UserAccountDeletedEvent` is the permanent account boundary and registration fence; no push-specific EVT fact is introduced | After committing deletion, the request attempts idempotent owner-first removal of every endpoint claim and subscription record | Shared `chatto-user-push-subscription-cleanup-v1` replicas redeliver partial or failed per-account cleanup until it succeeds. One renewable lease leader performs startup/periodic reconciliation without a fixed whole-pass deadline: it scans each current subscription and owner keyspace once, consults the exact permanent deletion fact, and repairs late writes, malformed records, and owner-only crash state | Recoverable, at-least-once physical erasure with partial-listing, redelivery, orphan-owner, late-write, and large-keyspace coverage |
| Notification attention materialization and push delivery | Message OCC retries persist resolved mention semantics on the existing message fact. That same fact selects the exact source-time room members with `message.read` for a root channel message. The Notification Decisions projection reconstructs compact account, room, RBAC, policy, and thread state at each delivered source sequence. No notification-only event or marker is added to `EVT`. The materializer writes Badge output to a bounded `RUNTIME_STATE` marker or appends rich lifecycle facts directly to the bounded `NOTIFICATIONS` stream | Message post requests do not wait for recipient fanout. The shared `chatto-notification-materializer-v1` EVT consumer derives deterministic output at the exact EVT boundary. It commits monotonic Badge markers with bounded concurrent KV OCC writes and one collective applied-revision barrier, and it commits high-fanout lifecycle output in bounded batches. It waits for local marker or occurrence projection state before it publishes content-free recipient invalidations with one flush, then confirms the source acknowledgement only after all idempotent writes succeed. A new occurrence can include a best-effort local-sound candidate; Badge cannot. Retraction, reaction removal, visibility loss, and account deletion remain existing domain facts. The separate `chatto-notification-alert-delivery-v1` consumer filters `notifications.signalled` through `events.DurableWorker`, waits for the projection at the delivered sequence, fences the materializer and current policy/authorization projections, revalidates current state, target visibility, subscription ownership, and DND, then sends Web Push with the click route selected from each browser subscription and appends one terminal `alert_resolved` fact before acknowledging | Deterministic recipient/source/signal-class identity, monotonic source-sequence Badge markers, a worker-position evaluator with compact pending deltas, removal tombstones, watched read and visibility boundaries, and projection fences make cross-replica ordering explicit. One startup repair plus scope-indexed watcher updates completes interrupted read-boundary handshakes without per-event global scans. Cleanup-only tombstone coordinates survive through the 24-hour physical grace even after semantic state expires at 90 days. There is no notification queue or prepared-work KV. `RUNTIME_STATE`, `NOTIFICATIONS`, and durable consumer state are backed up. An immutable two-minute push deadline and provider TTL bound delivery after contention, retry, or restore. Terminal projected state makes redelivery an ack-only no-op, but a crash after provider acceptance and before terminal persistence can duplicate a push. Local sound and transient invalidations are not recovered after failed publication or restart | Badge and occurrence creation/removal and push retry are recoverable and at least once. Local sound and live convergence hints are best-effort. Materializer and push-consumer state are included in owner-only durable-worker diagnostics |
| Server branding replacement cleanup | Server logo/banner set or cleared events make the old asset unreachable from projected configuration | The request deletes the prior NATS/S3 object and cached transforms after the config event commits | No durable cleanup worker scans superseded branding assets | Durable pointer update with best-effort orphan cleanup |

The notification consumer also processes configured-owner verified-email facts.
It retries materializing the durable RBAC owner role, while live authorization
recognizes only that role. Notification Decisions keeps a second in-memory
evaluator at the worker position and journals compact facts that the live
projection has seen first; acknowledged deltas and boundaries are then
released.

This keeps transient role-assignment or acknowledgement failures from
creating live/event-time divergence or redelivery gaps without serializing
server-wide state per message or adding notification-only EVT facts. Snapshot publication
also defers whenever a capture crosses the full notification-consumer floor,
including while a non-boundary worker fact is pending, preserving the
repository's last generation that restart can safely accept. Idle-tail
advancement is reconstructed after restart as the full-EVT prefix immediately
before the earliest fact following the filtered consumer's sparse raw AckFloor.

During a rolling feature upgrade, the materializer consumer name and filter set
act as an immutable capability generation. A new source-event schema uses a new
consumer generation; changing filters under an existing name fails startup
closed. Unknown source-event and prepared-target protobuf branches fail
retryably rather than acknowledging or deleting their work, allowing JetStream
to hand the delivery to a capable replica without adding notification-only EVT
facts.

Observability is currently domain-specific. Call reconciliation records its
consecutive LiveKit listing failures in `MEMORY_CACHE`. Owner-only asset-cleanup
diagnostics derive queue depth and delivery progress directly from the shared
JetStream consumer; they do not infer worker liveness from broker response
timestamps or transient pull requests. Other effects still
primarily emit structured logs, and there is no common metric/status contract
for pending effect count, oldest pending age, retry attempts, terminal failures,
or effect-consumer lag.

Failure coverage is also domain-specific. Call cleanup and message-owned asset
deletion have commit/failure, restart, independent-work, and late-replica
coverage; video processing covers durable delivery ack/retry decisions,
pre-queue backfill, exact-event confirmation
after ambiguous terminal publication, terminal manifest races, and bounded
prompt cleanup of failed generations.

Message-body cleanup covers immediate secure deletion after edits and
retractions. User-key shredding covers request-append failure, logical
fail-closed state before physical deletion, partial deletion, missing
completion, idempotent retry, and shutdown handoff to another replica.

Notification occurrence tests cover durable work recovery, replay-safe
per-signal-class identity, stream projection and snapshot behavior,
read/materialization ordering, dismissal tombstones, and direct push-consumer
retry. Branding cleanup and the message-body boot
sweep do not have equivalent crash-and-recovery coverage.

The call-key, user-DEK, and asset-creation compensation paths likewise lack
durable tests for cleanup failure followed by restart.

Cross-domain follow-up work is tracked in
[#1377](https://github.com/chattocorp/chatto/issues/1377), with separate issues
for physical asset deletion, user-key shredding, video ownership, and the
notification follow-up observability.

Transient `live.sync.>` publication is intentionally excluded from recovery:
clients treat those messages as invalidations and recover authoritative state
through projected reads. Auth email delivery is also outside this inventory:
registration, verification, and reset credentials live in `RUNTIME_STATE`, with
durable EVT records serving as security audit facts rather than an email queue.
