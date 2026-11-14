# NATS Resource Inventory

Key files: [`cli/internal/natsresources/natsresources.go`](../../cli/internal/natsresources/natsresources.go), [`cli/internal/core/storage.go`](../../cli/internal/core/storage.go), [`cli/internal/core/core_infrastructure.go`](../../cli/internal/core/core_infrastructure.go), [`cli/internal/evtstream/identity.go`](../../cli/internal/evtstream/identity.go), [`cli/internal/evtstream/subjects.go`](../../cli/internal/evtstream/subjects.go), [`cli/internal/core/subjects/subjects.go`](../../cli/internal/core/subjects/subjects.go), [`cli/internal/video/unit.go`](../../cli/internal/video/unit.go), [`cli/cmd/backup.go`](../../cli/cmd/backup.go)

Related decisions: [ADR-001](../adr/ADR-001-nats-jetstream-as-primary-data-store.md),
[ADR-034](../adr/ADR-034-single-event-stream.md),
[ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md),
[ADR-066](../adr/ADR-066-durable-asset-processing-runtime-unit.md), and
[ADR-069](../adr/ADR-069-explicit-durable-consumer-lifecycle.md), and
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md), and
[ADR-081](../adr/ADR-081-explicit-expiry-for-mutable-runtime-credentials.md), and
[ADR-084](../adr/ADR-084-separate-internal-protobufs-by-storage-contract.md), and
[ADR-114](../adr/ADR-114-jetstream-storage-conventions.md).

Key and subject schemas are maintained separately in the
[runtime state](runtime-state.md) and [subject and event](subjects-and-events.md)
inventories.

## Startup provisioning

The `natsresources` registry lists each resource name, kind, and backup
policy. It also lists legacy buckets that earlier versions created and that
upgraded servers can still have. Backup, restore, `chatto keys import`, and
the test reset use the registry. `storage.go` has one configuration function
for each resource, and a test pins each configuration. Another test makes sure
that core creates exactly the registered resources.

Chatto provisions its core streams, KV buckets, and Object Stores through
`jetstreamutil.CreateJetStreamResourceWithRetry`, including optional projection
snapshot storage. Chatto owns resource configuration and identity metadata.
The shared framework owns the bounded retry and cancellation mechanics.

Concurrent startup preserves published `EVT` and `NOTIFICATIONS` identities
and metadata. An earlier result that the stream does not exist must not clear
metadata that another replica has stored.

Chatto uses bounded exponential backoff with jitter during provisioning.
Each request has its own timeout, so a request can expire without ending the
startup context. Timeouts, missing responders, cluster unavailability, and the
existing transient store-creation and stream-name conflict errors are retried.
Parent cancellation stops provisioning. Permanent errors and retry exhaustion
fail startup. Resource configuration and stored identities do not change.
Readiness requires completed initialization and replay. The Kubernetes example
allows ten minutes for storage recovery and replay before it restarts the pod.

## Current resources

| Type         | Name                   | Storage | Backup | Description                                                                                                                                                                                                                                                                           |
| ------------ | ---------------------- | ------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Stream       | `EVT`                  | File    | Yes    | Event-sourcing log for durable `evtv1.Event` facts on `evt.>`                                                                                                                                                                                                                         |
| Stream       | `LOG`                  | File    | No     | Retained operational protobuf records from `chatto.core.log.v1`; seven-day default age, configurable with `core.log.retention`; no byte/count limit                                                                                                                                   |
| Stream       | `NOTIFICATIONS`        | File    | Yes    | Replicated bounded `notificationv1.NotificationEvent` log for 90-day notification signals, reads, removals, and push outcomes; per-message TTL adds a 24-hour physical-cleanup grace                                                                                                  |
| Stream       | `JOBS`                 | File    | Yes    | Latest protobuf dispatch per `jobs.{type}.{id}` subject; limits retention with a seven-day maximum age; repair publishes again for unfinished jobs. Current execution state is in `RUNTIME_STATE`                                                                                     |
| KV bucket    | `RUNTIME_STATE`        | File    | Yes    | Persisted latest-value records: `chatto.core.runtime_state.v1` protobuf, plus JSON credential and workflow records and raw binary read markers (ADR-114). Includes private presence choices, credentials, telemetry, notification boundaries, wrapped app DEKs, and snapshot pointers |
| KV bucket    | `MEMORY_CACHE`         | Memory  | No     | Volatile shared records from `chatto.core.cache_state.v1`, plus non-protobuf worker leases, cooldowns, counters, and health heartbeats                                                                                                                                                |
| KV bucket    | `ENCRYPTION_KEYS`      | File    | No     | KMS records from `chatto.core.key_material.v1`; excluded from backups                                                                                                                                                                                                                 |
| Object store | `SERVER_ASSETS`        | File    | Yes    | Default/legacy NATS-backed persisted asset binaries                                                                                                                                                                                                                                   |
| Object store | `PROJECTION_SNAPSHOTS` | File    | Yes    | Optional encrypted `chatto.core.projection.v1` snapshot objects; configurable TTL defaults to seven days                                                                                                                                                                              |
| Object store | `ASSET_CACHE`          | File    | No     | Always-on seven-day cache for resized images and Neighborhood images                                                                                                                                                                                                                  |
| NATS Core    | `live.sync.>`          | None    | No     | Non-durable `pubsubv1.PubSubEvent` values                                                                                                                                                                                                                                             |
| Republish    | `live.evt.>`           | None    | No     | Raw committed `EVT` facts republished by JetStream for server-side live delivery                                                                                                                                                                                                      |

## Projection consumers

