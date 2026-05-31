# FDR-015: Quick Switcher (Cmd-K)

**Status:** Active
**Last reviewed:** 2026-05-31

## Overview

A keyboard-driven palette for jumping between spaces, rooms, DMs, and well-known destinations. Triggered with `Cmd+K` (Mac) or `Ctrl+K` (Windows/Linux). Supports fuzzy search and remembers recently visited destinations.

## Behavior

- `Cmd+K` / `Ctrl+K` opens the palette from anywhere in the app. `Escape` or clicking outside closes it.
- On open, the palette fetches the user's accessible spaces, rooms, and DMs in parallel from all connected Chatto servers.
- Typing filters results with a fuzzy matcher. Items match on both label and detail (e.g., the containing space name); label matches score higher.
- Typing `#` as the first character restricts results to rooms only. The `#` is stripped before matching the rest.
- When the search field is empty, results group as: a "Recent" section first (if any), then by kind — "Go to" (well-known destinations), "Space", "Room", "DM" — each section alphabetical.
- "Go to" destinations are: **Browse Spaces** (shown only if any connected server grants `room.create` or equivalent listing access), **Direct Messages** (shown if any connected server has visible DM conversations or allows starting DMs), **Notifications** (always shown).
- DMs show participants' avatars (up to two for the "other" participants, or the self-avatar for self-DMs) and display names; spaces and rooms show the space logo.
- Multi-server setups show the server name as a detail label so destinations with similar names disambiguate.
- Arrow keys move selection; Enter navigates; the selected item scrolls into view.
- Hovering a result selects it; clicking navigates.
- The 15 most recent destinations are remembered (per device) and surfaced in the "Recent" section. When searching, recent destinations get a score boost so they outrank otherwise-equivalent matches.

## Design Decisions

### 1. Per-server parallel fetch on open

**Decision:** Every time the palette opens, it fires queries in parallel against every connected Chatto server to fetch the user's spaces, rooms, and DMs. Each server uses `requestPolicy: 'network-only'`. One server's failure doesn't block others.
**Why:** A multi-server user expects to see everything in one palette. Pre-loading and caching the global list would mean a stale cache problem; fetching on open keeps it correct. Parallel-with-Promise.allSettled keeps a slow server from breaking the whole palette. See ADR-025.
**Tradeoff:** Open latency depends on the slowest responding server. In practice queries are small and fast.

### 2. Fuzzy match with prefix-bias and recent-boost

**Decision:** Matches on label and detail strings; label matches outrank detail matches; the user's recent destinations get an additional score boost.
**Why:** Three tiers of relevance: exact label match > label substring > detail match. Recent boost layers on top because users who just visited a room are likely to want to go back. Without it, the user's most-likely target can be buried under alphabetical noise.
**Tradeoff:** The scoring is opinionated and not easily tunable per user. Worth it for the speed wins.

### 3. `#` prefix as a room filter

**Decision:** Typing `#` filters results to rooms only and strips the prefix before matching the rest of the query.
**Why:** Power users often know they're looking for a room and want to filter out the noise. `#` is the conventional room sigil — easy to type and easy to remember.
**Tradeoff:** A user whose room name actually starts with `#` (e.g., `#announcements`) might get unexpected matching. The filter strips only the first `#`, so a user searching for `#announce` matches a literal `announce`. Acceptable in practice.

### 4. Recent destinations stored per device

**Decision:** Recent destinations live in `localStorage`, not on the server.
**Why:** "Recent" is contextual to where the user is right now (this device, this session). Syncing across devices isn't valuable — what's recent on your phone is rarely what's recent on your laptop. Local storage is also free and instant.
**Tradeoff:** Recents don't survive cache clearing. Acceptable.

### 5. Well-known destinations gated by access

**Decision:** Browse Spaces only appears if at least one connected server allows listing spaces. Direct Messages appears if the user has DM conversations on a connected server or can start DMs there. Notifications always appears.
**Why:** Showing a destination the user can't reach is a worse experience than hiding it. DMs have no read permission; membership in an existing DM is enough to make the destination useful, while `message.post` means the user can start a new conversation. See ADR-037.
**Tradeoff:** The palette's "Go to" list changes depending on the user's permissions in connected servers. Considered correct behavior.

## Permissions

No dedicated permission. The palette respects whatever the connected servers expose to the user — spaces, rooms, and DMs they can see — plus the gating above for well-known destinations.

## Related

- **ADRs:** ADR-025 (multi-instance client architecture)
- **FDRs:** FDR-007 (Direct Messages), FDR-012 (Notifications)
