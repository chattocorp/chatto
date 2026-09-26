# ADR-107: Keep Chat Data Out of Device Storage

**Date:** 2026-09-26
**Status:** Accepted

## Context

[ADR-103](ADR-103-cached-first-client-startup.md) and
[ADR-104](ADR-104-checkpointed-client-projection-snapshots.md) stored a copy of
each server's client projection in IndexedDB. The copy contained room
resources, public profiles, timeline windows, member lists, notification
state, and a realtime replay checkpoint. On a cold load, the client showed the
copy before the server verified the viewer.

This design had these costs:

- Private chat data stayed on the device. Each privacy boundary needed a disk
  purge. Stale tabs needed write generations, persisted invalidation cutoffs,
  and clock-order protection so that they could not write purged data again.
- The client serialized the complete server projection 100 ms after most
  changes. On busy servers, this used main-thread time during normal use.
- The saved view was display data without authority. Route loads, the
  connection, management pages, account forms, and recovery needed separate
  gates for the time before viewer verification.
- 0.5 beta users reported slow room switches that came from repeated snapshot
  reads.

The benefits were a faster first paint on reload and offline reading of loaded
messages. No stable release included the feature.

## Decision

The client does not store chat data on the device. Server projections,
timelines, member lists, notification state, and the realtime resume cursor
exist only in memory. Each page load starts without a cursor and receives a
fresh snapshot from the server.

Device storage keeps the server catalogue, authentication records, and UI
preferences such as the last room and pane widths. It keeps no messages,
member lists, profiles, or notification content.

Route loads wait for discovery and viewer checks. Registry start begins
discovery and viewer checks for each registered server. Remote servers are
not held dormant until the user opens them.

A reconnect without a page load resumes from the in-memory cursor and keeps
the mounted view. [ADR-091](ADR-091-semantic-realtime-events-with-bounded-resume.md)
defines that behavior.

The service worker keeps its complete, versioned application shell. ADR-103
defines that shell. The shell contains no private data.

Cross-tab messages for sign-out, account changes, and server removal remain.
They clear in-memory private data in other tabs.

On each page load, the client deletes the `chatto-saved-views` IndexedDB
database that 0.5 beta clients created. A tab that runs an older client can
create the database again, so the deletion runs on every load. Remove this
cleanup after a later release.

This decision supersedes ADR-104. It supersedes ADR-103, except for the
service worker shell.

## Consequences

- A reload or cold launch shows loading states until the server responds. An
  offline launch shows no chat content.
- Privacy boundaries clear memory only. Disk purges, invalidation cutoffs, and
  the private-request hold are no longer necessary.
- No snapshot capture runs during normal use.
- Each registered server receives discovery and viewer requests at startup,
  as it did before ADR-103. This reveals the user's IP address to each
  registered server when the app starts.