Each projector owns one ephemeral ordered consumer. Chatto uses the name
`projection-<key>-<random>_<generation>` and stores the owner in consumer
metadata. On exit, the projector stops consumption and attempts to delete only
its current consumer. Five-minute inactivity expiry remains the fallback if
cleanup fails or the process crashes. See the [projection inventory](projections.md)
for the owners and lifecycle.

## Durable consumers

| Stream          | Consumer                                   | Filter                                                                                                                                                      | Ack contract                                                                                                                                                                                                                                                                                                                        | Owner                                                       |
| --------------- | ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| `EVT`           | `chatto-asset-processing-v1`               | Canonical and legacy asset-processing-started subjects                                                                                                      | Explicit ack after durable media job acceptance                                                                                                                                                                                                                                                                                     | Shared core replicas                                        |
| `EVT`           | `chatto-user-key-shredding-v1`             | `evt.user.*.user_key_shredding_requested`                                                                                                                   | Explicit ack after durable `user_key_shredding` job acceptance                                                                                                                                                                                                                                                                      | Shared `ChattoCore` replicas                                |
| `EVT`           | `chatto-user-push-subscription-cleanup-v1` | `evt.user.*.account_deleted`                                                                                                                                | Explicit ack after durable `push_subscription_cleanup` job acceptance. The permanent exact deletion fact also fences registration, and a leased global reconciliation pass repairs late writes and orphan owners without rescanning all owners for every historical delivery                                                        | Shared `ChattoCore` replicas through `events.DurableWorker` |
| `EVT`           | `chatto-call-key-cleanup-v1`               | `evt.room.*.call_ended`                                                                                                                                     | Explicit ack after durable `call_key_cleanup` job acceptance                                                                                                                                                                                                                                                                        | Shared `ChattoCore` replicas                                |
| `EVT`           | `chatto-asset-cleanup-v1`                  | `evt.asset.*.asset_deleted`                                                                                                                                 | Explicit ack after durable `asset_cleanup` job acceptance                                                                                                                                                                                                                                                                           | Shared `ChattoCore` replicas                                |
| `EVT`           | `chatto-notification-materializer-v1`      | Existing message, reaction, membership, room-layout, RBAC, account, and configured-owner facts; the name/filter pair is one immutable capability generation | The materializer waits until current local projections include the source, stores idempotent bounded notification output, then confirms the source acknowledgement. Interrupted, partially completed, failed, or schema-unsupported work is redelivered rather than discarded. New source schemas require a new consumer generation | Shared `ChattoCore` replicas through `events.DurableWorker` |
| `EVT`           | `chatto-invitation-notifications-v1`       | Room invitation creation, acceptance, and decline                                                                                                           | Starts with new events; creates idempotent notification output and removes handled invitations before acknowledgement. Separate capability generation preserves the existing message consumer and its pending work.                                                                                                                 | Shared `ChattoCore` replicas through `events.DurableWorker` |
| `NOTIFICATIONS` | `chatto-notification-alert-delivery-v1`    | `notifications.signalled`                                                                                                                                   | Explicit ack after durable `notification_alert` job acceptance                                                                                                                                                                                                                                                                      | Shared `ChattoCore` replicas through `events.DurableWorker` |

All consumers use file-backed durable consumer state. Effect sources only hand
work to `JOBS`; the job handlers perform the effect. Replay is safe because
asset-processing handlers accept projected terminal outcomes, key-shredding
and cleanup handlers repeat idempotent deletion, push cleanup is fenced against
late registration, and user-key handlers also accept an existing
physical-completion fact. Push delivery reads the bounded notification
lifecycle log; projected terminal state is its idempotency fence.

The consumer names are versioned persisted resource contracts. Required effect
consumers have no inactivity cleanup and survive worker shutdown or
scale-to-zero. Normal backups include both durable streams and their consumer
state.

Backups snapshot `EVT` before `RUNTIME_STATE`, then `NOTIFICATIONS`.
The materializer's durable EVT consumer position and idempotent output
coordinates make an in-flight cross-stream handoff recoverable at either backup
boundary.
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

## Job consumers and webhook source

`chatto-bot-webhook-source-v1` consumes `evt.room.*.message_posted` from EVT.
It acknowledges only after every selected endpoint has a durable job record
and confirmed dispatch. The media source and the effect sources in the
durable-consumer table use the same handoff rule. Source redelivery cannot
reset an existing job.

An effect job stores only the source subject, stream sequence, and message
digest. Its job ID derives from the job type, stream name, sequence, and
digest, so a recreated stream that reuses a sequence gets a new job. An attempt
whose source message no longer matches the digest fails permanently. Each attempt reads the
source message again. A missing notification signal completes its job, because
expiry reconciliation owns that outcome. A missing EVT source fails the job.

`chatto-jobs-v1` consumes `jobs.{type}.*` from `JOBS` for `webhook`,
`asset_cleanup`, `call_key_cleanup`, `push_subscription_cleanup`,
`user_key_shredding`, and `notification_alert`. Server replicas share this
durable consumer. `chatto-jobs-media-v1` consumes only `jobs.media.*`; server
replicas with media processing and standalone media processes share it. Both
use explicit acknowledgement, two-minute acknowledgement wait, unlimited
delivery, and at most 64 unacknowledged dispatches per consumer. Workers
acknowledge a dispatch when an attempt ends, so a job that waits for a retry
holds no unacknowledged dispatch; the record watcher publishes it again when it
is due. Current job revision
claims coordinate the overlapping consumers. Acknowledgement does not delete
retained dispatches. Both consumers appear in worker diagnostics.

Backups capture EVT before runtime state, then notifications and jobs. The
unfinished-job watcher repairs dispatches after restore, including a job whose
state was captured before its dispatch or whose consumer position advanced
between snapshots. See ADR-123.
