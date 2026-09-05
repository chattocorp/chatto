---
name: chatto-architecture-inventory
description: Maintain Chatto runtime documentation in docs/architecture/ when runtime structures change, or audit it when requested.
---

# Chatto Architecture Inventory

Read [the inventory index](../../../../docs/architecture/INDEX.md). It defines
the categories and documentation boundaries. Read only the categories relevant
to the task. Run a complete audit only when requested.

Update affected inventory files by default. For a review, audit, or request
for proposals, report findings without edits unless the user also requests
fixes. Report code defects separately; inventory work does not authorize
implementation changes.

## Source Checks

Paths below are relative to the repository root.

| Category | Check against |
| --- | --- |
| Runtime components | Core construction, runtime units, service and worker constructors; record ownership and lifecycle. |
| Projections | Projector registration, `Subjects()`, `projection_subjects_test.go`, and snapshot code; distinguish registered projectors from nested read models. |
| NATS resources | Stream, KV, and Object Store creation calls; record retention, storage, backup status, and owner. |
| Subjects and events | Subject helpers, persisted protobuf variants, and live publishers; check event tokens and aggregate subjects. |
| Runtime state | KV operations and object-key construction; record encoding, TTL, backup status, and security properties. |
| Durable effects | Workers and recovery tests; check durable triggers, retries, restart recovery, duplicate delivery, and multiple replicas. |
| Interfaces | Declared protobuf services, `API.Handlers()`, `API.OperatorHandlers()`, and HTTP mounts; check authentication and listener boundaries. |
| Realtime delivery | Protocol, handler, event models, and client event bus; check projection waits, authorization, queues, and failure behavior. |

## Editing And Verification

- Link to source code and relevant decisions. Do not copy tables from other
  categories or reproduce the generated per-RPC reference.
- Replace stale facts in place. Keep compatibility details only while they
  affect current behavior or stored data.
- Distinguish unavailable operational data from a healthy zero value.
- Keep retired resources out of the current resource inventory.
- Update the index when categories change. Keep `docs/ARCHITECTURE.md` as a
  short link to the inventory.
- Verify changed claims against code and check source and document links.
- Report the categories checked and any unresolved findings. Do not describe
  a partial check as a complete audit.
