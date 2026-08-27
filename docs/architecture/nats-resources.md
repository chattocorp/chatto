# NATS Resource Inventory

Key files: [`cli/internal/core/storage.go`](../../cli/internal/core/storage.go), [`cli/internal/core/core_infrastructure.go`](../../cli/internal/core/core_infrastructure.go), [`cli/internal/evtstream/identity.go`](../../cli/internal/evtstream/identity.go), [`cli/internal/evtstream/subjects.go`](../../cli/internal/evtstream/subjects.go), [`cli/internal/core/subjects/subjects.go`](../../cli/internal/core/subjects/subjects.go), [`cli/internal/video/unit.go`](../../cli/internal/video/unit.go), [`cli/cmd/backup.go`](../../cli/cmd/backup.go)

Related decisions: [ADR-001](../adr/ADR-001-nats-jetstream-as-primary-data-store.md),
[ADR-034](../adr/ADR-034-single-event-stream.md),
[ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md),
[ADR-066](../adr/ADR-066-durable-asset-processing-runtime-unit.md), and
[ADR-069](../adr/ADR-069-explicit-durable-consumer-lifecycle.md), and
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md), and
[ADR-081](../adr/ADR-081-explicit-expiry-for-mutable-runtime-credentials.md).

Key and subject schemas are maintained separately in the
[runtime state](runtime-state.md) and [subject and event](subjects-and-events.md)
inventories.

## Current resources

| Type         | Name                | Storage | Backup | Description                                                                 |
| ------------ | ------------------- | ------- | ------ | --------------------------------------------------------------------------- |
| Stream       | `EVT`               | File    | Yes    | Event-sourcing log for durable `corev1.Event` facts on `evt.>`              |
| Stream       | `NOTIFICATIONS`     | File    | Yes    | Replicated bounded event log for 90-day notification signals, reads, removals, and push outcomes; per-message TTL adds a 24-hour physical-cleanup grace |
| KV bucket    | `RUNTIME_STATE`     | File    | Yes    | Persisted latest-value runtime state, fixed-expiry bearer access verifiers, mutable cookie and renewable-session authorities with explicit expiry and per-message TTL, workflow credentials, notification read/visibility boundaries, wrapped app DEKs, and encrypted snapshot pointers |
| KV bucket    | `MEMORY_CACHE`      | Memory  | No     | Volatile presence, worker leases and cooldowns, reconciliation counters, and worker health heartbeats; recreated automatically after a full NATS restart |
| KV bucket    | `ENCRYPTION_KEYS`   | File    | No     | KMS key-encryption keys and per-call LiveKit E2EE keys; excluded from backups |
| Object store | `SERVER_ASSETS`     | File    | Yes    | Default/legacy NATS-backed persisted asset binaries                         |
| Object store | `PROJECTION_SNAPSHOTS` | File | Yes    | Optional encrypted projection snapshot objects; configurable TTL defaults to seven days |
| Object store | `ASSET_CACHE`       | File    | No     | Optional TTL cache for transformed image bytes                               |
| NATS Core    | `live.sync.>`       | None    | No     | Transient `corev1.LiveEvent` pubsub signals                                  |
| Republish    | `live.evt.>`        | None    | No     | Raw committed `EVT` facts republished by JetStream for server-side live delivery |

## Durable consumers

