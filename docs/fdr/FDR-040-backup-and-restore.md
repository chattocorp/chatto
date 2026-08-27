# FDR-040: Backup and Restore

**Status:** Active
**Last reviewed:** 2026-08-27

## Overview

Operators can copy Chatto's NATS JetStream data into one archive and restore
that archive into a stopped Chatto deployment. A backup can include the built-in
encryption keys and can use passphrase encryption. External S3 data needs a
separate backup policy.

## Behavior

- `chatto backup` writes eligible persistent JetStream streams, KV buckets,
  and Object Stores to one compressed archive.
- The archive includes `EVT`, `RUNTIME_STATE`, notification history,
  NATS-backed assets, and NATS-backed projection snapshots.
- Regeneratable caches, user presence, and S3-backed objects are not included.
- Encryption keys are not included by default. `--include-keys` makes a backup
  self-contained for the built-in KMS.
- `--encrypt` protects the complete archive with an age passphrase. Automated
  commands can read the passphrase from a restricted file or from explicitly
  selected standard input. Chatto does not accept a passphrase argument.
- Backup writes use private staging data and an atomic final move. A failed
  backup does not replace an existing archive with a partial file.
- `chatto restore` requires Chatto to be stopped. Embedded-NATS restore starts
  a temporary local server. External-NATS restore connects to the configured
  NATS service while the Chatto application remains stopped.
- Restore rejects unsafe paths and archives that exceed fixed entry-count,
  compressed-file, or expanded-size limits.
- Restore conflict modes can fail on existing resources, skip them, or replace
  them. Replacement is an explicit destructive operator choice.
- Keeping the same core secret preserves compatible runtime credentials and
  pending workflows. Changing it intentionally invalidates those records.

## Design Decisions

### 1. Back up the NATS storage boundary as one unit

**Decision:** One data archive contains the persistent NATS streams, KV
buckets, and Object Stores that are not excluded for security or regeneration.
**Why:** The event log, runtime records, notifications, and NATS-backed assets
form one restore boundary. A resource-by-resource operator procedure is easier
to run incorrectly.
**Tradeoff:** Large installations can produce large archives and need enough
temporary disk capacity for staging.

### 2. Keep external object storage outside the archive

**Decision:** S3-backed assets and projection-snapshot generations use the
operator's S3 backup policy instead of being copied into the NATS archive.
**Why:** These objects can be much larger than NATS state and already have an
independent storage lifecycle. Projection snapshots are disposable; assets are
not.
**Tradeoff:** An S3 deployment needs a coordinated data and object-storage
restore procedure.

### 3. Exclude key-encryption keys by default

**Decision:** Normal data backups exclude built-in KMS key-encryption records.
Operators can include them explicitly and should encrypt that archive.
**Why:** A data archive without the key-encryption keys cannot decrypt its own
protected message bodies and user data. Explicit inclusion makes the security
tradeoff visible.
**Tradeoff:** A backup without keys needs the matching separate key export.
Losing either part makes protected data unavailable.

### 4. Make passphrase handling explicit

**Decision:** Encrypted backup, restore, and key-export commands accept an
interactive prompt, a restricted passphrase file, or explicitly selected
standard input. They do not accept raw passphrases in command arguments.
**Why:** Command arguments can appear in shell history and process listings.
Explicit input modes also prevent an inherited pipe from becoming an
accidental secret source or blocking a command.
**Tradeoff:** Automated jobs need a secret file or an explicit pipe from a
secret manager.

### 5. Restore is bounded and operator-controlled

**Decision:** Restore validates the archive before resource replacement,
applies fixed resource limits, and requires an explicit conflict policy.
**Why:** Restore consumes untrusted file structure and can replace durable
state. Safe defaults must reject ambiguity and unreasonable expansion.
**Tradeoff:** Very large valid installations can eventually need higher
product limits rather than an unbounded override.

## Related

- **ADRs:** ADR-007 (per-user encryption and crypto-shredding), ADR-021 (dual
  asset storage), ADR-036 (runtime state), ADR-050 (projection snapshots)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-016 (Voice
  Calls), FDR-018 (Account Lifecycle), FDR-033 (Message Search)
