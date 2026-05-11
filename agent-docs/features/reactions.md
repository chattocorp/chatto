# Reactions

## Overview

- Users can react to messages with emoji reactions.
- Multiple users can react with the same emoji on the same message.
- Reactions are displayed as aggregated pills below the message content, showing the emoji, a count, and whether the current user has reacted.
- Hovering a reaction pill shows a tooltip with the names of users who reacted (up to 5, with overflow count).
- Clicking a reaction pill toggles the current user's reaction.

## Emoji System

- Reactions use shortcode names (e.g., `thumbsup`, `heart`), not raw Unicode. The frontend handles conversion for display.
- Only emojis from the gemoji dataset (GitHub's emoji set) are accepted.
- A quick-reaction bar appears on hover (desktop) with the user's most recently used emojis, falling back to a set of defaults. Recent reactions are persisted to localStorage.

## Interactions

- Reactions attach to **event IDs**, not message body IDs. This means a thread reply and its echo in the room timeline have independent reactions.
- Reaction changes are delivered as live-only events (not persisted). The KV bucket is the source of truth; live events just trigger the frontend to refetch.

## Permissions

- `message.react` — Gates the ability to add and remove reactions. Scoped at the server-role and per-room levels.
