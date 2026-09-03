# Instructions for Chatto Core Protobufs

The nearest package directory identifies each message's lifecycle and storage
contract. Follow [ADR-084](../../../docs/adr/ADR-084-separate-internal-protobufs-by-storage-contract.md).

## Public Realtime Boundary

Do not use messages from this package as public realtime payloads. Realtime
protocol 4 maps selected canonical events to dedicated payloads in
`chatto/realtime/v1/events.proto`. A stored field must not become public only
because it exists in a canonical event.

## Package Ownership

- `evt/v1` owns the canonical Event envelope, durable `EVT` facts, transient
  event variants, and values that are part of those events. Durable variants
  keep their stored compatibility contract. Transient variants use the
  reserved 20000 through 29999 oneof tag range and must not enter EVT.
- `notification/v1` owns the bounded `NOTIFICATIONS` envelope and lifecycle
  facts.
- `runtime_state/v1` owns durable latest-value records in `RUNTIME_STATE`.
- `key_material/v1` owns KMS records in `ENCRYPTION_KEYS`.
- `cache_state/v1` owns volatile shared records in `MEMORY_CACHE`.
- `projection/v1` owns rebuildable projection snapshot payloads.
- `live/v1` owns transient payload messages. The canonical `evt/v1.Event`
  envelope carries these payloads on NATS Core.

Do not put a type in a package because one consumer uses it. Put the type in the
package that owns its authoritative lifecycle.

## Compatibility

Directly stored protobuf bytes do not contain package or file names. However,
field numbers, wire types, cardinality, oneof structure, and enum numbers are
storage contracts. Keep stored changes additive unless an approved migration
or compatibility plan says otherwise.

Do not move a stored symbol only to reorganize code. A move needs explicit
compatibility review and executable tests against all supported stored schemas.
Do not store these messages in `google.protobuf.Any` or store their protobuf full
names without a separate decision and migration plan.

Compatibility fixtures are rolling release-boundary tests. They are not a
permanent archive of every Chatto schema. While Chatto 0.5 is in development,
the fixtures in
[`cli/internal/protocompat/testdata`](../../../cli/internal/protocompat/testdata)
must cover each supported Chatto 0.4 stored schema that can upgrade to 0.5
without a data migration. Do not rewrite or remove a fixture while that upgrade
boundary is supported.

After Chatto 0.5 is released and 0.6 development starts, capture the final 0.5
stored schema as the new compatibility boundary. When direct upgrades from 0.4
are no longer supported, remove the 0.4-specific fixtures and test mappings.
Do not keep old release fixtures only for historical completeness. Before an
agent rotates the fixture set, it must confirm the supported upgrade boundary.

Projection snapshots are rebuildable. A protobuf full-name change selects a new
snapshot contract and causes a cold EVT replay. Live messages are transient, but
their changes still need the approval and review rules in
[`proto/AGENTS.md`](../../AGENTS.md).
