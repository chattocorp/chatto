# FDR-005: Reactions

**Status:** Active
**Last reviewed:** 2026-05-26

## Overview

Users can react to a message with emoji. Reactions are aggregated into pills shown below the message body, displaying the emoji, a count, and whether the current user has reacted. Multiple users can react with the same emoji on the same message; clicking a pill toggles the current user's vote.

## Behavior

- Each pill shows: the emoji, how many users reacted with it, and a highlight when the current user has reacted.
- Hovering a pill shows a tooltip with up to 5 reactor names plus an overflow count.
- Clicking a pill toggles the current user's reaction.
- On desktop, hovering a message reveals a quick-reaction bar with the user's most recently used emojis (falling back to a default set if none have been used yet).
- Recent emoji selections persist in localStorage so the quick-bar stays personal across sessions.

## Design Decisions

### 1. Reactions key on canonical message event ID

**Decision:** A reaction is keyed by message event ID. When the user reacts to a channel echo of a thread reply, the backend canonicalizes the target to the original thread reply event ID, so the echo and original share reaction state.
**Why:** This preserves current product behavior while reactions move into the event-sourced architecture. Both surfaces represent the same underlying reply, and a shared reaction count avoids divergent state between the channel and thread views.
**Tradeoff:** The channel echo is not an independent social surface for reactions. If we want split reactions later, that should be a deliberate behavior change rather than a storage migration side effect.

### 2. Shortcodes, not raw Unicode

**Decision:** Reactions are stored as shortcode names like `thumbsup` or `heart`, drawn from the gemoji dataset (GitHub's emoji set). The frontend converts to display glyphs.
**Why:** NATS KV keys can't contain arbitrary Unicode, and storing the codepoint as a key would also lock us into one particular Unicode version's normalization rules. Shortcodes are stable, portable, and human-readable in storage.
**Tradeoff:** Emojis outside the gemoji set can't be used. The set is large enough that this rarely matters.

### 3. Durable events, in-memory projection is source of truth

**Decision:** Reaction add/remove changes append durable room-aggregate events to EVT (`evt.room.{roomId}.reaction_added` / `reaction_removed`). Current reaction state is derived by an in-memory projection keyed by message event ID, emoji shortcode, and actor/user ID. Live subscribers receive reactions through the EVT stream's `live.evt.>` republish path after projection readiness and authorization checks.
**Why:** Reactions are part of the event-sourcing migration tracked by #596. Keeping them in the room stream makes add/remove ordering explicit, gives replayable state, and removes the KV bucket from the hot read/write path.
**Tradeoff:** The first projection version keeps all current reaction state in RAM. That is simple and correct; bounded or demand-loaded projections can follow once the rest of the event-sourcing architecture is in place and real access patterns are measured.

### 4. Quick-reaction recents are per-device, not per-user

**Decision:** The recent-reactions list lives in `localStorage`, not on the server.
**Why:** Server-side recents would mean a "your recents" query on every message hover (frequent and small) and a new write per reaction. Local storage is free and fast. The downside — losing recents between devices — is small relative to the cost.
**Tradeoff:** Recents don't sync across devices.

## Permissions

- `message.react` — add or remove a reaction on a message. Scoped at server, group, and room.

## Related

- **ADRs:** ADR-026 (event identity via NanoID), ADR-033 (event-sourced state with projections), ADR-034 (single event stream), ADR-035 (per-aggregate migration)
- **FDRs:** FDR-003 (Thread Reply Echo)
