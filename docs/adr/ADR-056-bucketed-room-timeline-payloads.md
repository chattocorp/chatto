# ADR-056: Bucket Room Timeline Payloads by UTC Week

**Date:** 2026-07-24

## Context

The Room Timeline projection currently retains every projected event and every
current message body as decoded protobufs for the lifetime of the process. Its
indexes are efficient, but its resident payload memory grows with all room
history even when operators and members only read recent messages.

Room and thread timeline cursors use EVT stream sequences. Search returns
message identifiers that Chatto must authorize and hydrate against current
projection state. Any memory reduction therefore needs to preserve exact
pagination, thread, deletion, attachment, echo, and search semantics without
introducing a shared replay consumer or a second durable source of truth.

A benchmark against a 53,763-event production-derived EVT backup found 41,612
Room Timeline events. UTC-week buckets produced 375 room buckets; the largest
cold bucket held 1,963 events and 338 KB of encoded payload. UTC-month buckets
produced 269 room buckets, but the largest held 4,562 events and 783 KB. Exact
sequence loading of the largest weekly bucket took about 18 ms with bounded
concurrency, versus about 42 ms for the monthly bucket. The additional weekly
metadata was negligible compared with retained protobuf payloads.

## Decision

Keep one Room Timeline projection and its existing independent ordered EVT
consumer. Partition only its decoded payload cache into fixed UTC-week buckets.
Bucket geometry is deliberately not configurable because changing it would
invalidate snapshot layout and make operational behavior harder to compare.

The projection always retains lightweight metadata: timeline ordering,
event-to-bucket locators, message-body sequence history, visibility and
retraction state, and compact delta-varint encoded EVT sequences required to
reconstruct each bucket. The encoding folds the optional-obsolete-body marker
into each sequence delta. Recent buckets retain their decoded event and
current-body protobufs. The operator configures the recent hot window in days;
it defaults to 30 days.

When a read needs a cold bucket, the projection loads its referenced messages
directly from EVT with bounded concurrency, validates and decodes them, and
installs the reconstructed payload atomically. Concurrent readers share the
same load. Missing sequences are errors except for message-body facts that
Chatto may have securely deleted after they became obsolete. Search candidate
hydration uses the same loading boundary as timeline and thread reads.

The first version does not evict a bucket after it has been loaded. Its bucket
state records access and resident size so a later LRU policy can be added
without changing the storage model or read APIs. This deliberately accepts
process-lifetime cache growth from historical reads while removing the
unconditional cold-boot cost.

Room Timeline snapshots use a new codec contract. They retain the complete
lightweight directory and the same compact EVT references, plus decoded
payloads only for buckets inside the configured hot window. Incompatible or
missing snapshots cold-replay EVT as before. A changed hot-window setting
changes cache residency, not projected behavior or bucket identity.

Use event `created_at` to assign a fixed UTC week. A message's first projected
body or post establishes its bucket, which handles the durable ordering where
an initial `MessageBodyEvent` precedes its `MessagePostedEvent`. Later edits and
retractions follow the established message bucket. Events without a usable
timestamp use a deterministic timeless bucket rather than wall-clock time.

ThreadProjection remains independently consumed and retains its lightweight
reply references. Resolving a thread root or reply materializes the owning Room
Timeline bucket. Bucketing ThreadProjection's own metadata is deferred until
measurements show that it is worthwhile.

## Consequences

Normal startup and recent-room reads retain only recent decoded Room Timeline
payloads while preserving existing global cursors, authorization, and event
ordering. Historical reads pay an explicit EVT load and protobuf decode cost
once per process and bucket.

EVT remains required for cold bucket materialization. Operators must not prune
non-obsolete Room Timeline facts independently of the projection. Secure
deletion of obsolete body facts remains supported because the directory marks
those references as optional during reconstruction.

Exact sequence references are stored as monotonically increasing delta varints,
typically using one to three bytes per referenced fact. This is intentionally
simpler than ranges, which perform poorly when a room's events are interleaved
with other rooms and would make missing-fact handling more complex. Hydration
temporarily expands only the requested bucket into ordinary sequence records.

The parent projection still retains metadata proportional to history. A future
LRU bounds decoded cache residency; a separate archive projection remains an
alternative if metadata itself becomes material.
