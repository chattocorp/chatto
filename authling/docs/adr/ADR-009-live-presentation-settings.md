# ADR-009: Record Presentation Settings in the Primary Event Log

**Date:** 2026-10-30

**Status:** Accepted

## Context

Administrators need to edit public display text without a server restart.
Configuration changes also need an audit history: who changed the settings,
when they changed them, and what values they saved. A current-value store
cannot provide that history.

## Decision

Append `SiteSettingsChangedEvent` to the singleton subject
`authling.evt.site-settings` in `AUTHLING_EVT`. Each event records the acting
administrator's opaque account ID, the previous settings revision, and the
complete replacement settings. The standard event envelope records event ID
and timestamp. No additional stream or KV bucket is needed.

Store public display values directly in the event protobuf, without encryption
or a key dependency. These fields contain public site copy, not private account
data or secrets. Record the acting administrator by opaque ID, never by email.
Account erasure does not erase deployment configuration or its audit history.

A required in-memory projection validates and replays every settings event.
Reads capture the subject tail and wait for the projection before returning
current values. TOML/ENV supplies defaults only before the first settings event.
Read or replay errors fail closed. OIDC protocol endpoints and static assets do
not depend on these display settings.

Save checks current administrator access, synchronizes settings state, and
checks access again before appending with the editor's original subject revision.
After a conflict, synchronize again and return a conflict without retrying the
submitted edit. After a successful append, wait for local projection application.

Administrator revocation and a settings append are not one atomic transaction.
Authorization is checked immediately before append. This command cannot change
privileges or identity state, and settings replay does not depend on mutable
account state. The audit actor remains an opaque ID after account deletion.

## Consequences

The primary log retains each saved version and its actor and time. The projection
is disposable and rebuilt on restart. No audit-history browser page is included.
Lost write acknowledgements can leave an uncertain outcome; editors must inspect
saved settings before retrying. Subject OCC prevents a stale retry from replacing
a committed change.

Existing history is unchanged; tag 119 adds the settings event. Old account,
issuer, grant, and erasure projections filter other subjects, so they do not
consume it. Older binaries still show configured text. Upgrade every replica
before editing settings. A rollback retains events but displays old defaults;
an upgrade replays saved settings again. Event export or audit consumers that
decode all subjects must support the new event before it is written.

The KV implementation was unmerged development work and is removed without a
production migration. Local buckets created by that implementation are not read
or deleted. Re-enter any local experimental settings through the editor.
