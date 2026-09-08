# ADR-098: Retain Operational Diagnostics in LOG

**Date:** 2026-09-08
**Status:** Accepted

## Context

Repeated webhook failures need shared diagnostic history with limited retention.
They do not change domain state. Permanent EVT history is not appropriate for
these records. A process-local history cannot serve requests across replicas.

## Decision

Store operational records in the file-backed JetStream `LOG` stream on `log.>`.
Use `LimitsPolicy` with seven-day `MaxAge` by default. Operators can set
`core.log.retention` or `CHATTO_CORE_LOG_RETENTION`. Do not set count or byte limits.
Age retention does not impose a fixed storage ceiling during a burst.

Put the internal protobuf envelope and typed payloads in `chatto.core.log.v1`.
The envelope has a stable ID, recording time, severity, and payload oneof.
Producers supply fixed safe categories and opaque resource IDs. Do not store
credentials, raw errors, response bodies, message bodies, or personal data.
The first payload is a terminal outbound webhook failure.

Use one subject per terminal delivery, under its bot and endpoint scope.
Expected-last-subject sequence zero suppresses duplicate appends while the
record exists. Retention ends this suppression. LOG is diagnostic history,
not a job queue, recovery record, or permanent execution ledger.

Read records directly from JetStream. The bot manager API returns complete
failure records in recording order, with bounded page sizes and a captured
upper boundary. Encrypted cursors bind the viewer, endpoint, and stream
creation identity. Expiry can remove records between pages. No log projection,
KV index, or durable read consumer is needed.

Keep webhook scheduling and retries unchanged. A bounded log read can suppress
an already recorded terminal delivery. A read failure does not prevent HTTP.
A failed log append produces a safe server log and ends the recording attempt;
it never retries HTTP. Source-time expiry still prevents old messages from
sending HTTP after log retention ends.

New failures do not enter EVT. Keep the historical EVT protobuf variant and
subject mapping readable, but do not copy old failures to LOG. This supersedes
the failure-storage decision in ADR-097. Exclude LOG from backups because it is
not recovery state. Restored servers start with empty diagnostic history.
An upgrade can retry a still-live delivery whose terminal failure exists only
in historical EVT. Receivers must continue to tolerate duplicate delivery IDs.

## Consequences

All replicas read the same retained history. Expired failures disappear from
the latest summary and future history queries. An empty history does not prove
successful delivery. The history view refreshes periodically while open.

LOG loss cannot be repaired from EVT. A process that stops before recording a
failure leaves no diagnostic record. Cross-replica duplicate suppression ends
when a record expires. Additional payload types require an explicit schema and
subject mapping. Per-category retention and an admin-wide log view are deferred.
