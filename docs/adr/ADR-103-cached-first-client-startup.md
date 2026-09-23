# ADR-103: Open a Saved Chat View Before Client Connection

**Date:** 2026-09-23
**Status:** Accepted

## Context

The frontend has a bounded saved chat view and a versioned offline shell.
Before this decision, route loading waited for server discovery and viewer
checks before it read the saved view. The root service worker also waited for
a navigation request to fail before it served its cached document. A slow
server could therefore delay useful content that was already on the device.

## Decision

On an initial chat route with a saved view for the registered server and user,
the client creates its stores without network work and restores that view into
the normal chat layout. It starts discovery and viewer checks after the first
paint for the selected server. Other remote servers start discovery when
the user opens them. The saved viewer remains display data. While its viewer
check is pending, the connection holds private reads and rejects server
actions. It allows the viewer check to proceed. A successful check releases
the held reads and realtime startup only for the same user. The response guard
records the private data generation after the held read is released. This
prevents viewer verification from rejecting a valid response. An account change
clears the old private view through the existing session replacement boundary.
On chat-wide pages without a selected saved view, the client starts the origin
server with the public shell requests. These pages use its live projections.
Unopened remote servers remain dormant.
After origin sign-out, a dormant remote bearer session can be selected for
navigation. Its viewer check starts when that route opens.

Message permalinks use live startup because their target can be outside the
bounded saved window. The client resolves that target after the live viewer
and room state are ready.

The installed app opens the origin chat route so an offline launch can use
that saved view. It opens the last room saved on the device, or the overview
when no last room exists.

The service worker serves its complete, versioned shell document first on a
later app navigation. It fetches the document from the network when the cache
is absent. API, authentication, realtime, and protected assets remain on the
network. No realtime cursor or query result is saved with the view.

## Consequences

- A repeat launch can show saved rooms and text before the server responds.
- The view remains read-only until the server confirms the session and current
  permissions. A failed check does not turn saved identity into authority.
- Users can see an older installed frontend version until a service worker
  update finishes. The worker installs a complete new shell before activation.
- A first visit, an expired view, or unavailable device storage still uses the
  normal network-led startup path.
