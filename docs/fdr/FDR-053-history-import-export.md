# FDR-053: History Import and Export

**Status:** Experimental
**Last reviewed:** 2026-10-30

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

- `chatto operator import PATH` reads a local directory and calls individual
  resource APIs through the configured operator socket. It creates new channels.
  Repeating the same immutable import with its local journal resumes it.
- `--validate-only` checks the archive and destination over the local operator
  socket without server writes. Invalid references,
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
- User records can reference avatar assets. Imports fill empty avatars and
  preserve existing ones under user-aggregate concurrency checks. Both assignment
  and preservation commit permanent receipts; retries cannot restore an image
  cleared after import. Images use the normal avatar processing pipeline.
- New accounts can be humans or bots, active or deactivated. Creation and initial
  deactivation commit atomically with the operation receipt. Imported bots receive
  no credentials and belong to the current active human server owner by default.
  `--bot-owner` can select another active human and is required when multiple
  owners exist. The selected owner ID is saved before creation for stable retries.
  Bots with no source username receive a deterministic `bot-` name derived from
  the source ID. Deleted accounts remain unsupported.
- The operator importer trusts source emails. It matches existing accounts by
  trimmed, case-insensitive email and preserves their profiles, credentials, and
  activation status. Bot accounts cannot have email claims. Source records sharing
  email identities must have the same kind and status.
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

### 3. Create new channels

**Decision:** Initial import does not merge conversation histories. Existing
accounts are reused only through email matching.
**Why:** Email ownership establishes account identity. Automatic reconciliation
of room histories and access policies can expose private history.
**Tradeoff:** Operators cannot use this as an incremental synchronization tool.

## Gates

The commands use root-equivalent local operator access. They do not grant
ordinary users server-wide export privileges. Source structure, references,
file hashes, supported record types, and email conflicts are checked before
writes. Server policy and media validation still apply to each API call and can
stop a partial import. Trusted imported emails can enable configured email
sign-in; no passwords are created. Private rooms remain unsupported.

Progress is saved in a private local journal under the user cache directory.
Join/leave records can carry final reaction snapshots. The CLI retains the
event ID returned by `ImportRoomEvent` and calls `ImportReaction` for each known
reactor. These calls preserve the membership event as their target and do not
invent source reaction times. Repeated calls do not create duplicate reactions.
Room-event receipts now retain the event ID. Older receipts contain only a room
ID and return an empty event ID; the CLI stops if reactions need that missing
target, without repeating the membership transition. Older servers remain
usable for archives without membership reactions. UI display is separate.

`--progress PATH` selects another file. Preserve it and the immutable archive
for retries. Operation receipts on the server prevent duplicate mutations even
when a successful response is lost. Recovery does not use the sending retry
window. The CLI holds an exclusive journal lock. It does not copy source files
into that journal. Replayable history results share disk checkpoints to reduce
import overhead. After interruption, the CLI can repeat recent requests with
the same operation keys and recover their results without duplicate events.
Identity and upload prerequisites remain durable before dependent requests;
successful completion makes all progress durable.
Adjacent message reaction transitions can share one atomic request. Invalid
input commits none of the new transitions in that request. Exact retries skip
previously committed transitions, including those imported through individual
calls. The CLI preserves source order and uses individual calls when the server
does not support batches. This does not impose an archive size limit.
New messages without files can share a request with their reactions. Replies
can refer to earlier messages in that request by operation key. Forward
references and cycles are invalid. Invalid input commits no new message facts;
new author encryption keys can remain after a later preparation failure. Thread
declarations, tombstones, and reactions preserve their source order. Files,
edits, and room transitions end the current group. On older servers, the CLI
uses individual calls with the same operation keys and resolved references.
Keep imported channels unused until completion; the importer
does not merge concurrent changes or automatically roll back partial work.

Files use chunked uploads when first referenced. Shared source files are
uploaded separately for each message, as required by native asset ownership.
For JetStream object storage, explicit ingest-queue rejections (429/10167)
are retried up to five times with exponential delays. The server rewinds the
input and retains the object key before it commits an upload offset or asset
fact. Other errors and non-seekable inputs are not retried by this mechanism.
Upload completion is retryable. Incomplete uploads can be restarted; normal
server upload expiry and per-file policies still apply. There is no total
archive size or record-count limit. One JSON record is limited to 1 MiB.

## Related

- **ADRs:** [ADR-119](../adr/ADR-119-portable-history-interchange.md), ADR-042
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion),
  FDR-005 (Reactions), FDR-018 (Account Lifecycle), FDR-019 (Room Lifecycle),
  FDR-028 (Operator API & CLI), FDR-040 (Backup and Restore)

## Open Questions

- Extend the first import subset to private channels, other user
  states, unavailable content/files, and unknown message creation times.
- Optimize reference-index memory use for very large archives.
- Specify consistent export cuts across history, catalogs, content keys, and
  attachment storage, and how interrupted output is identified and removed.
- Freeze the Markdown dialect and decide structured mention support before v1
  release. Channel-only scope does not promise complete Chatto server export.
