# NATS Resource Inventory

Key files: [`cli/internal/core/core.go`](../../cli/internal/core/core.go), [`cli/internal/events/subjects.go`](../../cli/internal/events/subjects.go), [`proto/chatto/core/v1/event.proto`](../../proto/chatto/core/v1/event.proto), [`cli/internal/core/subjects/subjects.go`](../../cli/internal/core/subjects/subjects.go)

Related decisions: [ADR-001](../adr/ADR-001-nats-jetstream-as-primary-data-store.md),
[ADR-034](../adr/ADR-034-single-event-stream.md), and
[ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md).

Key and subject schemas are maintained separately in the
[runtime state](runtime-state.md) and [subject and event](subjects-and-events.md)
inventories.

## Current resources

| Type         | Name                | Storage | Backup | Description                                                                 |
| ------------ | ------------------- | ------- | ------ | --------------------------------------------------------------------------- |
| Stream       | `EVT`               | File    | Yes    | Event-sourcing log for durable `corev1.Event` facts on `evt.>`              |
| KV bucket    | `RUNTIME_STATE`     | File    | Yes    | Persisted latest-value runtime state, auth/session tokens, notifications, wrapped app DEKs, encrypted snapshot pointers |
| KV bucket    | `MEMORY_CACHE`      | Memory  | No     | Volatile presence, leader leases, reconciliation counters, and worker health heartbeats |
| KV bucket    | `ENCRYPTION_KEYS`   | File    | No     | KMS key-encryption keys and per-call LiveKit E2EE keys; excluded from backups |
| Object store | `SERVER_ASSETS`     | File    | Yes    | Default/legacy NATS-backed persisted asset binaries                         |
| Object store | `PROJECTION_SNAPSHOTS` | File | Yes    | Optional encrypted projection snapshot objects                              |
| Object store | `ASSET_CACHE`       | File    | No     | Optional TTL cache for transformed image bytes                               |
| NATS Core    | `live.sync.>`       | None    | No     | Transient `corev1.LiveEvent` pubsub signals                                  |
| Republish    | `live.evt.>`        | None    | No     | Raw committed `EVT` facts republished by JetStream for server-side live delivery |
