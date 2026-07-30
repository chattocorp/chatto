# ADR-056: Incubate an Extractable NATS Event-Sourcing Framework

**Date:** 2026-07-30

## Context

[ADR-033](ADR-033-event-sourced-state-with-projections.md) deliberately chose a
small internal Go package instead of a third-party event-sourcing framework.
That package now owns proven NATS JetStream mechanics for mandatory OCC,
ordered projection replay, read-your-writes barriers, failure propagation, and
optional restore capabilities.

Chatto's composition layer has also accumulated application policy around those
mechanics: stable diagnostic keys, display names, memory estimates, snapshot
eligibility, domain model ownership, and the concrete `corev1.Event` envelope.
Combining those concerns into a richer Chatto-specific runtime abstraction
would make today's wiring shorter but make a future standalone library harder
to identify and extract.

Projection-aware models additionally received projections and projectors as
separate constructor arguments. Nothing in those signatures proved that the
projector owned the supplied projection, so a wiring mistake could combine a
read model with another projection's replay frontier.

## Decision

Treat `cli/internal/events` as the incubator for a small Event Sourcing on NATS
framework that may later become a standalone Go module.

Framework-owned responsibilities are:

- opaque-byte event-log reads, OCC-only publishing, and atomic append
  mechanics;
- stream positions and projection readiness barriers;
- ordered consumer, replay, startup batching, and failure lifecycles;
- optional snapshot and local-checkpoint capability hooks that bind persisted
  state to an opaque application-supplied stream identity; and
- a typed `ProjectionHandle` that keeps a projection with the exact projector
  constructed for it.

Chatto-owned responsibilities remain:

- the concrete event envelope, event vocabulary, subjects, and aggregate
  choices;
- domain projections, services, authorization, and response assembly;
- stable registration keys, display names, admin memory estimates, and
  diagnostic inventory;
- stream identity discovery, metadata naming, format, and validation;
- snapshot enablement, repository, encryption, retention, and worker policy;
  and
- runtime composition in `ChattoCore`.

`evtstream.NewProjectionHandle` is Chatto's normal construction path over
`events.NewDecodedProjectionHandle`. Code adapting an
already-created projector may use `evtstream.BindProjectionHandle`, which rejects a
projector built for a different projection. Both constructors require pointer
projection implementations so the projector and read side cannot receive
separate value copies. Projection-aware models consume handles rather than
parallel projection/projector arguments.

`EncodedEventLog` is the envelope-neutral storage boundary. It owns NATS
message-ID deduplication, OCC headers, atomic batches, stream positions, and
opaque record reads. Chatto's `evtstream.Publisher` is the typed adapter that
validates `corev1.Event`, uses its stable ID, and protobuf-encodes or decodes at
the boundary. This preserves the existing persisted bytes and lets the write
mechanics evolve without knowing Chatto's event vocabulary.

`NewDecodedProjector` is the matching envelope-neutral replay boundary.
Applications supply an `EventDecoder[E]` and `EventProjection[E]`; the
framework retains ordered consumption, subject filtering, startup batching,
readiness, snapshots, checkpoints, and failure handling.
`internal/evtstream` owns Chatto's `NewProjector`, `Projection`,
`SequencedEvent`, and publisher APIs as specializations over `corev1.Event` and
its unchanged protobuf codec. `internal/events` has no production dependency on
Chatto protobufs or subject policy.

Chatto owns its versioned EVT incarnation format and the
`chatto.evt.incarnation` stream metadata through `internal/evtstream`.
Composition passes Chatto's resolver into snapshot and checkpoint
configuration. At restore time the projector invokes it with the same fresh
`StreamInfo` used for sequence bounds, preventing an old identity from being
combined with a recreated stream's bounds. The projector and snapshot
repository require only a non-empty opaque result and never impose Chatto's
metadata key or identity syntax. Snapshot capture carries the identity bound to
that projector run alongside its state and cutoff; application publication
does not maintain a second identity value.

This decision does not create or promise a public module yet. The physical
package direction is application/core code to `internal/evtstream`, then to
`internal/events`. Extraction will happen only when concrete framework users
show the smallest useful public API.

## Consequences

Projection ownership and replay readiness can no longer be mismatched silently
in normal wiring. Model constructors are shorter, and the reusable lifecycle
unit is visible both in the core runtime and the independently runnable bundled
search provider.

New event-sourcing mechanics should be evaluated for `internal/events`; new
Chatto policy should stay in `internal/core` or the owning runtime unit. This
creates a reviewable extraction boundary without forcing premature package
stability or generic abstractions.

The handle adds one small generic API and an identity check for adapting
existing projectors. It intentionally does not absorb registration metadata or
snapshot policy, so some application composition remains explicit.

Extraction still requires deliberate work. The reusable package no longer
depends on any other Chatto production package: its production imports are
limited to the Go standard library and `nats.go`. It privately classifies the
JetStream wrong-last-sequence errors needed to preserve `ErrConflict` rather
than depending on Chatto's application-wide JetStream helpers. Generic
projection replay can use another application envelope without changing the
ordered lifecycle, while `internal/evtstream` keeps Chatto's storage contract
explicit and unchanged.
