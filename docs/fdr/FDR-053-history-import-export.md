# FDR-053: History Import and Export

**Status:** Experimental
**Last reviewed:** 2026-10-26

## Overview

Operators will import and export channel history using one documented format.
Converters for Slack and other platforms will produce that format. The first
implementation provides a draft specification, schemas, examples, conformance
tests, and a native importer with durable recovery. Export remains planned.
The Slack converter produces this format, including local original file bytes.
It preserves source account states and declares unavailable data. Conversion
does not require a Chatto server; destination validation remains a separate step.
The [schema guide](../../schema/interchange/README.md) lists the supported
subset and implementation limits.

## Behavior

- `chatto operator import PATH` validates and imports a directory into a
  destination without conversation history through the configured operator socket. Repeating the same
  immutable import will resume it; no separate plan or mapping file is required.
- `--validate-only` checks the archive and destination over the local operator
  socket without writes. Invalid references,
  invalid history, missing files, and file integrity failures stop validation.
- `chatto operator export PATH` will produce a readable archive at a consistent
  source cut and refuse an existing output destination.
- Channel and username prefixes and suffixes are optional. Empty defaults
  preserve source names. Naming options are bound to durable import progress;
  changed options cannot resume the same import.
- `--accept-omissions` is required for writes when the archive declares losses.
- Room topic becomes the displayed description. Room purpose is stored
  separately for future export; the current frontend does not edit it.
- Known joins, leaves, room changes, posts, edits, deletions, threads, and
  reactions will retain their history. Unknown times will remain unknown.
- Final snapshots will preserve known state without inventing timeline events.
  The final membership set will not produce simultaneous historical join notices.
- Deleted content will stay deleted, including old versions of that content.
  Reactions and surviving replies may remain on tombstones.
- Missing files and other losses will be explicit. Import will not silently
  rename users, merge accounts, make private rooms public, or discard records.
- Import will not send historical notifications, invitations, or webhooks.
- The operator importer trusts source emails. It matches existing accounts by
  trimmed, case-insensitive email and preserves their profiles and credentials.
  Unowned emails become verified addresses; shared source emails resolve to one
  account. Conflicting existing owners stop validation before writes. The first
  source record supplies the profile of a new account. Names alone never match
  accounts, and naming decorations apply only to new accounts.
- Archives will contain readable personal data. They will not contain passwords,
  sessions, source download credentials, or server encryption keys.

## Design Decisions

### 1. Share a portable contract across platforms

**Decision:** Platform converters produce a common archive; Chatto owns import.
**Why:** Identity mapping, attachments, and recovery should be implemented once.
See [ADR-119](../adr/ADR-119-portable-history-interchange.md).
**Tradeoff:** The format needs its own versioning and conformance tests.

### 2. Preserve history without inventing missing events

**Decision:** Accept historical events and explicitly final state snapshots.
**Why:** Sources often omit edit history, reaction times, or membership times.
**Tradeoff:** Consumers must distinguish observations from historical transitions.

### 3. Start with a blank destination

**Decision:** Initial import does not merge conversation histories. Existing
accounts are reused only through email matching.
**Why:** Email ownership establishes account identity. Automatic reconciliation
of room histories and access policies can expose private history.
**Tradeoff:** Operators cannot use this as an incremental synchronization tool.

## Gates

The commands use the existing root-equivalent local operator access.
They do not grant ordinary users server-wide export privileges. The first
implementation validates destination compatibility before writing any records.
The initial destination can contain existing accounts and empty channels,
but no conversation history. Trusted imported emails can enable configured email
sign-in; no passwords are created. Private rooms remain unsupported.

Progress is saved with each operation. Recovery does not depend on the normal
sending retry window or client memory. Unrelated server changes stop an
unfinished import.
Operators must keep the destination unused until completion. No automatic
rollback is attempted. Files use stable destination keys so retries reuse the
same bytes after a storage acknowledgement is lost.

## Related

- **ADRs:** [ADR-119](../adr/ADR-119-portable-history-interchange.md), ADR-042
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion),
  FDR-005 (Reactions), FDR-018 (Account Lifecycle), FDR-019 (Room Lifecycle),
  FDR-028 (Operator API & CLI), FDR-040 (Backup and Restore)

## Open Questions

- Extend the first import subset to private channels, other user
  states, unavailable content/files, and unknown message creation times.
- Stream large archives and support shared file references and media derivatives.
- Specify consistent export cuts across history, catalogs, content keys, and
  attachment storage, and how interrupted output is identified and removed.
- Freeze the Markdown dialect and decide structured mention support before v1
  release. Channel-only scope does not promise complete Chatto server export.
