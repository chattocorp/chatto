# ADR-050: Ephemeral Encrypted Projection Snapshots

**Date:** 2026-07-13

## Context

[ADR-033](ADR-033-event-sourced-state-with-projections.md) makes `EVT` the
source of truth and process-local projections the read side. Every Chatto
process currently rebuilds every projection by replaying `EVT` from the
beginning. Bounded replay-only idempotency state, profiling hooks, and
reproducible benchmarks have reduced and exposed that cost, but
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

### Snapshot contracts are projection-scoped

Each snapshot records three distinct identifiers:

- an envelope format version for framing, compression, encryption, and
  integrity metadata;
- an opaque, projection-scoped contract ID, such as `v1`, covering serialized
  state, replay semantics, consumed event families, and cutoff meaning; and
- the producing Chatto version for diagnostics only.

A projection contract ID changes when restored state would no longer be
equivalent to replaying EVT through the recorded cutoff. Unrelated Chatto
releases do not invalidate a snapshot. Contract IDs are bounded path-safe,
projection-local equality tokens, not ordered schema or Chatto versions, and
Chatto performs no migration between them. Forward upgrades and rollbacks use
independent contract-scoped pointers and cold-replay when their own contract
has no usable generation.

Snapshot codecs use explicit protobuf messages. They do not serialize internal
Go structs through reflection, `gob`, or generic JSON. A codec may omit derived
indexes and rebuild them during restore.

### Snapshots reuse the configured binary-storage backend

Snapshot persistence uses a private internal blob-store boundary backed by the
configured asset-storage backend:

- NATS-backed deployments store snapshot objects in the dedicated
  `PROJECTION_SNAPSHOTS` NATS Object Store; and
- S3-backed deployments store snapshot objects in the configured S3 bucket.

Snapshot generations use the reserved per-projection prefix
`internal/projection-snapshots/{projection}/{contract}/objects/{opaqueKeyEpoch}/{generationId}`.
They are not assets: they do not produce
asset lifecycle events, receive signed URLs, participate in user-facing asset
APIs, or enter asset cleanup decisions.

Contract IDs are local to one projection. Adding another projection does not
change an existing contract. Pointer keys and generation paths include the
projection and contract, so forward and rollback deployments cannot read,
rotate, delete, or apply no-regression checks across contracts.

The small encrypted current/previous pointer lives in `RUNTIME_STATE`, using KV
`Create` and revisioned `Update` for optimistic concurrency. This is true for
both NATS and S3 payload backends. A stale lease holder can upload a generation,
but it cannot regress a newer pointer; a failed pointer CAS rolls back the
unpublished upload and leaves the newer history intact.

The pointer also carries each generation's cutoff sequence, creation time, EVT
incarnation, and projection contract ID. A writer may refresh the same
cutoff once it is old, but the repository rejects a redundant refresh when a
previous lease holder already published a fresh, authenticated, usable
generation. Missing or corrupt current objects are repaired instead. It rejects
a lower cutoff for the same EVT incarnation and snapshot contract.
Revision OCC prevents a concurrent writer from replacing newer history.

Changing a contract leaves the prior encrypted pointer untouched. This keeps
rollback acceleration available until its generation objects expire and lets a
new contract begin at any valid cutoff without comparing it to the old
contract's frontier. Superseded pointers are harmless encrypted runtime
records, and their generation objects expire through the normal retention
policy.

NATS-backed snapshots are included in `chatto backup` as opaque encrypted
objects because `PROJECTION_SNAPSHOTS` is a file-backed JetStream resource.
S3-backed snapshot generations follow the deployment's S3 backup policy, like
S3-backed user assets; the Chatto backup command does not copy either kind of
S3 object into its NATS archive. The encrypted pointer remains in the backed-up
`RUNTIME_STATE` bucket but is disposable when its S3 generation is absent. A
carried snapshot can avoid reconstructing snapshotted projection state when the
restored deployment retains the same `core.secret_key`.

Snapshots remain optional: an absent or undecryptable snapshot causes cold
replay. Backup tooling does not decrypt, interpret, or make snapshots part of
backup correctness.

Asset-storage migration may copy the reserved snapshot namespace when doing so
is cheap, but does not promise to preserve it. Moving to another storage backend
without snapshots causes cold replay and generation of new snapshots.

### Payloads are compressed, encrypted, and privacy-reviewed

Each projection payload is deterministically protobuf-encoded, compressed, and
then encrypted with an authenticated cipher before upload. The initial
envelope uses XChaCha20-Poly1305 with a random salt and nonce and a key derived
from `core.secret_key` using a snapshot-specific domain separator. Rotating
`core.secret_key` invalidates existing snapshots and causes cold replay.
Generation object paths include an opaque epoch derived from that key. Storage
expiry is age-based and may remove generations from any current key epoch once
they exceed the configured retention. A missing generation always causes cold
replay.

