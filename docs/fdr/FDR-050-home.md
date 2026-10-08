# FDR-050: Home

**Status:** Experimental
**Last reviewed:** 2026-10-09

## Overview

Home is one page that shows what needs the user on all signed-in servers. It
is at `/chat/home`. The Home button at the top of the Server Gutter opens it.
The Quick Switcher also contains it.

## Behavior

- Home shows three sections:
  - **Calls in progress:** calls in direct messages and in rooms that the user
    is a member of. Each row shows the room, the number of participants, and
    their avatars. The section appears only when a call is in progress.
  - **Direct messages:** the visible direct messages of all servers. Direct
    messages with Important notifications come first, then direct messages
    with other notifications, then direct messages with unread messages.
  - **Notifications:** the same feed as the Notifications page, with the same
    open, delete, and dismiss actions. See [FDR-012](FDR-012-notifications.md).
- Calls and direct messages come first, in a side column on wide screens and
  above the notifications on narrow screens. The reading order and the visual
  order are the same.
- The direct-message list shows a loading state only until one server can list
  its rooms. An unreachable server does not keep the list loading.
- Each row names its server when the user is signed in to more than one server.
- A row opens its room on its server.
- Home uses only data that the client already holds for each server. It does
  not send more requests to servers than the Notifications page.
- `/chat` continues to open the origin server. Home is not the landing page.

## Design Decisions

### 1. Combine existing per-server data in the frontend

**Decision:** Build Home from each server's projection and notification store.
**Why:** All the data exists in the client. A bot works on one server, so the
cross-server view is frontend policy, not `@chatto/client` behavior (ADR-111).
**Tradeoff:** Home can show only what the client has loaded. Rooms do not have
an activity time, so direct messages without attention keep the room order of
their server.

### 2. Share the notification feed with the Notifications page

**Decision:** Render one notification feed component on both pages.
**Why:** The two pages cannot diverge in grouping, read state, or actions.
**Tradeoff:** Changes to the feed affect both pages.

## Related

- **ADRs:** [ADR-111](../adr/ADR-111-move-client-state-into-chatto-client.md)
- **FDRs:** [FDR-012](FDR-012-notifications.md),
  [FDR-015](FDR-015-quick-switcher.md), [FDR-049](FDR-049-server-gutter.md)
