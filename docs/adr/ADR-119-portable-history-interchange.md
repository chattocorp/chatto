# ADR-119: Portable History Interchange

**Date:** 2026-10-26

## Context

The Slack converter currently calls per-record operator APIs. It owns identity
mapping, retry journals, attachments, and historical behavior. Every additional
platform would need similar destination-specific logic. Operators also need a
readable export that another application can consume.

Internal EVT is not a portable archive contract. Message bodies are encrypted,
identity and body events are separate, and storage events include implementation
details. Source platforms often export only current bodies or final membership,
not the complete sequence of transitions. Treating these observations as dated
events would fabricate history.

## Decision

Define an independent, versioned JSON interchange format. Its structural
authority is JSON Schema Draft 2020-12. A written specification defines ordering,
reference integrity, final-state rules, and compatibility. Synthetic archives
and conformance tests accompany both. V1 remains a draft until runtime import,
export, recovery, and semantic round-trip tests validate it.

The format uses catalogs and an ordered event log. Preserve known joins, leaves,
posts, edits, deletions, reactions, and room changes as historical events. Use
explicit final snapshots when source history is incomplete. Snapshots do not
invent timestamps or create timeline activity. Final membership snapshots are
authoritative for access; final room catalog metadata is authoritative for the
room's current properties.

Use portable IDs and local asset bytes. Converters interpret source data and
produce this format without calling Chatto. The generic importer owns validation,
mapping, durable resume, and production of valid internal events. Do not append
raw archive records to EVT. Do not reuse sending's bounded collision window as
the complete import recovery mechanism.

This file contract is separate from the protobuf-first network APIs in
[ADR-042](ADR-042-protobuf-first-public-api.md). It does not change EVT's
protobuf storage contract. Reuse domain concepts and suitable value types,
not the internal event union. Backup/restore remains the storage-and-key recovery
mechanism; interchange transfers supported readable content and history.

The first importer records `ImportProgressedEvent` in
`evt.import.server.import_progressed`, in the same atomic batch as each operation.
Receipts bind archive bytes and naming options, retain allocated identities, and
have no TTL. A whole-EVT guard prevents concurrent writers from applying stale
import steps. This first version requires exclusive destination use until
completion and binds one archive to the destination. Original file writes use
stable keys; a retry after a storage failure writes the same bytes to that key.

Native facts carry `ImportContext` to suppress historical live delivery and
notification effects through replay. Its hidden-timeline marker distinguishes
final reconciliation from known historical transitions. Unknown source times
remain marked unknown even when native envelopes require an internal time.
Room purpose is an additive persisted field separate from the displayed topic.

## Consequences

New platform converters share one destination contract. Users can inspect their
exports without a Chatto server. Incomplete sources do not require fake history.
Files remain local and credentials do not enter the archive.

Chatto must maintain a second, deliberately portable schema and mapping layer.
Schema validation alone cannot verify references, destination support, or
consistent source cuts. Runtime implementation must add those checks, durable
crash recovery, and historical side-effect policy. Export must preserve deletion
privacy rather than reveal old encrypted-body revisions as plaintext.

V1 initially covers channels and requires a destination without messages.
Existing accounts are matched by normalized email under the same whole-EVT
guard as import progress. Account creation, encrypted email claims, and the
mapping receipt commit atomically. Source identities sharing emails resolve to
one account; claims spanning distinct existing accounts are rejected. Existing
account profiles and credentials are preserved. The operator importer treats
source email claims as trusted verified addresses. General history merging,
incremental exports, DMs, permissions, and other unsupported data require future
design. Known omissions must be explicit. Released format versions must have
stable schemas and semantics, independent of internal storage refactors.
