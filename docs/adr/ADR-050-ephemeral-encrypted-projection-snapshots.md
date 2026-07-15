# ADR-050: Ephemeral Encrypted Projection Snapshots

**Date:** 2026-07-13

## Context

[ADR-033](ADR-033-event-sourced-state-with-projections.md) makes `EVT` the
source of truth and process-local projections the read side. Every Chatto
process currently rebuilds every projection by replaying `EVT` from the
beginning. Shared replay, bounded replay-only idempotency state, profiling
hooks, and reproducible benchmarks have reduced and exposed that cost, but
startup time and allocation still grow with retained event history.

Snapshots can accelerate startup by persisting a derived projection state and
replaying only later events. They also introduce a second representation of
state with failure, compatibility, storage, privacy, and multi-replica
concerns. In particular:

- a snapshot must never become necessary to recover domain state;
- projection implementations and their retained shapes evolve independently
  of the Chatto release version;
- a partially uploaded or stale generation must not become authoritative;
- configured deployments may store assets in NATS Object Store or S3;
- projection memory can contain decrypted PII or message content that must not
  be copied into a server-key-encrypted cache because doing so would weaken
  crypto-shredding; and
- multiple Chatto replicas can attempt the same background work.

Event expiration, compaction, and archival are separate decisions. Chatto does
not need them in order to use snapshots for startup acceleration.

## Decision

Add optional, ephemeral projection snapshots under the following contract.

### EVT remains permanent and authoritative

`EVT` remains the only durable source of domain truth and is retained
indefinitely. Snapshots do not permit event expiration, truncation, compaction,
or archival. Deleting every snapshot may make startup slower but must not
change reconstructed state or lose data.

Missing, corrupt, incompatible, undecryptable, or otherwise invalid snapshots
fall back automatically to replay from `EVT`. Snapshot failures do not prevent
Chatto from starting when `EVT` itself is available. Bootstrap snapshot loads
have a 15-second deadline so a stalled object backend cannot hold core startup
indefinitely.

### Compatibility is projection-scoped

Each snapshot records three distinct versions:

- an envelope format version for framing, compression, encryption, and
  integrity metadata;
- a projection compatibility ID, such as `threads-v1`, for the meaning and
  serialized shape of one projection; and
- the producing Chatto version for diagnostics only.

A projection compatibility ID changes when restored state would no longer be
equivalent to replay under the current projection logic. Unrelated Chatto
releases do not invalidate a snapshot. The initial implementation accepts only
exact compatibility IDs and performs no snapshot migration. Forward upgrades
and rollbacks ignore unknown IDs and cold-replay instead.

Snapshot codecs use explicit protobuf messages. They do not serialize internal
Go structs through reflection, `gob`, or generic JSON. A codec may omit derived
indexes and rebuild them during restore.

### Snapshots reuse the configured binary-storage backend

Snapshot persistence uses a private internal blob-store boundary backed by the
configured asset-storage backend:

- NATS-backed deployments store snapshot objects in the dedicated
  `PROJECTION_SNAPSHOTS` NATS Object Store; and
- S3-backed deployments store snapshot objects in the configured S3 bucket.

Snapshot generations use a reserved internal prefix such as
`internal/projection-snapshots/v1/objects/<opaque-key-epoch>/`. They are not assets: they do not produce
asset lifecycle events, receive signed URLs, participate in user-facing asset
APIs, or enter asset cleanup decisions.

Namespace membership is immutable once shipped. `v1` contains only Threads.
`v2` contains Room Directory, Server Config, Room Group Layout, Room Timeline,
Call State, Assets, Reactions, Content Keys, RBAC, and Mentionables. Adding a
projection to either namespace is forbidden; a different cohort requires a new
namespace version. This
prevents an older replica's cleaner from treating a newer projection's
generations as abandoned during a mixed-version rollout.

