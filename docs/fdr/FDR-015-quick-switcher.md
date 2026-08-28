# FDR-015: Quick Switcher (Cmd-K)

**Status:** Active
**Last reviewed:** 2026-08-27

## Overview

A keyboard-driven palette for moving between registered servers, joined rooms,
visible DMs, and Notifications. It also searches server members and messages.
Users open it with `Cmd+K` on macOS or `Ctrl+K` on other platforms. It supports
fuzzy matching and remembers recent destinations on the device.

## Behavior

- `Cmd+K` / `Ctrl+K` opens the palette from anywhere in the app. `Escape` or clicking outside closes it.
- On open, the palette reads each registered server's current projected
  navigation state. The empty catalogue contains every registered server,
  joined channel room, visible DM, and Notifications.
- A non-`#` and non-`?` query also searches the member directory on each
  registered server where the viewer can start DMs.
- Typing filters results with a fuzzy matcher. Items match on both label and
  detail, such as the server name. Label matches score higher.
- Typing `#` as the first character restricts results to rooms only. The `#` is stripped before matching the rest.
- Typing `?` as the first character switches to message search. The client asks every registered server whose Search feature is supported and ready or degraded for its top results, then combines them by the provider relevance score. A failed or unavailable server does not hide results from the others.
- Message results identify their author, room, and server. Selecting one opens that message in its room or thread context; message results are not recorded as recent destinations.
- When the search field is empty, results group as: a "Recent" section first
  when it has entries, then by kind: destination, server, room, and DM. Each
  non-recent section is alphabetical.
- Server member results are search-only; they do not appear in the empty palette. Selecting a member starts or reuses a 1:1 DM with that user and navigates to the resulting DM room. Selecting the current user starts or reuses their self-DM.
- Existing DM rooms appear in the empty palette but are not included in typed search results; typed user lookup is handled through the server member results instead.
- Notifications is the only well-known destination. Servers link to their
  Overview page.
- DMs show participant avatars and display names. Servers and channel rooms
  show the server logo.
- Multi-server setups show the server name as a detail label so destinations with similar names disambiguate.
- Arrow keys move selection; Enter navigates; the selected item scrolls into view.
- Hovering a result selects it; clicking navigates.
- The 15 most recent destinations are remembered (per device) and surfaced in the "Recent" section. When searching, recent destinations get a score boost so they outrank otherwise-equivalent matches.

## Design Decisions

### 1. Projected navigation catalogue with parallel searches

**Decision:** Opening the palette composes the current navigation projections
from every registered server. It does not fetch a second room catalogue.
Member and message searches run in parallel against eligible registered
servers. One server's search failure does not block results from another.
**Why:** The per-server projections already own room and DM convergence. Reusing
them makes opening immediate and avoids a duplicate cache lifecycle. Parallel
search still gives users one cross-server result set. See ADR-025.
**Tradeoff:** A server that has not finished its projection catch-up can have an
incomplete catalogue until its normal navigation state converges.

### 2. Fuzzy match with prefix-bias and recent-boost

**Decision:** Matches on label and detail strings; label matches outrank detail matches; the user's recent destinations get an additional score boost.
**Why:** Three tiers of relevance: exact label match > label substring > detail match. Recent boost layers on top because users who just visited a room are likely to want to go back. Without it, the user's most-likely target can be buried under alphabetical noise.
**Tradeoff:** The scoring is opinionated and not easily tunable per user. Worth it for the speed wins.

### 3. `#` prefix as a room filter

**Decision:** Typing `#` filters results to rooms only and strips the prefix before matching the rest of the query.
**Why:** Power users often know they're looking for a room and want to filter out the noise. `#` is the conventional room sigil — easy to type and easy to remember.
**Tradeoff:** A user whose room name actually starts with `#` (e.g., `#announcements`) might get unexpected matching. The filter strips only the first `#`, so a user searching for `#announce` matches a literal `announce`. Acceptable in practice.

### 4. Cross-server message search uses provider scores

**Decision:** A `?` query fans out to compatible servers and sorts the accumulated results by each provider's relevance score. The client does not recalculate relevance. Equal scores fall back to newest first and then a stable message ID order.
**Why:** Round-robin merging quickly becomes noisy for users registered with many servers. Search providers already have the corpus statistics and query model needed to rank their own hits; discarding that signal would make the combined list substantially less useful.
**Tradeoff:** Raw scores are only directly comparable while servers use compatible query normalization and scoring implementations. This deliberately simple first version accepts that limitation; a future federation-aware scoring contract can replace it if operators adopt materially different providers.

### 5. Recent destinations stored per device

**Decision:** Recent destinations live in `localStorage`, not on the server.
**Why:** "Recent" is contextual to where the user is right now (this device, this session). Syncing across devices isn't valuable — what's recent on your phone is rarely what's recent on your laptop. Local storage is also free and instant.
**Tradeoff:** Recents don't survive cache clearing. Acceptable.

### 6. Keep the well-known destination list small

**Decision:** Notifications is the only well-known destination. Server entries
open server Overview pages. Visible DMs appear as their rooms, and member search
starts new DMs when the viewer has permission.
**Why:** Each current destination has a concrete target. Separate browsing and
DM landing pages no longer exist.
**Tradeoff:** Users discover new rooms through server navigation rather than a
dedicated switcher destination.

## Permissions

No dedicated permission. The palette uses each server's projected navigation
visibility. Member search runs only where the viewer can start a DM, and
message search uses the server's Search availability and normal read boundary.

## Related

- **ADRs:** ADR-025 (multi-instance client architecture)
- **FDRs:** FDR-007 (Direct Messages), FDR-012 (Notifications)
