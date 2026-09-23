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
paint for the selected server. Other registered servers start discovery when
the user opens them. The saved viewer remains display data and cannot authorize
server actions or a realtime connection. A successful viewer check releases this
startup gate only for the same user. An account change clears the old private
view through the existing session replacement boundary.

The installed app opens the origin chat route so an offline launch can use
that saved view and the last room saved on the device.

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
