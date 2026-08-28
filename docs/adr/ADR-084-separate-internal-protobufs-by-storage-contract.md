# ADR-084: Separate Internal Protobufs by Storage Contract

**Status:** Accepted
**Date:** 2026-08-28

## Context

The `chatto.core.v1` protobuf package contained several types of data. It
contained durable EVT facts, the bounded notification event log, transient live
signals, runtime-state records, key material, cache records, and projection
snapshots. A file name was often the only indication of a message's lifecycle.
This made it easy to apply the wrong compatibility or storage rule.

Chatto stores most internal protobuf records as direct protobuf bytes. These
records do not use `google.protobuf.Any`, type URLs, or another stored protobuf
full name. The package and file name do not occur in this wire format. Therefore,
a package move can preserve stored bytes when every field number, wire type,
cardinality, oneof shape, and enum number stays compatible.

Projection snapshot contract IDs are different. Their schema fingerprints use
protobuf full names. A package move selects a new snapshot contract and causes a
cold replay. Projection snapshots are disposable and can be rebuilt from EVT.

## Decision

Chatto separates internal protobufs into packages that name their lifecycle and
storage contract:

| Package | Contract |
| --- | --- |
| `chatto.core.evt.v1` | Durable facts and values stored in `EVT` |
| `chatto.core.notification.v1` | Facts stored in `NOTIFICATIONS` |
| `chatto.core.runtime_state.v1` | Durable latest-value records stored in `RUNTIME_STATE` |
| `chatto.core.key_material.v1` | KMS records stored in `ENCRYPTION_KEYS` |
| `chatto.core.cache_state.v1` | Volatile shared records stored in `MEMORY_CACHE` |
| `chatto.core.projection.v1` | Rebuildable projection snapshot payloads |
| `chatto.core.live.v1` | Transient internal signals published on `live.sync.>` |

Types that are part of an EVT fact stay in `chatto.core.evt.v1`, even when a
projection or runtime operation also uses them. For example, notification
delivery policy is durable configuration in EVT. The notification package owns
the separate notification-log envelope and its signal and lifecycle types.

The package split does not change NATS resource names, subjects, keys, or stored
field contracts. It does not require a data migration. Current binaries decode
existing `EVT`, `NOTIFICATIONS`, `RUNTIME_STATE`, `ENCRYPTION_KEYS`, and
`MEMORY_CACHE` protobuf values with the new generated types.

A compatibility test freezes descriptors from Chatto 0.4.20 and from the last
schema before this split. It compares each old stored field with its relocated
field. It also marshals representative old messages and decodes them with the
current generated types. The test fails on unknown fields or changed wire
bytes. The storage breaking-change check permits only the exact old file set to
move when this test passes. Normal strict Buf checks apply after the new layout
is in the comparison base.

The projection package move deliberately changes projection snapshot contract
IDs. Servers can ignore old snapshots and replay EVT. The live package move can
break generated source and the internal live wire during the 0.5 development
cycle. The public realtime protocol remains a separate mapped contract.

Future contributors must not move a stored symbol only to improve organization.
A later move needs explicit compatibility review and an executable fixture for
every supported stored schema. Stored records must not add a dependency on
protobuf full names without a new decision and migration plan.

## Consequences

- Package names show which storage and compatibility rules apply to a message.
- Generated Go imports make lifecycle boundaries visible in consumers.
- Chatto 0.4 servers can upgrade without an EVT or runtime-state rewrite.
- Old projection snapshots are not restored after the split. A server performs
  a normal cold replay and can write snapshots under the new contract IDs.
- The one-time source and descriptor-name break is accepted for Chatto 0.5.
- New internal protobufs have an explicit owner instead of joining one mixed
  package.

## Related

- [ADR-008](ADR-008-protobuf-for-event-serialization.md)
- [ADR-012](ADR-012-two-tier-realtime-events.md)
- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-076](ADR-076-deterministic-notification-occurrences.md)
