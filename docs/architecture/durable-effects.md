# Durable Effect Inventory

Key files: [`cli/internal/events/incremental_effect_consumer.go`](../../cli/internal/events/incremental_effect_consumer.go), [`cli/internal/core/call_model.go`](../../cli/internal/core/call_model.go), [`cli/internal/core/asset_model.go`](../../cli/internal/core/asset_model.go), [`cli/internal/core/message_body_cleanup.go`](../../cli/internal/core/message_body_cleanup.go), [`cli/internal/video/service.go`](../../cli/internal/video/service.go)

Related decisions: [ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md)
and [ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md).

Some durable facts require work in a different storage system or external
service. The table records the current execution and recovery contract. A
durable trigger means unfinished work can be rediscovered after a crash; it
does not by itself guarantee that every implementation currently performs that
recovery.

`IncrementalEffectConsumer` provides the shared low-level mechanism for
process-local cursoring and independent failed-effect retries over a filtered
EVT lane. Domain models still own lease selection, polling cadence, backoff,
idempotent handlers, logging, and lifecycle. Call-key cleanup and asset cleanup
use separate domain-owned consumers.

| Effect | Durable fact or invariant | Immediate execution | Restart and multi-replica behavior | Current status |
| ------ | ------------------------- | ------------------- | ---------------------------------- | -------------- |
| Ended-call E2EE key shredding | `CallEndedEvent`; the call ID deterministically identifies the KMS key | The committing request attempts to shred only the ended call's key | The elected call reconciler incrementally scans `call_ended` facts by global EVT sequence and retries idempotent shredding, including facts committed by other replicas | Recoverable; failure, restart, and late non-holder commit paths are covered by focused tests |
| LiveKit participant eviction after membership loss | `UserLeftRoomEvent` plus paired call leave/end facts for current writers | The membership mutation best-effort calls LiveKit `RemoveParticipant` after projection catch-up | The elected reconciler compares LiveKit rooms with current call projection state; unmatched historical calls get a durable reconciliation `CallEndedEvent` before eviction | Recoverable while LiveKit remains observable; room-not-found is treated as successful cleanup |
| Call-key creation compensation | A successful `CallStartedEvent` retains the newly created key; an append conflict means the pre-created key is unused | The call mutation creates the key before EVT append and shreds it after a failed/conflicting append | Failed compensation is logged; no durable fact identifies a key that was created but never committed | Best-effort compensation with an orphan-key gap |
| User DEK creation compensation | A successful `UserDEKGeneratedEvent` declares the KEK and wrapped content-key references | Initial DEK generation creates both key records before EVT append and attempts to shred both after append failure or conflict | Compensation errors are discarded; no durable fact identifies key records that were created but never committed | Best-effort compensation with an orphan-key gap |
| Video derivative processing | `AssetProcessingStartedEvent`; `AssetProcessingSucceededEvent` or `AssetProcessingFailedEvent` is terminal | A process-local callback starts bounded ffmpeg work, creates temporary MP4 renditions, uploads a thumbnail and HLS segments, then publishes the terminal manifest; animated GIF loops upload one MP4 derivative. Failure publication has an independent bounded context and precedes separately bounded prompt cleanup | Each enabled video service scans the asset projection at boot for assets without a completed/failed outcome, including `STARTED`-only attempts, and resubmits them. Terminal event OCC prevents later manifest replacement. A losing or failed attempt receives only bounded prompt cleanup. An ambiguous success append is checked by exact event ID; if confirmation also fails, its output is retained | Processing ownership and failed-generation cleanup remain best-effort until a durable worker design replaces them. Duplicate processing, interrupted cleanup, an unconfirmed success that did not commit, and an uncommitted derivative creation can leave orphaned storage |
| Asset and branding binary creation compensation | `AssetCreatedEvent` or a server logo/banner event declares the stored object and its owner | Completed uploads and branding uploads write NATS/S3 bytes before the durable event or pointer update; attachment upload failure attempts immediate deletion | Attachment cleanup failure is ignored, and a branding upload abandoned before `SetServerLogo`/`SetServerBanner` has no durable owner or discovery path | Best-effort compensation with orphan-object gaps |
| Asset binary and transform-cache deletion | `AssetDeletedEvent` makes projected reads and signed asset resolution reject the asset; the asset ID locates the canonical aggregate's durable creation metadata | Message deletion, attachment removal, account cleanup, pending-upload expiry, and derivative cleanup delete NATS/S3 bytes and cached transforms after recording deletion | The elected `asset_cleanup` worker consumes canonical deletion facts, loads storage metadata from their creation facts, and retries idempotent binary/cache deletion. A source-video tombstone also re-reads its durable HLS manifest and tombstones any still-live HLS children, repairing deletion by an older HLS-unaware replica; beta room-scoped facts without a canonical creation aggregate are skipped | Recoverable for canonical message-owned asset deletion facts and mixed-version HLS source cleanup; beta room-scoped cleanup and failed-generation derivatives without a deletion fact remain best-effort |
| Obsolete or retracted message-body erasure | `MessageEditedEvent`, `MessageRetractedEvent`, and hidden echo state make prior `MessageBodyEvent` payloads obsolete | The mutation calls JetStream `SecureDeleteMsg` for projected obsolete body sequences | After projections catch up at boot, every replica derives all obsolete body sequences and repeats idempotent secure deletion | Recoverable from EVT projection state; boot work is not lease-owned |
| User content-key and KEK shredding | `UserKeyShreddedEvent` tells projections to tombstone encrypted user content | Content keys and wrapping keys are irreversibly shredded before the event is appended | If event append fails after shredding, a retry finds no remaining key and does not currently recreate the missing tombstone fact | Irreversible pre-commit effect with a durable-signal gap |
| Runtime credential cleanup after security changes | Password, account-deletion, and external-identity events advance durable user/auth state before stored sessions and tokens are deleted | The request scans and deletes matching `RUNTIME_STATE` credentials and publishes transient session termination | Credential generation prevents stale credentials from authenticating new requests or reconnects; stale records remain cleanup debt, and an already-open realtime connection depends on best-effort session termination | New authentication is durably revoked; physical cleanup and immediate live disconnect are best-effort |
| Notifications derived from messages | `MessagePostedEvent` contains the source message, actor, room, mentions, and thread relationships | The posting request derives recipient-specific notification records in `RUNTIME_STATE`, publishes live invalidations, and asynchronously invokes web push | Notification creation is not replayed from EVT after a crash; push retries are limited to the active callback and provider behavior | Best-effort derived user state; a crash can lose notification records or push delivery |
| Server branding replacement cleanup | Server logo/banner set or cleared events make the old asset unreachable from projected configuration | The request deletes the prior NATS/S3 object and cached transforms after the config event commits | No durable cleanup worker scans superseded branding assets | Durable pointer update with best-effort orphan cleanup |

