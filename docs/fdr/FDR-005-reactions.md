# FDR-005: Reactions

**Status:** Active
**Last reviewed:** 2026-06-01

## Overview

Users can react to a message with emoji. Reactions are aggregated into pills shown below the message body, displaying the emoji, a count, and whether the current user has reacted. Multiple users can react with the same emoji on the same message; clicking a pill toggles the current user's vote.

## Behavior

- Each pill shows: the emoji, how many users reacted with it, and a highlight when the current user has reacted.
- Hovering a pill shows a tooltip with up to 5 reactor names plus an overflow count.
- Clicking a pill toggles the current user's reaction.
- On desktop, hovering a message reveals a quick-reaction bar with the user's most recently used emojis (falling back to a default set if none have been used yet).
- Recent emoji selections persist in localStorage so the quick-bar stays personal across sessions.

## Design Decisions

### 1. Reactions key on message event ID

**Decision:** A reaction is keyed by the specific message event ID the user reacted to. A channel echo of a thread reply and the original thread reply are separate visible events, so they accumulate independent reaction state.
**Why:** Message identity lives on the EVT envelope. Keeping reactions attached to the exact envelope matches the GraphQL event model and avoids hidden canonicalization between two visible artifacts.
**Tradeoff:** A reply echoed into the channel can show different reaction counts in the channel and thread views. That is intentional: people are reacting to the appearance they can see.

### 2. Shortcodes, not raw Unicode

**Decision:** Reactions are stored as shortcode names like `thumbsup` or `heart`, drawn from the gemoji dataset (GitHub's emoji set). The frontend converts to display glyphs.
**Why:** NATS KV keys can't contain arbitrary Unicode, and storing the codepoint as a key would also lock us into one particular Unicode version's normalization rules. Shortcodes are stable, portable, and human-readable in storage.
**Tradeoff:** Emojis outside the gemoji set can't be used. The set is large enough that this rarely matters.

### 3. Durable events, in-memory projection is source of truth

**Decision:** Reaction add/remove changes append durable room-aggregate events to EVT (`evt.room.{roomId}.reaction_added` / `reaction_removed`). Current reaction state is derived by an in-memory projection keyed by message event ID, emoji shortcode, and actor/user ID. Live subscribers receive reactions through the EVT stream's `live.evt.>` republish path after projection readiness and authorization checks.
**Why:** Reactions are part of the event-sourcing migration tracked by #596. Keeping them in the room stream makes add/remove ordering explicit, gives replayable state, and removes the KV bucket from the hot read/write path.
**Tradeoff:** The first projection version keeps all current reaction state in RAM. That is simple and correct; bounded or demand-loaded projections can follow once the rest of the event-sourcing architecture is in place and real access patterns are measured.

### 4. GraphQL exposes reactor names as a bounded preview

**Decision:** `ReactionSummary.count` is the total current count, while `ReactionSummary.users(first:)` returns a bounded preview of reacting users (default 3, max 10). The message UI asks for five preview users and renders any remaining count as overflow.
**Why:** Reaction pills need a quick hover tooltip, not an unbounded user directory embedded in every message event. Keeping the full count separate preserves the main signal while preventing popular reactions from inflating timeline payloads.
**Tradeoff:** Clients that need a complete reactor list will need a future dedicated paginated query instead of overloading the message timeline shape.

### 5. Quick-reaction recents are per-device, not per-user

**Decision:** The recent-reactions list lives in `localStorage`, not on the server.
**Why:** Server-side recents would mean a "your recents" query on every message hover (frequent and small) and a new write per reaction. Local storage is free and fast. The downside — losing recents between devices — is small relative to the cost.
**Tradeoff:** Recents don't sync across devices.

## Permissions

- `message.react` — add or remove a reaction on a message. Scoped at server, group, and room.

## Related

- **ADRs:** ADR-026 (event identity via NanoID), ADR-033 (event-sourced state with projections), ADR-034 (single event stream), ADR-035 (per-aggregate migration)
- **FDRs:** FDR-003 (Thread Reply Echo)
