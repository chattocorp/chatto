# FDR-061: Server Overview

**Status:** Active
**Last reviewed:** 2026-11-26

## Behavior

The server Overview at `/chat/{server}/overview` is the home page of a server. A member opens it from the sidebar **Overview** entry, or lands on it when the app has no last room for the server ([FDR-026](FDR-026-last-room-memory.md)). The page shows these sections from top to bottom:

1. **Identity.** The banner, logo, name, and description of the server. A server without a banner shows the gradient of its name.
2. **Setup reminder.** Members with the server-management permission see a list of the empty public profile fields: logo, banner, description, and welcome message. The reminder links to the General settings. It goes away when the profile is complete.
3. **Shield Mode notice.** Members who hold protected roles but have not activated Shield Mode see the same notice as on the Settings Overview: the shield button beside their name activates their additional permissions.
4. **Welcome and message of the day.** The welcome message and the message of the day, rendered with the message renderer, so links to Chatto rooms, servers, and messages show as pills. The message of the day also stays in the application header, with the same pills. The header shows only its first line; the dialog that opens from it shows the complete message. Administrators edit both messages in the General settings with the composer editor that they selected for messages, which shows the same pills.
5. **Happening now.** One panel with a row for each active call in a joined room, with its participants and a **Join** action. The action opens the room with its call panel. A **Catch up** card lists up to five joined channel rooms with unread messages or notifications. Rooms with important notifications come first. The room name opens the room. Each notification badge opens the newest notification that it counts, as the sidebar badges do. Direct messages have their own card: the five most recent conversations with messages, with the same unread markers and notification badges.
6. **Rooms.** The room directory, with search and a membership filter: **All**, **Joined**, or **Not joined**. **All** is the default. Rooms created in the last seven days that the member has not joined show a **New** label; a joined room is already known. Each room group is a section, named with the same `section-label` style as the sidebar's room groups, with a grid of room `tile`s and a small **Join all** action. A joined room's tile opens the room. At rest it shows only a check mark, named **Joined** for assistive technology; it turns into **Leave** on hover or focus.
7. **Neighbors.** The Neighbors that the server advertises ([FDR-042](FDR-042-chatto-neighbors.md)). A card opens the server when it is already in the app. Otherwise it opens the Server Directory, which owns the join flow.

On wide screens, the welcome message and the rooms form the main column. The message of the day, the calls, and **Catch up** form a narrow side column. On narrow screens, the side column moves between the welcome message and the rooms. The page hides each section that has nothing to show, except the identity and the rooms. Without side-column content, the main column takes the full width. A member can hide the welcome message, and an administrator can hide the setup reminder. Each choice applies to one server on the current device.

## Rationale

The Overview was only a room directory. New members land on it, so it is the best place to introduce the server and to show where activity is. All of its data is already in the server projection or in public server data. The page does not need a new API, so it works with every connectable server version.

The membership filter replaces a split into separate "Your rooms" and "Discover" tabs. With tabs, a room would move out of view when the member joins or leaves it. With the filter, rows stay in place in the default **All** view.

The side column keeps short, current items next to the long welcome message and the room directory, so no row is left half empty. The welcome message and setup reminder can be hidden because they repeat on every visit. The device stores only a flag for each server, not the content (ADR-107). Members cannot hide the message of the day, because administrators use it for current information.

The Neighbors section reads the cached Neighborhood from the current server. The logos and banners are copies on that server, so the browser does not contact the Neighbors.

The page shows no online-member count. The API has no server-wide presence count, and a count from a universal room is not available on every server.

## Implementation

- `apps/frontend/src/routes/chat/[serverId]/overview/+page.svelte` composes the sections from `$lib/components/overview/`.
- `$lib/serverOverview` selects the missing profile fields and the catch-up rooms.
- `$lib/storage/overviewDismissals` stores the hidden cards per server.
- `RoomDirectory` owns the membership filter.
