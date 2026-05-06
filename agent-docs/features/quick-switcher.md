# Quick Switcher (Cmd-K)

## Overview

A keyboard-driven palette for quickly navigating between spaces, rooms, DM conversations, and well-known destinations. Activated with `Cmd+K` (Mac) or `Ctrl+K` (Windows/Linux), it loads all items the user has access to and supports fuzzy search with a `#` prefix filter for rooms only.

## Triggers

- `Cmd+K` / `Ctrl+K` opens the palette via `GlobalKeyboardShortcuts.svelte`, which is mounted once at the root layout level.
- Pressing `Escape` or clicking outside the dialog closes it.

## Behavior

- **Data loading**: On open, fires GraphQL queries in parallel across all connected instances to fetch joined spaces, rooms per space, and DM conversations. Multi-instance setups show the instance name as a detail label.
- **Fuzzy search**: Typing filters results using `fuzzyMatch`. Label matches score higher than detail (space name) matches. Results are sorted by score.
- **`#` prefix**: Typing `#` restricts results to rooms only, stripping the prefix before matching.
- **Recent destinations**: The palette remembers the last 15 destinations the user navigated to (stored in localStorage). When the search field is empty, a "Recent" section appears at the top, ordered by recency. When searching, recent destinations receive a score boost so they rank higher than non-recent items with equivalent fuzzy match scores.
- **Well-known destinations**: The palette includes fixed navigation destinations: Browse Spaces, Direct Messages, and Notifications. Browse Spaces only appears if any connected instance grants `canListSpaces`. Direct Messages only appears if any instance grants `canViewDMs`. Notifications is always shown. These display with an icon (rather than a space logo) and a "Go to" kind label.
- **Group headers**: When the search field is empty, results are grouped into a "Recent" section first (if any), followed by kind-based sections (Go to, Space, Room, DM), each sorted alphabetically.
- **Keyboard navigation**: Arrow keys move selection, Enter navigates to the selected item. The selected item scrolls into view automatically.
- **Mouse interaction**: Hovering a result updates the selection; clicking navigates.
- **DM display**: DM results show participant avatars (up to 2 for the "other" participants, or self-avatar for self-DMs) and display names instead of a room name.
- **Space logos**: Space and room results show the space logo (image or gradient-backed initial letter).

## Key Implementation Details

- Uses a native `<dialog>` element with `showModal()` for proper focus trapping and backdrop.
- Data is fetched with `requestPolicy: 'network-only'` to always get fresh results.
- All instance/space/room fetches use `Promise.allSettled` so one failure doesn't block others.
- The `filtered` list is a `$derived.by` computation that re-runs whenever `query` or `allItems` changes, resetting `selectedIndex` to 0.

## Files

| File | Role |
|------|------|
| `frontend/src/lib/components/QuickSwitcher.svelte` | Palette UI, data loading, fuzzy filtering, navigation |
| `frontend/src/lib/components/GlobalKeyboardShortcuts.svelte` | `Cmd+K` binding, mounts QuickSwitcher |
| `frontend/src/lib/state/recentQuickSwitcher.svelte.ts` | Recent destinations tracking (localStorage) |
| `frontend/src/lib/fuzzyMatch.ts` | Fuzzy string matching algorithm |
| `frontend/e2e/quick-switcher.test.ts` | E2E tests for open/close, search, keyboard nav, click nav |
