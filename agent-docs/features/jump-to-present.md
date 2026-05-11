# Jump to Present

## Overview

When a user scrolls up from the bottom of a room's message list, a "Jump to Present" button appears, allowing them to quickly return to the latest messages. The button displays the date of the first visible message for context (e.g., "Yesterday | Jump to Present") and changes its label to "New messages" when new messages arrive while scrolled up.

## Triggers

The button appears in two distinct scenarios, both handled by the same `EventList` component:

1. **Scrolled up from bottom** — The user manually scrolls up in a room that has enough messages to overflow the viewport. Clicking the button scrolls back to the bottom.
2. **Jumped mode** — The user navigated to a historical message (e.g., via a reply attribution link or search result). The room loads a window of messages around the target. Clicking the button exits jumped mode entirely, reloading the latest messages from the server.

Both scenarios share the same button styling and date display logic.

## Behavior

- **Date display**: As the user scrolls, the component tracks the first visible event's date using `formatDayLabel` (the same formatter used for day separators). This date appears in the button with reduced opacity, separated by a pipe character.
- **New messages indicator**: When new messages arrive while scrolled up (scenario 1), the button label switches from "Jump to Present" to "New messages". This resets when the user scrolls back to the bottom.
- **Auto-dismiss**: The button disappears when the user scrolls close to the bottom (within 50px). In jumped mode, it also dismisses when the user has scrolled to the bottom and all newer messages have been loaded.
- **Fade transition**: The button uses Svelte's `fade` transition (150ms) for smooth appearance and disappearance.

## Key Implementation Details

- **Scroll detection**: `handleVirtuaScroll` in `EventList.svelte` tracks the scroll offset. It sets `shouldScrollToBottom = false` when the user scrolls up (offset decreases by more than 10px and distance from bottom exceeds 100px), and re-enables it when within 50px of the bottom.
- **Scroll-up lock**: A short timer-based lock (150ms) prevents virtua's internal scroll corrections (`$fixScrollJump`) from immediately re-enabling auto-scroll after a user scroll-up is detected.
- **Date tracking**: The first visible event's date is determined by calling `virtualizerHandle.findItemIndex(offset)` and walking forward through `virtualItems` to find the first event-type item.
- **Jumped mode exit**: In jumped mode, reaching the bottom with `hasReachedEnd = true` automatically calls `onJumpToPresent`, which triggers the parent `RoomEventsPane` to reload the latest page of messages.

## Files

| File | Role |
|------|------|
| `frontend/src/routes/chat/[serverId]/(chrome)/[roomId]/EventList.svelte` | Button rendering, scroll detection, date tracking |
| `frontend/src/routes/chat/[serverId]/(chrome)/[roomId]/RoomEventsPane.svelte` | Jumped mode state management, `onJumpToPresent` handler |
| `frontend/src/lib/utils/formatTime.ts` | `formatDayLabel` — formats dates as "Today", "Yesterday", or localized date strings |
| `frontend/src/lib/state/scrollState.svelte.ts` | Scroll state context shared between EventList and MessageComposer |
