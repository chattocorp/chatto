# ADR-090: Hydrate Room Timeline Payloads from EVT

**Date:** 2026-09-03

**Status:** Accepted

## Context

`ServerContentView` gives client-readable EVT state one ordered apply barrier
and one exact EVT sequence. Its Room Timeline component still retains every
visible timeline event and every current encrypted message body as protobuf
payloads in RAM. Its snapshot also stores these payloads. Memory and snapshot
size therefore grow with complete room history.

Most room timeline reads need two different forms of data. They need compact
state to select and order entries, and they need complete event payloads to
build the response. EVT is the durable source of the complete payloads and
already supports an exact read by stream sequence.

Time buckets can limit a cache or a disk-backed index. They do not limit RAM if
the projection still retains one complete event payload for each historical
entry. A clear hydration boundary also lets a disposable cache accelerate
repeated reads without making cached data part of projected state.

## Decision

The Room Timeline component stores a compact sequence index. It does not store
complete EVT event payloads or message body payloads.

Each indexed timeline entry stores only the data needed for projected reads and
domain decisions. This includes the EVT sequence, event ID, room ID, actor ID,
creation time, event type, thread root, and echo relationship when applicable.
The component keeps its existing derived indexes for room order, event lookup,
pins, tombstones, echo links, attachment-bearing messages, slow mode, and
secure deletion.

Message body state stores the current body-event sequence and the sequences of
superseded body events. It also stores small derived values that a bounded read
needs before payload hydration, such as the current attachment count.
Retraction and key shredding deactivate the current reference but retain the
complete sequence history needed for secure deletion.

The Room Timeline snapshot stores the compact index and derived state. It does
not store complete timeline events or message bodies. The changed snapshot
schema selects a new component contract. An older or invalid snapshot causes a
normal cold EVT replay.

### Exact EVT reader

Chatto's typed EVT adapter provides exact sequence reads. The reader validates
and decodes one stored EVT record and can read a bounded set of sequences with
limited concurrency. It removes duplicate sequence requests and returns
results in caller order.

The first implementation uses leader-backed JetStream stream reads for cache
misses. A later implementation can use `DirectGet`, a broker batch operation,
or another read method behind the same boundary. Such an implementation must
preserve read-your-writes and must fall back when a replica is behind.

The shared event framework wraps exact stream reads in an optional
process-local message cache. The cache key is the exact sequence within its
bound stream. It stores copied opaque record bytes and broker metadata, not
decoded Chatto protobuf values. Successful reads extend the entry's idle
lifetime. Failed, missing, and invalid broker responses are not cached.

Chatto enables this cache for timeline hydration. The sliding idle lifetime is
set by `core.evt_read_cache_idle_ttl` and defaults to 15 minutes. A background
reader lifecycle removes expired entries. Secure deletion also removes the
affected sequence from the local cache and prevents an in-progress read from
putting it back. A NATS continuity loss clears the complete cache before
application recovery. Each replica owns its cache. Cache loss or a cache miss
causes a normal EVT read.

### Timeline hydration

A room timeline hydrator converts compact projection references into complete
events and current message bodies. It validates each fetched record against the
projected sequence, event ID, event kind, room ID, and message target. A missing
or mismatched active record is an error. It must not look like normal absence
or deletion.

The hydrator uses this procedure:

1. Capture an immutable read plan from the Room Timeline component.
2. Release the `ServerContentView` apply barrier.
3. Read and decode the required EVT records.
4. Revalidate the projected references that can change.
5. Retry a bounded number of times if an edit, retraction, or shred made a
   mutable body reference stale.

The hydrator never performs NATS, KMS, object-store, or other external I/O
while it holds the projection apply barrier. A page keeps its projected order
even when the reader loads records concurrently.

Timeline entry references are immutable after projection. A read can complete
after a later visibility change, consistent with Chatto's request-time
authorization rule. Current body references are mutable and must be
revalidated after EVT I/O.

A current message body is loaded from its active body-event sequence. A
retracted, hidden, or key-shredded message has no active body and produces the
existing tombstone behavior. The fetched encrypted body is kept only for the
read that decrypts or maps it.

### Authorization

Authorization remains a projected decision against `ServerContentView`, as
defined by ADR-087 and ADR-089. Complete events fetched from EVT are response
data. They are not an authorization source.

The operation first completes its stable request-time authorization procedure
against the content view. It then selects authorized timeline references and
hydrates only those references. A fetched payload cannot add visibility that
the projected plan did not grant. The hydrator validates room and message
identity so a corrupt or incorrect index cannot cross a room boundary.

The existing request-time rule still permits a command or read to complete
after a later concurrent authority change. This decision does not add a global
authorization fence or a cross-replica lock.

A later realtime snapshot can use the same optimistic read plan with an exact
content-view capture. It can capture compact state, body references, and
sequence `N` under the apply barrier, hydrate after it releases the barrier,
and then verify that every selected mutable reference is unchanged. If a
selected reference changed or disappeared, it must retry the complete capture
at a later sequence. Unrelated later events do not change the detached state
captured at `N`. This ADR does not implement that realtime protocol.

### Scope

This decision adds only the individual stream-message cache around the exact
EVT reader. It does not cache completed timeline pages or time buckets. A later
cache can retain completed timeline hydration buckets when measurement shows
that the additional policy is useful. That cache must remain disposable,
bounded, and non-authoritative.

This decision also does not require time buckets for the in-memory index. Per-
room sequence-ordered slices support bounded pagination and event-ID lookup.
Time partitions remain an option for a later disk-backed index or payload
cache.

## Consequences

- Complete room timeline and message body payloads no longer make projection
  RAM and snapshots grow with payload size.
- The compact index still grows with the number of timeline and message body
  facts. A later local materialization can bound that index in RAM.
- Timeline reads perform EVT I/O and can fail when EVT is unavailable or an
  indexed record is missing or corrupt.
- Repeated timeline reads can use copied EVT records from the process-local
  cache until their sliding idle lifetime expires.
- Page reads can fetch only the records required for the page. Attachment reads
  can use projected counts to apply pagination before body hydration.
- One hydration boundary makes a later cache, `DirectGet`, or broker batch read
  an implementation change instead of another projection redesign.
- Projection application remains deterministic and contains no external I/O.
- Snapshots become smaller and do not duplicate encrypted message content.

## Related

- [ADR-007](ADR-007-per-user-encryption-with-crypto-shredding.md)
- [ADR-011](ADR-011-message-body-event-split.md)
- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-087](ADR-087-request-time-authorization-with-aggregate-occ.md)
- [ADR-088](ADR-088-componentized-projections-behind-one-apply-barrier.md)
- [ADR-089](ADR-089-server-content-view.md)
