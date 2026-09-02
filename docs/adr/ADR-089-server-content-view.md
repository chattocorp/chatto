# ADR-089: Project Client-Readable EVT State into ServerContentView

**Date:** 2026-09-02

**Status:** Accepted

## Context

Chatto derives client-readable server content from several process-local
projections. Room metadata, membership, layout, timelines, threads, reactions,
calls, assets, users, configuration, and RBAC each have an independent EVT
consumer and replay frontier.

Writers must select and wait for every projection needed by the next read.
Operations that assemble a broad client view can wait for a captured EVT
cutoff, but the projections can advance independently while the operation
reads them. The result does not have one exact sequence that describes all
included EVT state.

ADR-051 defines realtime protocol 2. Its current server-side catch-up captures
an EVT cutoff, waits for several projections, and then reads their independent
models. `ServerContentView` replaces this internal readiness and capture
mechanism. This ADR does not change the public protocol, cursor, privacy, or
convergence rules. A later realtime refactor can also use the exact content
view and sequence.

Not all server state is EVT-backed client content. Authentication material,
presence, sessions, read markers, notification lifecycle state, search
indexes, and worker-specific state have different privacy, source-log, or
lifecycle contracts.

## Decision

Create one process-local componentized projection named `ServerContentView`.
It is Chatto's coherent view of client-readable content derived from the
primary `EVT` stream. One projector owns its consumer, apply barrier,
readiness, failure state, and applied EVT sequence.

`ServerContentView` uses the componentized projection capability from ADR-088.
Its projector owns one ordered `evt.>` consumer, one apply barrier, one
readiness state, and one applied global EVT sequence. Focused component models
remain separate behind that barrier.

### Included components

The content view contains these state areas:

| Area | State |
| --- | --- |
| Server presentation and preferences | Server profile, branding, Neighbor records, and client-visible user preferences |
| User directory | Account and profile state, custom status, verified-email state, bot ownership, and encrypted PII needed for authorized hydration |
| Rooms | Room metadata, membership, bans, Universal-room behavior, Room Groups, and sidebar layout |
| Timelines | Visible room entries, encrypted message bodies, tombstones, channel echoes, pins, and message hydration metadata |
| Threads | Thread identity, replies, participants, follow state, and interaction relationships |
| Reactions and calls | Current message reactions and active durable room-call state |
| Assets | Asset declarations, ownership, processing state, derivative relationships, deletion state, and client-visible references |
| Authorization and mentions | RBAC roles and decisions plus mentionable identities |
| Content support | Wrapped content-key facts and other non-client-facing indexes required to hydrate included content safely |

Component names and ownership can follow current domain model boundaries. The
table defines state scope, not one required Go type per row.

### Independent materializations

The following state does not join `ServerContentView`:

- password verifiers, authentication generations, bot API key verifiers,
  incoming-webhook verifiers, external identity subjects, and OAuth consent;
- OAuth client registration and invitation-capability state;
- presence, sessions, read markers, and other latest-value runtime state;
- notification-decision state and notification occurrences;
- the disk-backed message search index;
- runtime-unit-private projections such as the asset-processing projection;
  and
- materializations over a source log other than `EVT`.

These materializations keep an independent projector when they still need
ordered replay. The notification occurrence projection keeps its independent
`NOTIFICATIONS` sequence. Runtime-state indexes do not receive an EVT sequence.

### Sequence and reads

`ServerContentView` sequence `N` means that every included component has
examined all EVT records through global sequence `N`, and a capture at that
barrier contains no state from after `N`.

A broad API or realtime operation obtains component state and the sequence
through one capture. It can perform DTO assembly and network writes after it
releases the apply barrier. A focused component getter remains concurrency
safe through its component lock, but it does not claim one combined sequence.
A decision that combines components or depends on projection health uses a
`ServerContentView` read transaction. Internal focused handles share the view
projector's wait, readiness, and failure lifecycle; they do not own independent
replay frontiers.

Writers use the content-view projector for read-your-writes waits when the
response or next decision reads included state. The wait does not replace the
aggregate-specific OCC boundary used by the command.

An internal EVT sequence must not cross a public API boundary directly. A
public resume cursor that contains the sequence must remain confidential,
authenticated, viewer-bound, and stream-incarnation-bound.

The EVT sequence describes only the EVT-derived content. An operation that
also reconciles presence, read markers, notifications, or other runtime state
must identify those values as state from their own authority. It must not
claim that the EVT sequence orders them.

One realtime connection captures content and receives later EVT-derived
operations from the same Chatto replica. If a reconnect reaches another
replica, that replica waits until its local `ServerContentView` reaches the
sequence in the validated cursor before it plans resumed delivery. Chatto does
not coordinate one distributed in-memory content view across replicas.

### Request-time authorization

