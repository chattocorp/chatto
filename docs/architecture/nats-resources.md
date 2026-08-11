# NATS Resource Inventory

Key files: [`cli/internal/core/storage.go`](../../cli/internal/core/storage.go), [`cli/internal/core/core_infrastructure.go`](../../cli/internal/core/core_infrastructure.go), [`cli/internal/evtstream/identity.go`](../../cli/internal/evtstream/identity.go), [`cli/internal/evtstream/subjects.go`](../../cli/internal/evtstream/subjects.go), [`cli/internal/core/subjects/subjects.go`](../../cli/internal/core/subjects/subjects.go), [`cli/internal/video/unit.go`](../../cli/internal/video/unit.go), [`cli/cmd/backup.go`](../../cli/cmd/backup.go)

Related decisions: [ADR-001](../adr/ADR-001-nats-jetstream-as-primary-data-store.md),
[ADR-034](../adr/ADR-034-single-event-stream.md),
[ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md),
[ADR-066](../adr/ADR-066-durable-asset-processing-runtime-unit.md), and
[ADR-069](../adr/ADR-069-explicit-durable-consumer-lifecycle.md).

Key and subject schemas are maintained separately in the
[runtime state](runtime-state.md) and [subject and event](subjects-and-events.md)
inventories.

## Current resources

| Type         | Name                | Storage | Backup | Description                                                                 |
| ------------ | ------------------- | ------- | ------ | --------------------------------------------------------------------------- |
| Stream       | `EVT`               | File    | Yes    | Event-sourcing log for durable `corev1.Event` facts on `evt.>`              |
| KV bucket    | `RUNTIME_STATE`     | File    | Yes    | Persisted latest-value runtime state, auth/session tokens, notifications, wrapped app DEKs, encrypted snapshot pointers |
| KV bucket    | `MEMORY_CACHE`      | Memory  | No     | Volatile presence, worker leases and cooldowns, and reconciliation counters; recreated automatically after a full NATS restart |
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
| `EVT` | `chatto-call-key-cleanup-v1` | `evt.room.*.call_ended` | Explicit ack after idempotent call-key shredding; interrupted or failed work is redelivered | Shared `ChattoCore` replicas |
| `EVT` | `chatto-asset-cleanup-v1` | `evt.asset.*.asset_deleted` | Explicit ack after idempotent binary and transform-cache deletion; interrupted or failed work is redelivered | Shared `ChattoCore` replicas |

All consumers use file-backed durable consumer state inherited from `EVT` and
do not introduce separate work streams. Replaying older facts is safe:
asset-processing workers acknowledge projected terminal outcomes, while
key-shredding and cleanup workers repeat idempotent deletion; user-key workers
additionally acknowledge an existing physical-completion fact.

The consumer names are versioned persisted resource contracts. Required effect
consumers have no inactivity cleanup and survive worker shutdown or
scale-to-zero; `EVT` backups include their consumer state. Chatto currently has
no retired durable effect consumers. A future removal or incompatible contract
change follows ADR-069's explicit drain, rollout, and deletion lifecycle.

## EVT stream identity

`EVT` metadata key `chatto.evt.incarnation` stores a versioned identity with
the existing `evt-incarnation-v1:` format. Chatto creates the value from the
stream creation timestamp when metadata is missing, preserves it through
normal updates and backups, and changes it when the stream is recreated.
`internal/evtstream` is the application-owned authority for this metadata and
format. Projector restore resolves it from the same fresh stream-info snapshot
as the relevant sequence bounds; projection persistence treats the validated
result as opaque.
