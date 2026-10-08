# FDR-049: Server Gutter

**Status:** Active
**Last reviewed:** 2026-10-08

## Overview

The Server Gutter lets users select and arrange the servers known to this
frontend. The order belongs to this browser or app installation.

## Behavior

- The server that hosts the frontend is first and has a small home badge.
  It scrolls with the list and cannot be moved. A standalone frontend has no
  home server.
- Users can drag remote server icons with a mouse. The server menu also has
  **Move up** and **Move down** actions, with unavailable moves disabled.
  Touch input uses the menu, including on devices with a mouse or trackpad.
  Taps select servers and swipes scroll the list.
- New servers appear after the existing remote servers. Signed-out and
  unavailable servers can be moved. Moving a server does not select it.
- Removing a server clears its saved position. Adding it again puts it after
  the retained servers.
- Completed moves are saved on this device. Other open tabs on the same
  frontend origin update without a reload or a request to a server.
- Drag previews stay in the tab that owns the drag. The last saved order wins
  when tabs make concurrent moves. Changes to list membership cancel a drag.
- A tab reads the saved order again when it gains focus or resumes. Separate
  browser profiles and different frontend origins keep separate orders.
- If browser storage is unavailable, users can still change the order in
  the current tab.
- The Add Server action stays at the bottom and opens the Server Directory.

## Design Decisions

### 1. Keep the home server first

**Decision:** Mark the server that hosts the frontend and keep it first.
**Why:** Its place stays clear when the user adds or moves remote servers.
**Tradeoff:** Users cannot place a remote server before it.

### 2. Share saved order through browser storage

**Decision:** Share completed moves between tabs that use the same storage.
**Why:** The catalogue stays device-local under ADR-074. No server needs to
receive the user's layout preference.
**Tradeoff:** Order does not follow the user to another device or frontend
origin. Concurrent moves use the last saved order.

### 3. Keep display policy in the frontend

**Decision:** Keep server order separate from client registrations and sessions.
**Why:** This is frontend layout policy under ADR-112. Other clients can
choose their own layout.
**Tradeoff:** Each frontend must own its display order.

## Related

- **ADRs:** [ADR-074](../adr/ADR-074-keep-server-catalogue-device-local.md),
  [ADR-112](../adr/ADR-112-keep-the-server-catalogue-in-the-frontend.md)
- **FDRs:** [FDR-023](FDR-023-authentication-and-sessions.md),
  [FDR-031](FDR-031-client-server-compatibility-discovery.md),
  [FDR-042](FDR-042-chatto-neighbors.md)
- **Issue:** [#2856](https://github.com/chattocorp/chatto/issues/2856)
