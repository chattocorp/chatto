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
- optional snapshot and local-checkpoint capability hooks; and
- a typed `ProjectionHandle` that keeps a projection with the exact projector
  constructed for it.

Chatto-owned responsibilities remain:

- the concrete event envelope, event vocabulary, subjects, and aggregate
  choices;
- domain projections, services, authorization, and response assembly;
- stable registration keys, display names, admin memory estimates, and
  diagnostic inventory;
- snapshot enablement, repository, encryption, retention, and worker policy;
  and
- runtime composition in `ChattoCore`.

`NewProjectionHandle` is the normal construction path. Code adapting an
already-created projector may use `BindProjectionHandle`, which rejects a
projector built for a different projection. Both constructors require pointer
projection implementations so the projector and read side cannot receive
separate value copies. Projection-aware models consume handles rather than
parallel projection/projector arguments.

`EncodedEventLog` is the envelope-neutral storage boundary. It owns NATS
message-ID deduplication, OCC headers, atomic batches, stream positions, and
opaque record reads. Chatto's `Publisher` remains the typed adapter that
validates `corev1.Event`, uses its stable ID, and protobuf-encodes or decodes at
the boundary. This preserves the existing persisted bytes and lets the write
mechanics evolve without knowing Chatto's event vocabulary.

This decision does not create or promise a public module yet. The current
projector still decodes Chatto's `corev1.Event`. Before extraction, its read
path needs a matching codec boundary, and Chatto-specific stream identity
naming must move behind application-supplied configuration. We will make those
changes only when concrete framework users show the smallest useful API.

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

Extraction still requires deliberate work. The write mechanics no longer
depend on Chatto's protobuf event envelope, but projection replay still does,
and the package embeds Chatto-specific naming and validation assumptions.