Earlier canary layouts grouped projections under global `v1`, `v2`, and `v3`
directories. Upgrading intentionally cold-replays and publishes the new
per-projection layout. Application S3 expiry does not guess at legacy keys;
those objects require provider lifecycle or explicit operator cleanup. The
dedicated NATS Object Store TTL applies to both layouts.

The unencrypted envelope contains only the framing data required to select the
decryption scheme, derive the key, and authenticate the ciphertext: a magic
value, envelope version, key-scheme identifier, random salt and nonce, and the
opaque object generation ID. Projection names, contract IDs, EVT stream
identity and sequence, creation time, checksums, entry counts, and other
semantic metadata live inside the encrypted authenticated payload. Object paths
reveal the fixed projection key and contract ID, but not server data,
EVT positions, or creation metadata.

Ciphertext length and backend write time remain observable; padding policies
may be added later if those side channels prove material.

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
- projection key, contract ID, and producer version;
- payload size and checksum; and
- creation time.

Writers upload the complete immutable bundle before replacing an encrypted
contract-scoped pointer containing the current and previous opaque generation
IDs. The pointer is stored in NATS KV independently of the payload backend and
uses revision OCC,
so concurrent or stale writers cannot regress its history. Loaders validate the
current generation, then the previous generation, and cold-replay if neither is
valid. The pointer's KV key is derived from `core.secret_key`, the projection
key, and contract ID so it does not disclose which contract it addresses.

Restore validates the envelope, authentication tag, manifest, projection
contract, cutoff bounds, and the current EVT incarnation identity before
mutating a live projection. Chatto stores the opaque identity in EVT stream
metadata so it survives process reconstruction and backup restore but changes
when EVT is deleted and recreated. Missing metadata is deterministically
derived once from the stream creation time so concurrent replicas converge,
then persisted; `StreamInfo.Created` is not used for later comparisons because
it is not stable across embedded NATS process reconstruction.

Projection restore codecs are transactional: a rejected payload must leave
prior state unchanged so the projector can reset to its cold-start state and
replay all of `EVT`. Capturing a snapshot must bind projection state and its
applied EVT sequence at one projector-owned barrier. Reading projection state
and a projector cursor in separate unsynchronized operations is invalid.

Each projection pointer retains current and previous generation references. A
writer deletes the generation that falls out of that window and rolls back a
newly uploaded generation when pointer publication reports failure. Writers treat a missing
pointer as an empty history and may replace a cryptographically or structurally
invalid pointer at its observed revision. A storage transport error while
reading the pointer aborts the write without uploading a generation or changing
either retained fallback. A revision conflict aborts publication and rolls back
the uploaded generation rather than overwriting newer history.

`core.projection_snapshot_retention` controls generation lifetime and defaults
to seven days. NATS-backed deployments apply it as the TTL of the dedicated
`PROJECTION_SNAPSHOTS` Object Store, so JetStream owns expiry.

S3-backed deployments use age-based expiry after each elected publication pass
unless `core.projection_snapshot_s3_cleanup` is disabled for an external
provider lifecycle policy. Application expiry lists only the exact reserved
root and accepts only the per-projection generation grammar. Before deletion it
uses `HEAD` to require the expected content type and the private
`chatto-object-type=projection-snapshot` metadata marker. It deletes marked
objects older than retention regardless of pointer references. Deleting a
current or previous generation can only cause cold replay.

An S3 expiry pass has a five-minute deadline and deletes at most 100 objects or
1 GiB. Listing or metadata failures delete nothing; cancellation or deletion
failure stops the batch. Malformed, unmarked, legacy-layout, recent, and
non-snapshot objects are ignored. Expiry reclaims abandoned uploads, stale key
epochs, and failed rollback deletions without interpreting encrypted pointers.

The repository rejects projection payloads larger than 64 MiB and bounds encrypted
and decompressed representations separately. This is a guardrail against
restart-loop memory amplification, not a final production storage budget;
projections that outgrow it cold-replay and log the failed generation attempt.

### Each snapshot pass is elected

Snapshot generation is background work owned by one replica at a time through
the existing distributed lease mechanism backed by `MEMORY_CACHE`. Every
replica schedules checks, but each pass attempts the lease only once. The
winner refreshes the lease while building, rechecks ownership before publishing
a completed manifest, and releases the lease before waiting for the next
hourly check. Loss of the lease abandons the in-progress generation.

The lease reduces duplicate work but is not the correctness boundary.
Immutable generation bundles, current/previous fallback, and validation keep
stale workers and interrupted uploads harmless. Loaders never trust
process-local ownership state.

