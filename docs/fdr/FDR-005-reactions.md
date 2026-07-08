# FDR-005: Reactions

**Status:** Active
**Last reviewed:** 2026-07-03

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

**Decision:** A reaction is keyed by the canonical message event ID. For ordinary messages this is the visible event ID; for a channel echo of a thread reply, it is the original thread reply event ID. Echo event IDs remain accepted as aliases at API boundaries.
**Why:** The echo and original render the same contribution in different views. Sharing one reaction set keeps counts, viewer state, and reactor previews consistent wherever the reply appears.
**Tradeoff:** Reaction reads and replay need the room timeline's echo link to resolve aliases. Historical echo-keyed reaction facts are interpreted as reactions on the original reply without rewriting EVT.

### 2. Shortcodes, not raw Unicode

**Decision:** Reactions are stored as shortcode names like `thumbsup` or `heart`, drawn from the gemoji dataset (GitHub's emoji set). The frontend converts to display glyphs.
**Why:** NATS KV keys can't contain arbitrary Unicode, and storing the codepoint as a key would also lock us into one particular Unicode version's normalization rules. Shortcodes are stable, portable, and human-readable in storage.
**Tradeoff:** Emojis outside the gemoji set can't be used. The set is large enough that this rarely matters.

### 3. Durable events, in-memory projection is source of truth

**Decision:** Reaction add/remove changes append durable room-aggregate events to EVT (`evt.room.{roomId}.reaction_added` / `reaction_removed`). Current reaction state is derived by an in-memory projection keyed by canonical message event ID, emoji shortcode, and actor/user ID. The projection consumes the room aggregate namespace so mutation snapshots can pair current reaction state with the room's applied OCC sequence and so replay can resolve echo aliases from prior message facts. Live subscribers receive reactions through the EVT stream's `live.evt.>` republish path after projection readiness and authorization checks.
**Why:** Reactions are durable room facts. Keeping them in the room stream makes add/remove ordering explicit, gives replayable state, removes the old KV bucket from the hot read/write path, and lets duplicate add/remove decisions retry safely under multi-replica contention.
**Tradeoff:** The first projection version keeps all current reaction state in RAM and consumes more room facts than it derives. That is simple and correct; bounded or demand-loaded projections can follow once the rest of the event-sourcing architecture is in place and real access patterns are measured.

### 4. Public APIs expose reactor names as a bounded preview

**Decision:** `ReactionSummary.count` is the total current count, while bounded reactor previews expose only a small set of reacting users. ConnectRPC room timeline responses expose hydrated reaction summaries with bounded preview semantics. Reaction writes use ConnectRPC `MessageService.AddReaction` and `RemoveReaction` in the web client and call the shared core operation model.
**Why:** Reaction pills need a quick hover tooltip, not an unbounded user directory embedded in every message event. Keeping the full count separate preserves the main signal while preventing popular reactions from inflating timeline payloads.
**Tradeoff:** Clients that need a complete reactor list will need a future dedicated paginated query instead of overloading the message timeline shape.

### 5. Quick-reaction recents are per-device, not per-user

**Decision:** The recent-reactions list lives in `localStorage`, not on the server.
**Why:** Server-side recents would mean a "your recents" query on every message hover (frequent and small) and a new write per reaction. Local storage is free and fast. The downside — losing recents between devices — is small relative to the cost.
**Tradeoff:** Recents don't sync across devices.

### 6. Web reconnect catch-up refreshes the current room window

**Decision:** On browser wake/reconnect, the web client refreshes the currently viewed room window from projected ConnectRPC timeline reads instead of replaying missed reaction events through its event bus. If the user is at the bottom it fetches the latest room page; if scrolled up it refetches around the visible anchor event and preserves scroll by event ID.
**Why:** Reactions mutate existing message rows. Refetching projected message rows updates reactions, edits, retractions, attachment processing state, and newly posted messages through one path, while avoiding fragile reconnect replay state in the browser.
**Tradeoff:** Message-row catch-up is scoped to the room/thread the user is actually viewing. Other rooms catch up through normal queries when opened, while server-scoped projected state such as notifications, unread/sidebar state, room layout, server profile/settings, and active-call indicators is refetched after event-bus gaps. The `myEvents` subscription is intentionally live-only and no longer exposes a replay cursor.

### 7. Web client reaction clicks are optimistic

**Decision:** The web client applies add/remove reaction clicks to the visible message store immediately, then reconciles the touched emoji from the ConnectRPC response. The server remains authoritative: live reaction events and reconnect refreshes refetch the projected message row and replace the local optimistic state.
**Why:** Reaction clicks should feel instant without changing the durable event model or public API.
**Tradeoff:** Reactor-name tooltips are best-effort during the optimistic window and become exact after the projected row refresh.

## Permissions

- `message.react` — add or remove a reaction on a message. Scoped at server, group, and room.

## Related

- **ADRs:** ADR-026 (event identity via NanoID), ADR-033 (event-sourced state with projections), ADR-034 (single event stream), ADR-035 (per-aggregate migration), ADR-042 (protobuf-first public API), ADR-044 (ConnectRPC service conventions)
- **FDRs:** FDR-003 (Thread Reply Echo)
