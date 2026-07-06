# ADR-008: Protobuf for Event Serialization

**Date:** 2026-03-01

## Context

Events published to JetStream streams need a serialization format. These events are written frequently (every message, every join/leave, every reaction), stored persistently, and deserialized on every read (message history, subscription delivery, backup/restore). The format affects storage size, read/write performance, and schema evolution.

Options considered:

- **JSON**: Human-readable, universally supported, but verbose and slow to parse. No built-in schema evolution story beyond "be careful."
- **MessagePack/CBOR**: Compact binary JSON. Smaller than JSON but still schemaless — no codegen, no typed contracts.
- **Protocol Buffers**: Typed schema with codegen for Go and other languages. Compact binary format. Field numbering provides forward/backward compatibility.

## Decision

Use Protocol Buffers (proto3) for all JetStream event serialization. Proto definitions live in `proto/` and generate Go types used by core services, projections, and public API mapping.

- Events are defined as proto messages with `oneof` for polymorphic event types (e.g., `SpaceRoomEvent` can be a `MessagePostedEvent`, `UserJoinedRoomEvent`, etc.)
- KV values that are structured data also use protobuf

## Consequences

- **Compact storage**: Protobuf is significantly smaller than JSON for structured data. With thousands of messages per room, this reduces JetStream storage costs.
- **Fast serialization**: Proto marshal/unmarshal is faster than JSON encode/decode, which matters for high-throughput message delivery.
- **Schema evolution via field numbers**: New fields can be added without breaking existing data. Old binaries skip unknown fields. This is critical since stored events must remain readable across upgrades.
- **Proto changes are breaking changes**: Modifying proto definitions affects wire format compatibility. Changes to `proto/` are treated as significant breaking changes according to the project status in `AGENTS.md`.
- **Not human-readable**: Debugging stream contents requires `protoc --decode_raw` or similar tooling. The `chatto-debugging` skill documents the workflow for inspecting raw stream data.
- **Codegen step required**: Any schema change requires running `mise codegen` to regenerate Go types. Forgetting this causes build failures.
- **API mapping**: Persisted event protos are storage contracts, not public API contracts. ConnectRPC/realtime API handlers map persisted event shapes into caller-facing API protos instead of exposing storage messages directly.
