# FDR-049: Server Gutter

**Status:** Active
**Last reviewed:** 2026-11-13

## Overview

The Server Gutter lets users select and arrange the servers known to this
frontend. The order belongs to this browser or app installation.

## Behavior

- All servers share one list and can be moved, including the server that
  hosts the frontend. There is no home badge.
- Users can drag server icons with a mouse. The server menu also has
  **Move up** and **Move down** actions, with unavailable moves disabled.
  Touch input uses the menu, including on devices with a mouse or trackpad.
  Taps select servers and swipes scroll the list.
- New servers appear after the existing servers. Signed-out and
  unavailable servers can be moved. Moving a server does not select it.
- Removing a server clears its saved position. Adding it again puts it after
  the retained servers.
- Completed moves are saved on this device. Other open tabs on the same
  frontend origin update without a reload or a request to a server.
- Existing saved layouts keep their positions until the user makes a move.
- Drag previews stay in the tab that owns the drag. The last saved order wins
  when tabs make concurrent moves. Changes to list membership cancel a drag.
- A tab reads the saved order again when it gains focus or resumes. Separate
  browser profiles and different frontend origins keep separate orders.
- If browser storage is unavailable, users can still change the order in
  the current tab.
- The Add Server action stays at the bottom and opens the Server Directory.
- A Chatto server shows the gutter in its bundled frontend only when the
  operator sets `frontend.gutter.enabled` to `true`. The default is
  `false`. Without the gutter, the frontend shows only its own server, and
  `/chat/servers` redirects to `/`. The standalone frontend and the desktop
  app always show the gutter.

## Design Decisions

### 1. Use the same ordering rules for every server

**Decision:** Let users move every server and omit the home badge.
**Why:** The server that hosts the frontend is not a special navigation item.
The same controls should apply to every server in the list.
**Tradeoff:** Users choose how to identify and position their servers.

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

### 4. Hide the gutter in the bundled frontend by default

**Decision:** The server writes a `chatto-gutter` meta element into the
bundled frontend document. Its value comes from
`frontend.gutter.enabled`, which is `false` by default. When the meta
element is missing, the frontend shows the gutter.
**Why:** Most members of a server use only that server. Operators can turn on
the gutter when they want it. The standalone frontend and the
desktop app exist to connect to many servers, so they keep it.
**Tradeoff:** Servers that a user added earlier stay in device storage, but
the bundled frontend shows no control to switch to them while the setting is
off. Server actions, including Sign out, stay available in the Server Menu of
the Server Sidebar header and in the settings navigation.

## Related

- **ADRs:** [ADR-074](../adr/ADR-074-keep-server-catalogue-device-local.md),
  [ADR-112](../adr/ADR-112-keep-the-server-catalogue-in-the-frontend.md)
- **FDRs:** [FDR-023](FDR-023-authentication-and-sessions.md),
  [FDR-031](FDR-031-client-server-compatibility-discovery.md),
  [FDR-042](FDR-042-chatto-neighbors.md)
- **Issue:** [#2856](https://github.com/chattocorp/chatto/issues/2856)