| Stream | Consumer | Filter | Ack contract | Owner |
| ------ | -------- | ------ | ------------ | ----- |
| `EVT` | `chatto-asset-processing-v1` | `evt.asset.*.asset_processing_started`, legacy `evt.room.*.asset_processing_started` | Explicit ack after a terminal asset outcome is projected; interrupted work is redelivered | Shared `asset-processing` runtime-unit replicas |
| `EVT` | `chatto-user-key-shredding-v1` | `evt.user.*.user_key_shredding_requested` | Explicit ack after idempotent key deletion and projected `UserKeyShreddedEvent`; interrupted or failed work is redelivered | Shared `ChattoCore` replicas |
| `EVT` | `chatto-user-push-subscription-cleanup-v1` | `evt.user.*.account_deleted` | Explicit ack after idempotent owner-first removal of the account's known push credentials; interrupted or partially failed cleanup is redelivered. The permanent exact deletion fact also fences registration, and a leased global reconciliation pass repairs late writes and orphan owners without rescanning all owners for every historical delivery | Shared `ChattoCore` replicas through `events.DurableWorker` |
| `EVT` | `chatto-call-key-cleanup-v1` | `evt.room.*.call_ended` | Explicit ack after idempotent call-key shredding; interrupted or failed work is redelivered | Shared `ChattoCore` replicas |
| `EVT` | `chatto-asset-cleanup-v1` | `evt.asset.*.asset_deleted` | Explicit ack after idempotent binary and transform-cache deletion; interrupted or failed work is redelivered | Shared `ChattoCore` replicas |
| `EVT` | `chatto-notification-materializer-v1` | Existing message, reaction, membership, room-layout, RBAC, account, and configured-owner facts; the name/filter pair is one immutable capability generation | Confirmed double ack only after exact-sequence derivation and idempotent lifecycle facts reach `NOTIFICATIONS`, with privacy boundaries persisted where required; interrupted, partially completed, failed, or schema-unsupported work is redelivered rather than discarded. New source schemas require a new consumer generation | Shared `ChattoCore` replicas through `events.DurableWorker` |
| `NOTIFICATIONS` | `chatto-notification-alert-delivery-v1` | `notifications.signalled` | Explicit ack after the projected occurrence has a terminal delivered/suppressed state; transient provider failures are redelivered within the immutable two-minute delivery horizon | Shared `ChattoCore` replicas through `events.DurableWorker` |

All consumers use file-backed durable consumer state. Most consume domain facts
from `EVT`: replaying those facts is safe because asset-processing workers
acknowledge projected terminal outcomes, key-shredding and cleanup workers
repeat idempotent deletion, push cleanup is fenced against late registration,
and user-key workers additionally acknowledge an existing physical-completion
fact. Push delivery instead consumes the bounded
notification lifecycle log directly; projected terminal state is its
idempotency fence.

The consumer names are versioned persisted resource contracts. Required effect
consumers have no inactivity cleanup and survive worker shutdown or
scale-to-zero; normal backups include both durable streams and their consumer
state. Backups snapshot `EVT` before `RUNTIME_STATE`, then `NOTIFICATIONS`.
The materializer's durable EVT consumer position and deterministic output make
an in-flight cross-stream handoff recoverable at either backup boundary.
Restored push deliveries still obey
their immutable source-time deadline, so restoring an old backup cannot renew
interruptive work. Chatto currently has no retired durable
effect consumers. A future removal or incompatible contract change follows
ADR-069's explicit drain, rollout, and deletion lifecycle.
If a required consumer disappears while its worker is running, the stale worker
supervisor then recreates the declared consumer through its application-owned
configuration; the shared worker framework never creates it.

## EVT stream identity

`EVT` metadata key `chatto.evt.incarnation` stores a versioned identity with
the existing `evt-incarnation-v1:` format. Chatto creates the value from the
stream creation timestamp when metadata is missing, preserves it through
normal updates and backups, and changes it when the stream is recreated.
`internal/evtstream` is the application-owned authority for this metadata and
format. Projector restore resolves it from the same fresh stream-info snapshot
as the relevant sequence bounds; projection persistence treats the validated
result as opaque.

## NOTIFICATIONS stream identity

`NOTIFICATIONS` metadata key `chatto.notifications.incarnation` stores a
versioned identity with the `notifications-incarnation-v1:` format. The
`internal/notificationstream` adapter creates and validates it independently
from `EVT`. Notification projection snapshots bind to this identity and the
notification stream sequence, allowing the shared snapshot framework to
support more than one application-owned event log without mixing coordinates.