The small encrypted current/previous pointer lives in `RUNTIME_STATE`, using KV
`Create` and revisioned `Update` for optimistic concurrency. This is true for
both NATS and S3 payload backends. A stale lease holder can upload a generation,
but it cannot regress a newer pointer; a failed pointer CAS rolls back the
unpublished upload and leaves the newer history intact.
The pointer also carries each generation's cutoff sequence, EVT incarnation,
and projection compatibility ID. A writer rejects a captured state that does
not advance the current generation for the same EVT incarnation and
compatibility contract. Revision OCC alone is insufficient because a writer
can capture old projection state before reading a newer pointer revision.

NATS-backed snapshots are included in `chatto backup` as opaque encrypted
objects because `PROJECTION_SNAPSHOTS` is a file-backed JetStream resource.
S3-backed snapshot generations follow the deployment's S3 backup policy, like
S3-backed user assets; the Chatto backup command does not copy either kind of
S3 object into its NATS archive. The encrypted pointer remains in the backed-up
`RUNTIME_STATE` bucket but is disposable when its S3 generation is absent. A
carried snapshot can avoid reconstructing snapshotted projection state when the
restored deployment retains the same `core.secret_key`. Snapshots remain
optional: an absent or undecryptable snapshot causes cold replay. Backup
tooling does not decrypt, interpret, or make snapshots part of backup
correctness.

Asset-storage migration may copy the reserved snapshot namespace when doing so
is cheap, but does not promise to preserve it. Moving to another storage backend
without snapshots causes cold replay and generation of new snapshots.

### Payloads are compressed, encrypted, and privacy-reviewed

Each projection payload is deterministically protobuf-encoded, compressed, and
then encrypted with an authenticated cipher before upload. The initial
envelope uses XChaCha20-Poly1305 with a random salt and nonce and a key derived
from `core.secret_key` using a snapshot-specific domain separator. Rotating
`core.secret_key` invalidates existing snapshots and causes cold replay.
Generation object paths include an opaque epoch derived from that key, so a
new-key cleaner cannot delete generations still used by an old-key replica
during a rolling secret change. Old key epochs are not automatically swept by
the new key and remain subject to the deployment's storage lifecycle or later
key-migration tooling.

The pre-epoch canary layout placed generation IDs directly below
`internal/projection-snapshots/v1/objects/`. Upgrading from that unshipped
canary layout intentionally cold-replays and does not let the new cleaner cross
the unauthenticated epoch boundary. Those legacy objects require the storage
provider's lifecycle policy or explicit later migration tooling.

The unencrypted envelope contains only the framing data required to select the
decryption scheme, derive the key, and authenticate the ciphertext: a magic
value, envelope version, key-scheme identifier, random salt and nonce, and the
opaque object generation ID. Projection names, compatibility IDs, EVT stream
identity and sequence, creation time, checksums, entry counts, and other
semantic metadata live inside the encrypted authenticated payload. Object names
and backend metadata are opaque and generic. Ciphertext length and backend
write time remain observable; padding policies may be added later if those
side channels prove material.

The envelope identifies its key scheme independently from its format. The
initial scheme is `core-secret-hkdf-v1`. A later release may add a server-scoped
KMS or external key provider, read both schemes during a transition, and write
new generations with the replacement scheme. Snapshots require no in-place key
migration because incompatible generations can always be discarded and rebuilt.

Envelope encryption is defense in depth and does not make arbitrary in-memory
state eligible for persistence. A snapshot codec must not contain decrypted
PII, plaintext message bodies, tokens, passwords, auth codes, unwrapped keys,
or other secret material. Sensitive fields must remain in a representation
whose existing crypto-shredding semantics are preserved. Projections that
cannot meet this requirement are not snapshot-eligible.

### Generations are immutable and self-validating

A snapshot generation is one immutable encrypted bundle containing its
manifest and projection payload. The encrypted manifest records at least:

- generation ID;
- EVT stream name, cutoff sequence, and its versioned incarnation identity;
- projection key, compatibility ID, and producer version;
- payload size and checksum; and
- creation time.

Writers upload the complete immutable bundle before replacing an encrypted
pointer containing the current and previous opaque generation IDs. The pointer
is stored in NATS KV independently of the payload backend and uses revision OCC,
so concurrent or stale writers cannot regress its history. Loaders validate the
current generation, then the previous generation, and cold-replay if neither is
valid. The pointer's KV key is derived from `core.secret_key` and the projection
key so it does not disclose which projection it addresses.