Observability is currently domain-specific. Call reconciliation records its
consecutive LiveKit listing failures in `MEMORY_CACHE`. The elected asset
cleanup worker publishes a privacy-safe shared heartbeat with initial-scan,
pending-retry, oldest-pending, pass, and cursor state; owner-only diagnostics
compare it with the live lease and latest matching EVT sequence to report
initializing, healthy, retrying, stalled, or inactive state. Other effects still
primarily emit structured logs, and there is no common metric/status contract
for pending effect count, oldest pending age, retry attempts, terminal failures,
or lease-owned effect-consumer lag.

Failure coverage is also domain-specific. Call cleanup and message-owned asset
deletion have commit/failure, restart, independent-work, and late non-holder
coverage; video processing covers boot resubmission, exact-event confirmation
after ambiguous terminal publication, terminal manifest races, and bounded
prompt cleanup of failed generations;
message-body cleanup covers immediate secure deletion after edits and
retractions. Notification derivation, branding cleanup, the message-body boot
sweep, and the user-key shred/event boundary do not have equivalent
crash-and-recovery coverage. The
call-key, user-DEK, and asset-creation compensation paths likewise lack durable
tests for cleanup failure followed by restart.

Cross-domain follow-up work is tracked in
[#1377](https://github.com/chattocorp/chatto/issues/1377), with separate issues
for physical asset deletion, user-key shredding, video ownership, and the
notification durability decision.

Transient `live.sync.>` publication is intentionally excluded from recovery:
clients treat those messages as invalidations and recover authoritative state
through projected reads. Auth email delivery is also outside this inventory:
registration, verification, and reset credentials live in `RUNTIME_STATE`, with
durable EVT records serving as security audit facts rather than an email queue.