Initialization is best-effort as well: snapshot Object Store, repository, EVT
identity, projector configuration, and lease failures disable the affected
snapshot workers and log the reason. They do not prevent core startup when EVT
is available.

The worker checks every eligible projection immediately after boot and then
hourly. It publishes immediately after a cold replay or when startup replay
advanced beyond the restored cutoff. A fresh, unchanged restore is not
republished. Once the latest generation is 23 hours old, the worker refreshes
it even when its cutoff is unchanged, ensuring quiet projections receive a new
storage timestamp before retention expiry without turning restarts into writes.
A lower cutoff for the same EVT history and contract is rejected. A
failure for one projection is logged and does not prevent the remaining jobs
from running. On S3, the elected pass also attempts a cluster-wide daily expiry
cooldown claim in `MEMORY_CACHE`. A successful bounded expiry retains that claim
for 24 hours; a failed expiry releases it so the next hourly publication pass
can retry. Losing or failing to acquire the publication lease does not consume
the expiry cooldown. Expiry failure does not stop publication checks.

### Each projection owns its replay frontier

Each projection has its own contract ID, pointer, current/previous
generations, cutoff, and fallback behavior. Most initial codecs use `v1`; the
privacy-separated user profile codec uses `v2`.

The initial 0.5 Threads implementation uses contract ID `v1`. It does not
import pre-EVT `thread_follow.*` records from `RUNTIME_STATE`; follow state is
rebuilt only from durable `ThreadFollowedEvent` and `ThreadUnfollowedEvent`
facts.

Each eligible projection loads its snapshot independently. A successful restore
starts that projection's ordered EVT consumer at one greater than its own
cutoff. A projection with no usable snapshot starts its consumer at sequence 1
without changing any sibling's frontier. Projections with no matching EVT
history have no state to accelerate and do not publish zero-cutoff generations.
Boot-time waiters are released through the same sequence-advance path used by
live events even when they begin waiting while restore is in flight.

Every registered projection must still become current before Chatto completes
boot. A cold projection can therefore determine total startup wall time, but it
does not make restored projections reread or decode their earlier history.

`UserProjection` retains encrypted PII source fields and materializes them only
at read boundaries. Its explicit `v2` codec stores those encrypted
values, lookup digests, wrapped DEK records, and non-secret profile metadata.
Credential-bearing state is owned by the separate `UserAuthProjection`; its
schema has no snapshot representation and its focused account, password,
external-identity, consent, deletion, and key-shredding facts cold-replay on
every startup. This structural split prevents profile snapshot code from
serializing password verifiers, authentication generations, raw provider
subjects, or OAuth consent.

Every eligible codec uses an explicit protobuf and transactional restore.
`RoomTimelineProjection` persists immutable event envelopes and encrypted
`MessageBody` envelopes without decrypting content, then rebuilds indexes.
`MentionablesProjection` persists wrapped DEK records and the latest encrypted
login source event rather than its plaintext handle map. `UserProjection`
persists encrypted profile fields and rebuilds its indexes transactionally.
Shred markers and the
existing durable representation of non-secret metadata are preserved.

Snapshot persistence is disabled by default. Operators enable it with
`[core].projection_snapshots = true` or
`CHATTO_CORE_PROJECTION_SNAPSHOTS=true`. Retention defaults to seven days and
can be changed with `[core].projection_snapshot_retention`. Operators that
configure S3 lifecycle expiry disable Chatto's redundant pass with
`[core].projection_snapshot_s3_cleanup = false`.

## Consequences

- Each eligible projection can start from its own compatible snapshot cutoff
  and replay only its later EVT delta without weakening the authority or
  retention of `EVT`.
- Snapshots naturally use cheaper S3 storage where configured while preserving
  the zero-dependency NATS default. NATS snapshots follow Chatto's NATS backup;
  S3 snapshots follow the operator's S3 backup policy.
- Snapshot payloads consume additional storage, network bandwidth, and
  background CPU. Compression, daily publication, configurable retention, NATS
  TTL, and bounded S3 age expiry constrain normal and abandoned objects. The
  fixed per-pass S3 limits are operational guardrails rather than a hard storage
  budget.
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
- `UserAuthProjection` still cold-replays its focused subject families. Startup
  time therefore includes authentication replay, snapshot loading, index
  reconstruction, and each eligible projection's tail replay.

## Out of Scope

- Expiring, compacting, truncating, or archiving `EVT`.
- Depending on snapshots for backup, disaster recovery, or domain correctness.
- Snapshot migration between projection contract IDs.
- Incremental or chained snapshots.
- Persisting decrypted sensitive projection state.
- A generic snapshot format shared by all projections.
- Selecting a default-on rollout policy.
- Extending `chatto backup` to include S3-backed assets or snapshots.