Restore validates the envelope, authentication tag, manifest, projection
compatibility, cutoff bounds, and the current EVT incarnation identity before
mutating a live projection. Chatto stores the opaque identity in EVT stream
metadata so it survives process reconstruction and backup restore but changes
when EVT is deleted and recreated. Missing metadata is deterministically
derived once from the stream creation time so concurrent replicas converge,
then persisted; `StreamInfo.Created` is not used for later comparisons because
it is not stable across embedded NATS process reconstruction.
Projection restore codecs are transactional: a
rejected payload must leave prior state unchanged so the projector can reset to
its cold-start state and replay all of `EVT`. Capturing a snapshot must bind
projection state and its applied EVT sequence at one projector-owned barrier;
reading projection state and a projector cursor in separate unsynchronized
operations is invalid.

Each projection retains the current and previous referenced generations. A writer deletes
the generation that falls out of that window and rolls back a newly uploaded
generation when pointer publication reports failure. Writers treat a missing
pointer as an empty history and may replace a cryptographically or structurally
invalid pointer at its observed revision. A storage transport error while
reading the pointer aborts the write without uploading a generation or changing
either retained fallback. A revision conflict aborts publication and rolls back
the uploaded generation rather than overwriting newer history.

An elected backend-listing sweeper reclaims objects abandoned by a process crash,
stale writer, or failed rollback deletion. It first
authenticates every registered projection pointer and completes a read-only
inventory of the current opaque key epoch while collecting a bounded batch of unreferenced generation objects
that are at least 24 hours old. Only after the inventory succeeds does it check
lease ownership and delete the collected batch. Pointers are not blob objects
and are never part of backend inventory. Current
and previous generations, recent objects, malformed keys, and unknown namespace
entries are never deleted. Pointer or listing failures delete nothing; lease
loss, cancellation, or deletion failure stops the pass.

The sweeper runs only while projection snapshots are enabled. One replica holds
a separate `MEMORY_CACHE` lease, waits a random 5-10 minutes after normal boot,
and then sweeps every six hours. Failed passes and successful passes that reach
a deletion limit retry after 30 minutes. Each pass has a five-minute deadline
and deletes at most 100 objects or 1 GiB. These fixed limits bound cleanup
work and provide catch-up behavior, but they are not a hard namespace storage
cap. Production evidence still needs to inform a final operator-tunable budget.

The repository rejects projection payloads larger than 64 MiB and bounds encrypted
and decompressed representations separately. This is a guardrail against
restart-loop memory amplification, not a final production storage budget;
projections that outgrow it cold-replay and log the failed generation attempt.

### One elected worker creates snapshots

Snapshot generation is background work owned by one replica at a time through
the existing distributed lease mechanism backed by `MEMORY_CACHE`. The worker
starts only after normal projection startup is complete, refreshes its lease
while building, and rechecks ownership before publishing a completed manifest.
Loss of the lease abandons the in-progress generation.

The lease reduces duplicate work but is not the correctness boundary.
Immutable generation bundles, current/previous fallback, and validation keep
stale workers and interrupted uploads harmless. Loaders never trust
process-local ownership state.

Initialization is best-effort as well: snapshot Object Store, repository, EVT
identity, projector configuration, and lease failures disable the affected
snapshot workers and log the reason. They do not prevent core startup when EVT
is available.

The worker attempts one generation per eligible projection after boot, and only
when that projection has advanced beyond its currently referenced compatible
snapshot. A failure for one projection is logged and does not prevent the
remaining jobs from running. It does not add a periodic generation cadence or
operator-tunable interval.
Cleanup uses its own elected periodic worker and does not delay generation,
readiness, or request handling.

### Eligible projections share a coordinated replay frontier

