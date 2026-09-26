# ADR-103: Open a Saved Chat View Before Client Connection

**Date:** 2026-09-23
**Status:** Partially superseded by [ADR-107](ADR-107-keep-chat-data-out-of-device-storage.md)

## Context

The frontend has a bounded saved chat view and a versioned offline shell.
Before this decision, route loading waited for server discovery and viewer
checks before it read the saved view. The root service worker also waited for
a navigation request to fail before it served its cached document. A slow
server could therefore delay useful content that was already on the device.

## Decision

ADR-107 removes the saved chat view. Only the service worker shell decision
below remains in effect. The rest of this record is historical.

Before ADR-107, ADR-104 replaced the storage limits and cursor-free format
described below with versioned resource snapshots and a shared, atomic replay
checkpoint.

On an initial chat route with a saved view for the registered server and user,
the client creates its stores without network work and restores that view into
the normal chat layout. It starts discovery and viewer checks after the first
paint for the selected server. Other remote servers start discovery when
the user opens them. The saved viewer remains display data. While its viewer
check is pending, the connection holds private reads and rejects server
actions. It allows the viewer check to proceed. A successful check releases
the held reads, commands, and realtime startup only for the same user. Commands
do not wait for the replacement snapshot or its resource reads.
The response guard
records the private data generation after the held read is released. This
prevents viewer verification from rejecting a valid response. An account change
clears the old private view through the existing session replacement boundary.
On chat-wide pages without a selected saved view, the client starts the origin
server with the public shell requests. These pages use its live projections.
Unopened remote servers remain dormant.

The UI reads the normal stores. A versioned IndexedDB record stores their
presentation state: room resources and groups, known public profiles, viewer
display preferences, notification state, loaded member lists, and complete
timeline rows for a bounded recent window. Restoration populates those same
stores. There is no separate saved-room selector or text-only message renderer.
Reconnect replaces or updates the state through the existing reducers.
An unchanged resource must keep the same presentation. Member refreshes keep
the displayed list until all replacement pages arrive.

The snapshot has no credentials, verified account state, or realtime cursor.
Cached viewer data does not populate the account-loading owner. API commands
remain blocked at the connection boundary while that snapshot is unverified.
The cache keeps at most 50 timeline rows for each of 10 recent rooms, with a
shared 8 MB limit and seven-day expiry. Invalid records and the previous
text-only format use normal live startup. Live cursor advances schedule a new
save. A verified room-access loss, message edit or retraction, attachment
deletion, or account deletion clears the disk snapshot. This also removes
copied profile references. A later verified save can capture the remaining state.
After origin sign-out, a dormant remote bearer session can be selected for
navigation. Its viewer check starts when that route opens.

A session that needs reauthentication does not restore its saved view, because
the server rejected that viewer. The client deletes the view and uses live
startup. If the origin rejects the viewer and no loaded data remains, the client
opens sign-in and returns to the current page afterwards. The reauthentication
notice remains for loaded data that the viewer can still read.

Every server route starts from the saved view. The client shows a message
permalink target from the saved window when the window contains it. Other
targets load through private requests, which wait until the viewer is verified.
Account settings, management forms, and the direct-message opener also wait for
the verified viewer or live permissions.

The installed app opens the origin chat route so an offline launch can use
that saved view. It opens the last room saved on the device, or the overview
when no last room exists.

The service worker serves its complete, versioned shell document first on a
later app navigation. It fetches the document from the network when the cache
is absent. API, authentication, realtime, and protected assets remain on the
network. No realtime cursor or query result is saved with the view.

## Consequences

- A repeat launch can show room groups, message badges, and member lists before
  the server responds. Attachment bytes still require the network.
- The view remains read-only until the server confirms the session and current
  permissions. A failed check does not turn saved identity into authority.
- Users can see an older installed frontend version until a service worker
  update finishes. The worker installs a complete new shell before activation.
- A first visit, an expired view, or unavailable device storage still uses the
  normal network-led startup path.
