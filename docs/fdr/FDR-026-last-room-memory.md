# FDR-026: Last-Room Memory

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

The frontend remembers, per connected server, the last channel room the user was in. When the user re-enters a server — by clicking its icon in the sidebar, by landing on the app after sign-in, or by hitting any link that points at the server root — they're taken back to that room instead of an intermediate landing page. The mechanic is local to the device and lightweight; if no last room is known, the user falls through to the server's Overview page.

## Behavior

- Visiting a channel room records it as the server's last room. The record is per-server, not global.
- Visiting `/chat/{server}` (the server root) redirects to the recorded last room. If none is stored, it falls through to the server's Overview page at `/chat/{server}/overview`.
- Visiting `/` (the app root) after sign-in redirects to the home server's last room when one exists. Otherwise it lands on the home server's root, which itself falls through to Overview.
- The Overview page is reachable directly at `/chat/{server}/overview` — typing that URL, or clicking the sidebar "Overview" entry, never bounces to the last room.
- DM rooms are deliberately not recorded. Only channel rooms qualify as a "last room".
- The record is cleared when:
  - The room becomes inaccessible (deleted, the user lost access, or the server says it can't be loaded). The user is redirected back to the server root, which then falls through to Overview.
  - The server is removed from the client (disconnect or sign out).
  - The user no longer has access to the server itself (validation fails).
- Each connected server has its own record. Switching servers in a multi-server setup lands on that server's own last room.
- The Cmd-K Quick Switcher's "Server" entry navigates to Overview, not to the last room. (See FDR-015.)

## Design Decisions

### 1. Per-server, not global

**Decision:** The last-room record is keyed by server ID. Each connected server has its own slot.
**Why:** In a multi-server setup, the user's mental model is "I was in room X on server A". A single global last-room would jump them to the wrong server's room when they switch icons. See ADR-025.
**Tradeoff:** Storage scales linearly with the number of connected servers. In practice that's a handful of bytes per server.

### 2. Server root is a redirector, Overview lives at its own URL

**Decision:** `/chat/{server}` is a redirect-only route. The actual Overview page is rendered at `/chat/{server}/overview`.
**Why:** Re-entering a server you've been using should land you in the room you were last in — that's the dominant case. But the Overview must still be reachable on demand (sidebar nav, Cmd-K, the empty-state link in the room list). Splitting the routes makes the URL itself the source of truth: one URL means "take me back where I was", the other means "show me the Overview".
**Tradeoff:** Two URLs for what was previously one. Worth it; the alternative (querystring flags or shallow state) would couple the page to its callers.

### 3. Channels only — never DMs

**Decision:** Only channel rooms are recorded. Entering a DM does not update the last-room slot.
**Why:** DMs are conversation-driven, not place-driven. Auto-landing in a DM the user opened a week ago is surprising and easily wrong (the conversation may be stale, the user may not want it surfaced first). Channels are the implicit "where I work" destination.
**Tradeoff:** A user whose primary use of a server is DMs gets the Overview as a landing page until they visit a channel. Acceptable; the Overview is a usable starting point.

### 4. Stored on-device in `localStorage`

**Decision:** The record lives in browser `localStorage`, namespaced per server. Not synced to the backend, not synced across devices.
**Why:** "The last room I was in" is contextual to the device and session — what's relevant on your laptop usually isn't relevant on your phone. Local storage is also free and instant; a server-synced version would need a write on every room entry. The same rationale applies to Quick Switcher recents (FDR-015).
**Tradeoff:** The record doesn't survive a cache clear or a switch to a fresh browser profile. The user falls back to Overview, which is harmless.

### 5. Cleared on access failure, not just on sign-out

**Decision:** When a room can't be loaded (deleted, lost access, server says no), the record is cleared and the user is redirected to the server root.
**Why:** Without this, the redirect would loop: server root → last room → access denied → server root → … A stale record is the most likely cause of a "I keep landing in a broken place" experience, and clearing it costs nothing.
**Tradeoff:** If the inaccessibility is transient (a network blip mid-load), the record is wiped and the user falls through to Overview on the next visit. The server validation flow distinguishes transient errors from genuine access denials, so this only fires on the latter.

## Related

- **ADRs:** ADR-025 (multi-instance client architecture)
- **FDRs:** FDR-015 (Quick Switcher), FDR-019 (Room Lifecycle)
