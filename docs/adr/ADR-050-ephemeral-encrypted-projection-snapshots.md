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
not need them in order to validate snapshot persistence and restoration. Using
snapshots to reduce total startup time requires a later change to the shared
replay frontier.

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

- NATS-backed deployments store snapshot objects in NATS Object Store; and
- S3-backed deployments store snapshot objects in the configured S3 bucket.

Snapshot objects use a reserved internal prefix such as
`internal/projection-snapshots/v1/`. They are not assets: they do not produce
asset lifecycle events, receive signed URLs, participate in user-facing asset
APIs, or enter asset cleanup decisions.

NATS-backed snapshots are included in `chatto backup` as opaque encrypted
objects because `SERVER_ASSETS` is part of the JetStream backup. S3-backed
snapshots follow the deployment's S3 backup policy, like S3-backed user assets;
the Chatto backup command does not copy either kind of S3 object into its NATS
archive. A carried snapshot can avoid reconstructing the snapshotted projection
state when the restored deployment retains the same `core.secret_key`, but the
Thread-only canary does not reduce total startup time. Snapshots remain
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
is not authoritative because the shared backend may be S3 and cannot be assumed
to provide JetStream KV OCC semantics. Loaders validate the current generation,
then the previous generation, and cold-replay if neither is valid. The pointer's
object locator is derived from `core.secret_key` and the projection key so it
does not disclose which projection it addresses.

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

The canary retains the current and previous referenced generations. It deletes
the generation that falls out of that window and rolls back a newly uploaded
generation when pointer publication reports failure. Writers treat a missing
pointer as an empty history and may replace a cryptographically or structurally
invalid pointer. A storage transport error while reading the pointer aborts the
write without uploading a generation or changing either retained fallback.

An elected backend-listing sweeper bounds objects abandoned by a process crash,
stale writer, failed rollback deletion, or `core.secret_key` rotation. It first
authenticates every registered projection pointer and completes a read-only
inventory. A second streaming pass deletes only unreferenced generation objects
and obsolete locator-shaped pointer objects that are at least 24 hours old.
Active pointers, current and previous generations, recent objects, malformed
keys, and unknown namespace entries are never deleted. Pointer or first-pass
listing failures delete nothing; lease loss, cancellation, listing failure, or
deletion failure during the second pass stops further deletion.

The sweeper runs only while projection snapshots are enabled. One replica holds
a separate `MEMORY_CACHE` lease, waits a random 5-10 minutes after normal boot,
and then sweeps every six hours. Failed passes retry after 30 minutes. Each pass
has a five-minute deadline and deletes at most 100 objects or 1 GiB. These fixed
canary limits bound cleanup work while production evidence informs any future
operator-tunable storage budget.

The canary rejects projection payloads larger than 64 MiB and bounds encrypted
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

The initial canary attempts one generation after boot, and only when the Thread
projection has advanced beyond the currently referenced compatible snapshot.
It does not add a periodic generation cadence or operator-tunable interval.
Cleanup uses its own elected periodic worker and does not delay generation,
readiness, or request handling.

### ThreadProjection is the first canary

The first production-shaped vertical slice snapshots `ThreadProjection` only.
It has meaningful replay cost and existing replay benchmarks, while its
canonical state can be represented by identifiers, sequences, timestamps,
counters, follow state, shred markers, and replay-compatibility metadata rather
than message bodies or decrypted PII.

The initial 0.5 implementation uses compatibility ID `threads-v1`. It does not
import pre-EVT `thread_follow.*` records from `RUNTIME_STATE`; follow state is
rebuilt only from durable `ThreadFollowedEvent` and `ThreadUnfollowedEvent`
facts.

The canary must prove that restoring at sequence `S` and applying later events
produces the same observable and canonical state as replaying the complete
fixture. Other projections continue to cold-replay. The current projector
architecture fans one ordered `evt.>` replay out to all projections, and core
startup completes only when every projection reaches its startup target. A
single restored projection therefore does not advance the shared replay start
and is not expected to improve total wall-clock startup time. It may not improve
the Thread projector's reported completion time either, because that projector
still waits for the shared fanout to reach its tail after skipping old thread
applications.

The canary validates storage, metadata confidentiality, encryption,
compatibility, publication, restore, tail replay, fallback, and state
equivalence. Any reduction in thread-application CPU is secondary. Real startup
acceleration requires enough compatible projection snapshots to advance the
minimum shared replay frontier, or a later decision to split projections into
independent replay cohorts. Neither outcome is assumed by this canary.

Broader snapshot support requires per-projection privacy review and benchmark
evidence. `RoomTimelineProjection` is a likely later candidate but is excluded
from the canary because its retained body and event state requires a more
careful crypto-shredding analysis.

Snapshot persistence is disabled by default. Operators enable it with
`[core].projection_snapshots = true` or
`CHATTO_CORE_PROJECTION_SNAPSHOTS=true`. The initial implementation snapshots
only `ThreadProjection`; extending coverage and advancing the shared replay
frontier remain separate follow-up work.

## Consequences

- Chatto can validate the storage, encryption, compatibility, and restore
  mechanics needed for future startup acceleration without weakening the
  authority or retention of `EVT`.
- Snapshots naturally use cheaper S3 storage where configured while preserving
  the zero-dependency NATS default. NATS snapshots follow Chatto's NATS backup;
  S3 snapshots follow the operator's S3 backup policy.
- Snapshot payloads consume additional storage, network bandwidth, and
  background CPU. Compression, current/previous generation retention, and the
  elected grace-period sweeper bound normal and abandoned object growth. Final
  generation cadence and operator-tunable storage budgets remain follow-up
  decisions informed by production measurements.
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
- The canary intentionally does not test total startup acceleration. It can
  justify or reject the storage and restore mechanism before the larger step of
  advancing the shared replay frontier or introducing replay cohorts.

## Out of Scope

- Expiring, compacting, truncating, or archiving `EVT`.
- Depending on snapshots for backup, disaster recovery, or domain correctness.
- Snapshot migration between projection compatibility IDs.
- Incremental or chained snapshots.
- Persisting decrypted sensitive projection state.
- A generic snapshot format shared by all projections.
- Selecting final cadence, storage-budget, or rollout defaults before the
  `ThreadProjection` canary is measured.
- Extending `chatto backup` to include S3-backed assets or snapshots.
