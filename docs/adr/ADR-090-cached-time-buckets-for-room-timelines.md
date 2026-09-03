# ADR-090: Reconstruct Room Timelines in Cached Time Buckets

**Date:** 2026-09-02

**Status:** Accepted

## Context

`ServerContentView` contains one room-timeline projection component. The
component keeps all room timeline entries and all current encrypted message
bodies in RAM. Its projection snapshot also stores copies of that data.

This design makes RAM use and projection snapshot size grow with the complete
room timeline history. The encrypted body payloads and retained Protobuf event
graphs are much larger than the metadata that Chatto needs to find those
records in `EVT`.

ADR-033 makes `EVT` the source of domain truth. ADR-050 makes projection
snapshots optional acceleration data. ADR-088 lets one projection component
store focused snapshot data inside the `ServerContentView` snapshot cohort.
These decisions let the timeline component persist a small reconstruction
index without persisting a second copy of each message.

Room history must continue to support timeline pages, message-ID reads, thread
windows, around-message reads, pins, attachments, echoes, edits, retractions,
and crypto-shredding. A normal request must not scan an unbounded part of
`EVT`.

## Decision

Keep an always-resident room-timeline bucket directory and reconstruct timeline
entry payloads and message data in time-based buckets from `EVT`.

### Time-based bucket identity

One timeline bucket belongs to one room and one UTC time interval. The
operator can configure the interval. One week is the default. This default
keeps reconstruction work and cache entries smaller than a monthly default.
An operator can select a different interval for the server's workload.

The first version uses strict time boundaries. It does not split one interval
into size-based shards. The time of the original `MessagePostedEvent` selects
the bucket for a message. The event's stable EVT stream sequence, not its
timestamp, controls timeline order and pagination.

A visible non-message room-timeline entry uses its own creation time. A
historical event without a usable creation time goes into a deterministic
compatibility bucket for that room. This preserves replay compatibility
without making the stream sequence into a second bucket policy.

The configured interval and its boundary rules are part of the timeline
snapshot restore contract. If the configuration does not match a stored
directory, Chatto rejects the complete `ServerContentView` cohort and cold-
replays `EVT`.

### Always-resident bucket directory

The room-timeline component keeps the bucket directory in RAM. Every event on
`evt.room.{roomId}.*` adds its exact EVT stream sequence to the room's bucket
for the event occurrence time. For each bucket, the directory stores these
sequences in a slice. The first version does not compress the slice, combine
sequences into ranges, or use a second persistent index.

The directory also stores the compact metadata needed for bounded routing and
lookups. This includes:

- the room and interval for each bucket;
- the mapping from a message event ID to its original bucket;
- timeline ordering and visibility metadata that a query needs to select the
  correct buckets;
- current pin, echo, tombstone, attachment-locator, attachment-asset-ID,
  attachment-count, and secure-delete metadata that must be available before a
  bucket is loaded; and
- a bucket revision used to install a reconstructed bucket safely.

The directory can retain bodyless public timeline facts that existing readers
need for selection, authorization, and pagination. These facts do not contain
the encrypted message body. The bucket cache keeps the decoded records that
the current body state needs. It can omit obsolete or securely deleted private
body records.

The sequence slice is a current reconstruction recipe. A reducer can replace
or remove a sequence when a newer fact makes an older private body fact
unnecessary. The slice does not have to preserve every historical reduction
step.

This directory still grows with message count, but it retains compact routing
metadata instead of full event and encrypted-body objects. This decision
accepts that metadata growth for the first version.

### Replay and live application

During a cold replay, the timeline component builds the bucket directory from
events in stream order. It does not retain reconstructed message data for an
inactive bucket. It can populate a bucket that the cache already contains.

A `MessageBodyEvent` for a new message precedes its `MessagePostedEvent` in the
atomic write. The component keeps the body sequence as a pending reference
until the post arrives and selects the canonical bucket. A projection snapshot
must include pending references when its cutoff falls between these records.

An edit or retraction uses the message-ID locator to add the new sequence to
the bucket of the target message. A linked echo adds the sequence to the echo's
own bucket too. This target-bucket reference is in addition to the normal
occurrence-time reference. Chatto stores the sequence only once when both
references select the same bucket. The event's new timestamp does not move
either message to another bucket. When an affected bucket is in the cache, the
same projection apply operation updates the directory and the cached
materialization.

Global user key-shredding state stays outside individual bucket recipes. A
read applies that current state when it hydrates a reconstructed message.
Chatto does not copy one key-shredding sequence into every bucket that contains
a message from that user.

Secure deletion can remove private `MessageBodyEvent` records from `EVT` as
defined by ADR-007, ADR-011, and ADR-033. The exact occurrence recipe can keep
the deleted sequence, but reconstruction does not retain an obsolete,
retracted, or shredded body payload. Separate bounded cleanup state can retain
the sequence until the delete attempt finishes. A missing current body for a
visible message causes the bucket load to fail.

### Projection snapshots

The room-timeline part of a `ServerContentView` snapshot cohort stores the
bucket directory, exact room-event sequence slices, target-bucket mutation
references, message locators, pending references, and other required compact
metadata at the cohort cutoff.

It does not store the bucket cache, raw `MessageBodyEvent` records, or encrypted
message bodies. It can store the bodyless public timeline facts that form part
of the compact directory. A snapshot restore installs the directory and then
replays later EVT records before Chatto becomes ready. Without a valid
snapshot, Chatto replays all retained EVT history to rebuild the directory.