`ThreadProjection` remains in the permanently frozen `v1` namespace. The ten
additional eligible projections use the permanently frozen `v2` namespace.
Each projection retains its own compatibility ID, pointer, current/previous
generations, cutoff, and fallback behavior; the namespace is a cleanup and
mixed-version safety boundary, not one atomic multi-projection bundle.

The initial 0.5 implementation uses compatibility ID `threads-v1`. It does not
import pre-EVT `thread_follow.*` records from `RUNTIME_STATE`; follow state is
rebuilt only from durable `ThreadFollowedEvent` and `ThreadUnfollowedEvent`
facts.

Eligible projection snapshots load concurrently. Once every restore finishes,
their shared ordered `evt.>` consumer begins at one greater than the lowest
restored cutoff. Any eligible projection with a non-zero startup target and no
usable snapshot forces that cohort to sequence 1. Projections with no matching
EVT history do not constrain the frontier. Every projector still skips events
through its own restored cutoff, so different generation cutoffs are safe.
Boot-time waiters are released through the same sequence-advance path used by
live events even when they begin waiting while restore is in flight.

`UserProjection` retains encrypted PII source fields and materializes them only
at read boundaries, but it still retains password verifiers and other
authentication state whose snapshot representation has not been approved. It
runs in an independent consumer restricted to `evt.user.>`, so a required cold
user replay cannot pull the room-heavy snapshot cohort back to sequence 1.
Extending snapshot support to Users requires a codec that preserves per-user
crypto-shredding and does not persist credential material under the snapshot
envelope alone.

Every eligible codec uses an explicit protobuf and transactional restore.
`RoomTimelineProjection` persists immutable event envelopes and encrypted
`MessageBody` envelopes without decrypting content, then rebuilds indexes.
`MentionablesProjection` persists wrapped DEK records and the latest encrypted
login source event rather than its plaintext handle map. Shred markers and the
existing durable representation of non-secret metadata are preserved.

Snapshot persistence is disabled by default. Operators enable it with
`[core].projection_snapshots = true` or
`CHATTO_CORE_PROJECTION_SNAPSHOTS=true`.

## Consequences

- Chatto can start the room-heavy projection cohort from the lowest compatible
  snapshot cutoff and replay only the later EVT delta without weakening the
  authority or retention of `EVT`.
- Snapshots naturally use cheaper S3 storage where configured while preserving
  the zero-dependency NATS default. NATS snapshots follow Chatto's NATS backup;
  S3 snapshots follow the operator's S3 backup policy.
- Snapshot payloads consume additional storage, network bandwidth, and
  background CPU. Compression, current/previous generation retention, and the
  elected grace-period sweeper reclaim normal and abandoned objects. A hard
  namespace storage cap, final generation cadence, and operator-tunable budgets
  remain follow-up decisions informed by production measurements. Key rotation
  intentionally leaves the prior opaque generation epoch for provider lifecycle
  or later migration tooling rather than letting a new-key cleaner cross epochs.
- Upgrades and rollbacks remain safe but may cold-replay when projection
  compatibility changes.
- Storage-backend availability can affect the optimization but cannot affect
  correctness. A snapshot backend outage falls back to EVT replay.
- Reusing the asset backend requires a strict namespace and backup boundary so
  derived internal objects are not mistaken for user assets. Backups may carry
  them, but never depend on them.
- The existing no-op `Projection.Snapshot` and `Restore` methods gain a concrete
  orchestration contract without requiring every projection to implement them.
- Snapshot codecs become maintained projection interfaces. Changes to
  projection semantics require an explicit compatibility decision.
- `UserProjection` still cold-replays its focused subject family. Startup time
  therefore includes user replay, snapshot loading, index reconstruction, and
  the eligible cohort's tail replay.

## Out of Scope

- Expiring, compacting, truncating, or archiving `EVT`.
- Depending on snapshots for backup, disaster recovery, or domain correctness.
- Snapshot migration between projection compatibility IDs.
- Incremental or chained snapshots.
- Persisting decrypted sensitive projection state.
- A generic snapshot format shared by all projections.
- Selecting a final cadence, storage budget, or default-on rollout policy.
- Extending `chatto backup` to include S3-backed assets or snapshots.