`ServerContentView` becomes the projection boundary for the authorization
inputs that it contains. This includes account state, bot ownership, room
metadata, membership, bans, Room Group placement, and RBAC state. Permission
resolution and other request-time authorization helpers read these components
through one content-view read transaction instead of several projection locks.

The stable authorization procedure from ADR-087 remains in effect:

1. Capture the current EVT subject tails for every relevant authorization
   input.
2. Wait until `ServerContentView` applies those positions.
3. Evaluate the complete authorization decision in one content-view read
   transaction.
4. Read the same subject tails again.
5. Repeat the procedure when an input changed during the decision.

The read transaction holds the view barrier only for pure in-memory work. It
does not perform NATS, KMS, object-storage, or other external I/O. The final
tail validation remains the authorization decision point defined by ADR-087.
A later concurrent authority change does not cancel the command.

Authentication, credential validity, sessions, OAuth client state,
invitation-capability state, and other excluded inputs keep their current
authorities and readiness checks. An operation that depends on both content-
view state and an excluded materialization must validate both boundaries. The
content-view barrier is process-local; it is not a cross-replica lock or a
commit fence.

Commands continue to use aggregate-specific OCC for domain invariants. They do
not use the global content-view sequence as a whole-stream OCC token. After a
conflict, a command repeats authorization and domain validation from its
original intent as required by ADR-087.

### Persistence

Portable persistence stores `ServerContentView` as one projection snapshot
cohort. Each included component has its own key, contract ID, and one or more
bounded parts, but all parts share one EVT cutoff and stream incarnation.

The initial migration uses one part for each current component. Each part
keeps ADR-050's 64 MiB payload limit, 80 MiB encrypted-object limit, and 72 MiB
decompression limit. The registered component set fixes the component-count
limit, and each initial component contract fixes its part-count limit at one.
The repository calculates the cumulative bound from those registered limits
with checked arithmetic and processes stored parts one at a time. An oversized
or invalid cohort selects cold replay. Time-based message parts are defined by
the later message-history decision, not by this ADR.

The snapshot repository installs only a complete compatible cohort. It does
not mix old per-projection snapshots, components from different cutoffs, or a
snapshot component with a cold-replayed component. An invalid cohort causes a
complete content-view cold replay.

The new content view uses a new projection and repository namespace. Existing
per-projection snapshot objects and pointers remain disposable data for old
binaries and expire through normal retention. The new version does not migrate
them.

### Migration and compatibility

The migration replaces all in-scope projector registrations, waits, snapshot
jobs, and diagnostics. Chatto does not run the old and new content projection
graphs together in production.

Use an exclusive-version server cutover. Stop old Chatto replicas before new
replicas begin to serve requests. Mixed old and new replicas are not supported
for this refactor, and the implementation does not include a rollout bridge.

The change does not alter durable EVT protobufs, subjects, event order, or OCC
boundaries. The new view must replay all supported historical EVT shapes. The
test suite, focused differential replay tests, snapshot tests, and projection
benchmarks are the acceptance signal for the replacement.

When accepted, this decision partially supersedes the independent core-
projection lifecycle described by ADR-033 and ADR-050. It also supersedes only
ADR-051's server-side multi-projection readiness and capture mechanism. It
preserves ADR-051's public protocol, cursor protection, privacy,
authorization, and convergence decisions. This ADR applies the componentized
Loom mechanism from ADR-088 and preserves the optional-persistence rule from
ADR-054.

## Consequences

- Client-content reads gain one explicit consistency coordinate.
- Current and later realtime implementations can capture content and its EVT
  sequence without coordinating many advancing projections.
- Read-your-writes wiring becomes smaller for included content.
- Stable request-time authorization reads common authority inputs from one
  sequence-consistent content view while it preserves ADR-087 subject-tail
  validation and aggregate OCC.
- Chatto uses one broad EVT delivery and decode path for the content view
  instead of many related consumers.
- A slow or failed included component affects the complete content view. State
  with a different failure or privacy boundary remains independent.
- The content view does not represent all state needed by a complete client.
  Transports still reconcile notifications, presence, read markers, and other
  runtime values from their own authorities.
- Component payloads avoid one monolithic snapshot object, but a restore still
  accepts or rejects the cohort as one unit.
- The first startup after upgrade cold-replays `ServerContentView` because old
  per-projection snapshots are not a compatible cohort.
- The refactor does not supply mixed-version deployment compatibility. This is
  acceptable during the 0.5 alpha cycle.

## Related

- [ADR-009](ADR-009-webhook-driven-voice-call-state.md)
- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-051](ADR-051-server-scoped-resumable-client-projection.md)
- [ADR-054](ADR-054-optional-projection-persistence.md)
- [ADR-056](ADR-056-extractable-nats-event-sourcing-framework.md)
- [ADR-073](ADR-073-define-the-loom-architecture.md)
- [ADR-084](ADR-084-separate-internal-protobufs-by-storage-contract.md)
- [ADR-087](ADR-087-request-time-authorization-with-aggregate-occ.md)
