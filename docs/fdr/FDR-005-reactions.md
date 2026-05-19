# FDR-005: Reactions

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Users can react to a message with emoji. Reactions are aggregated into pills shown below the message body, displaying the emoji, a count, and whether the current user has reacted. Multiple users can react with the same emoji on the same message; clicking a pill toggles the current user's vote.

## Behavior

- Each pill shows: the emoji, how many users reacted with it, and a highlight when the current user has reacted.
- Hovering a pill shows a tooltip with up to 5 reactor names plus an overflow count.
- Clicking a pill toggles the current user's reaction.
- On desktop, hovering a message reveals a quick-reaction bar with the user's most recently used emojis (falling back to a default set if none have been used yet).
- Recent emoji selections persist in localStorage so the quick-bar stays personal across sessions.

## Design Decisions

### 1. Reactions key on event ID, not message body ID

**Decision:** A reaction is keyed by the event it was added to. A thread reply and its channel echo therefore accumulate reactions independently.
**Why:** Reacting in the channel is a different social signal from reacting inside the thread. Combining them would mute one of those signals. See FDR-003.
**Tradeoff:** The total reaction count across a reply-and-echo pair is split between two events. No single canonical number. Matches user intuition.

### 2. Shortcodes, not raw Unicode

**Decision:** Reactions are stored as shortcode names like `thumbsup` or `heart`, drawn from the gemoji dataset (GitHub's emoji set). The frontend converts to display glyphs.
**Why:** NATS KV keys can't contain arbitrary Unicode, and storing the codepoint as a key would also lock us into one particular Unicode version's normalization rules. Shortcodes are stable, portable, and human-readable in storage.
**Tradeoff:** Emojis outside the gemoji set can't be used. The set is large enough that this rarely matters.

### 3. Live-only events, KV is source of truth

**Decision:** Reaction add/remove changes publish as transient live events; the KV bucket holds the authoritative state. Live events just trigger the frontend to refetch.
**Why:** A reaction event has no audit value (the current count is what matters; the history doesn't). Persisting them to JetStream would multiply event volume for no gain. See ADR-006 and ADR-012.
**Tradeoff:** A client that misses the live event briefly shows stale counts until it next refetches. Acceptable given the visual stakes.

### 4. Quick-reaction recents are per-device, not per-user

**Decision:** The recent-reactions list lives in `localStorage`, not on the server.
**Why:** Server-side recents would mean a "your recents" query on every message hover (frequent and small) and a new write per reaction. Local storage is free and fast. The downside — losing recents between devices — is small relative to the cost.
**Tradeoff:** Recents don't sync across devices.

## Permissions

- `message.react` — add or remove a reaction on a message. Scoped at server, group, and room.

## Related

- **ADRs:** ADR-006 (KV as source of truth, streams as audit logs), ADR-012 (two-tier real-time events), ADR-026 (event identity via NanoID)
- **FDRs:** FDR-003 (Thread Reply Echo)