The snapshot remains disposable acceleration data. Loss of all projection
snapshots does not cause message loss. `EVT` remains the reconstruction
authority.

### Bucket reconstruction and cache

A historical read uses the directory to select a finite set of buckets. A
message-ID read uses the locator to select the owning bucket directly. Chatto
fetches the listed EVT records by their exact stream sequences and reduces
them in stream order.

Bucket reconstruction does not hold the `ServerContentView` apply barrier
while it performs NATS I/O. A load uses this procedure:

1. Under the timeline projection lock, capture the bucket recipe and revision.
2. Release the lock and fetch the referenced EVT records.
3. Re-enter the lock and compare the bucket revision.
4. Fetch the sequences that a concurrent apply added if the recipe changed.
5. Install the bucket only when it represents an exact known revision.

The `ServerContentView` apply barrier encloses every timeline reducer apply.
The component lock therefore detects any timeline change that can affect the
captured recipe without keeping the wider barrier across I/O.

Concurrent requests for the same bucket share one in-progress load. The load
has a separate 30-second lifetime, so cancellation by its first caller does
not cancel the load for other callers. Each caller can stop waiting with its
own context. Loads also have a bounded concurrency limit. A failed load does
not install partial state.

Materialized buckets live in one process-local, access-ordered cache. The
operator can configure the idle timeout. Chatto evicts an unpinned bucket after
it has not been used for that period. The default idle timeout is 15 minutes.
Each hydrated body read refreshes the bucket access time while it holds the
timeline projection lock. An eviction cannot clear the body between the read
decision and this refresh.
The first version does not use a byte budget and does not persist cache
contents. Each replica reconstructs and evicts its own buckets.

The current bucket is always pinned. The operator can also configure a recent
pinned period. Chatto pins every bucket whose time interval overlaps that
period. The default is four weeks. With one-week buckets, this normally pins
the current partial bucket and the four preceding buckets. Chatto reconstructs
all pinned buckets before core readiness. When a bucket leaves the pinned
period, normal idle tracking and eviction begin.

### Reads and authorization

Request-time authorization continues to use current state from
`ServerContentView` as defined by ADR-087 and ADR-089. Authorization evaluation
uses the stable content-view procedure. The final subject-tail validation from
ADR-087 remains the authorization decision point. Bucket reconstruction is
message-data hydration. It occurs outside a content-view read transaction.

A later authorization change does not cancel a decision that already passed
the stable request-time authorization procedure. A concurrent timeline apply
changes the bucket revision and makes reconstruction retry. No NATS or object-
storage I/O occurs while the content-view barrier is held.

Focused projections can keep compact all-history indexes when current API
behavior needs them. Thread reply lists, interaction relationships, current
pins, attachment locators, attachment asset IDs, and attachment counts can
select message IDs or buckets without retaining a second copy of message event
data. The attachment list ignores missing or deleted assets and selects the
requested page before it reconstructs message bodies. Those indexes remain
derived state and stay consistent with the timeline bucket directory.

## Consequences

- RAM no longer contains one full event and encrypted-body graph for every
  message in server history.
- Projection snapshots no longer contain a second copy of all message data.
- A cold replay still reads all retained EVT history, but it discards inactive
  message payloads after it builds their bucket metadata.
- The first read of an inactive bucket has NATS read and reduction latency.
  Later reads can use the cached materialization.
- The exact sequence slices and message locators still grow with history. This
  is smaller than retained message data, but it is not a constant-size model.
- A broad sequence slice can require many exact EVT reads. A later change can
  compress sequences or use ranges without changing bucket identity or read
  behavior.
- A busy time interval can create a large bucket. The first version accepts
  this risk. Operators can select a shorter interval than the one-week
  default, and a later change can add shards.
- The idle cache has no byte budget. A workload that repeatedly reads many
  large buckets can retain more RAM until the idle period expires. This is an
  accepted first-version tradeoff.
- The current bucket and the configured recent period stay in RAM. The default
  retains approximately four to five weekly buckets and reconstructs them
  before Chatto becomes ready.
- An interval configuration change causes a complete `ServerContentView` cold
  replay because snapshot cohorts restore as one complete generation.
- Late message mutations need a reliable message-ID-to-bucket locator and a
  race-safe cache installation procedure.
- Secure deletion must mark a body unavailable or superseded before it removes
  the referenced private record.

## Out of Scope

- Sequence compression, sequence ranges, and alternate index formats.
- Size-based bucket shards or byte-bounded caches.
- Replica-local projection persistence from Project 3.
- EVT expiration or archival.
- The public realtime protocol rework.
- Moving every compact thread, reaction, pin, or attachment index out of RAM.

## Related

- [ADR-007](ADR-007-per-user-encryption-with-crypto-shredding.md)
- [ADR-011](ADR-011-message-body-event-split.md)
- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-054](ADR-054-optional-projection-persistence.md)
- [ADR-073](ADR-073-define-the-loom-architecture.md)
- [ADR-087](ADR-087-request-time-authorization-with-aggregate-occ.md)
- [ADR-088](ADR-088-componentized-projections-behind-one-apply-barrier.md)
- [ADR-089](ADR-089-server-content-view.md)
- [Issue #2269](https://github.com/chattocorp/chatto/issues/2269)
