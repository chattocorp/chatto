# FDR-005: Reactions

**Status:** Active
**Last reviewed:** 2026-10-28

## Overview

Users can react to a message with emoji. Reactions are aggregated into pills shown below the message body, displaying the emoji, a count, and whether the current user has reacted. Multiple users can react with the same emoji on the same message; clicking a pill toggles the current user's vote.

## Behavior

- Integrations can read all user IDs for a message reaction in bounded pages.
  The five-user preview does not limit this list. The list requires the same
  access as the message and accepts channel echoes as aliases.

- Each pill shows: the emoji, how many users reacted with it, and a highlight when the current user has reacted.
- Hovering a pill shows a tooltip with up to 5 reactor names plus an overflow count.
- A message with reactions has a Reactions action in its desktop menu and touch action sheet, even when the reader cannot add a reaction. It opens a details view with the count for each emoji. Readers select an emoji to see all accounts that used it. Long lists load in pages.
- Clicking a pill toggles the current user's reaction.
- Adding or removing a reaction requires room membership and `message.react`.
  In a channel room, it also requires broad `message.read`, or
  `message.read-interactions` with a relationship to the message's thread. DM
  membership authorizes the read.
- Another user's reaction to your message creates one exact Ambient
  notification occurrence when the reaction preference resolves to
  Notification or Push notification. Your own reactions do not notify you.
  Removing the reaction removes that exact occurrence.
- The notification list may consolidate every actor and emoji for the same
  reacted-to message into one row, while unread badges continue to count the
  exact underlying occurrences.
- A user can add up to 20 distinct emoji reactions to one message. Reaching the limit rolls back the attempted reaction and shows a specific explanation; removing a reaction frees a slot.
- The quick-reaction bar, desktop menu, and touch action sheet start with
  thumbs up, wave, laugh, and pray, in that order. They also show up to two
  distinct non-pinned emojis selected through message reaction pickers,
  with the most recent choice first. Empty recent slots have no fallback.
- Reaction-picker choices update quick reactions immediately, including after
  a room change. Profile-status choices, quick-reaction buttons, and reaction
  pills do not change this history. A picker choice remains in the history
  even if its reaction request fails.
- Reaction history is local to the browser and server and persists across
  reloads. It starts empty and does not import the general emoji history.
  The full emoji picker's Recently Used section keeps its separate history,
  which includes profile-status choices.

## Design Decisions

### 1. Reactions key on canonical message event ID

**Decision:** A reaction is keyed by the canonical message event ID. For ordinary messages this is the visible event ID; for a channel echo of a thread reply, it is the original thread reply event ID. Echo event IDs remain accepted as aliases at API boundaries.
**Why:** The echo and original render the same contribution in different views. Sharing one reaction set keeps counts, viewer state, and reactor previews consistent wherever the reply appears.
**Tradeoff:** Reaction reads and replay need the room timeline's echo link to resolve aliases. Historical echo-keyed reaction facts are interpreted as reactions on the original reply without rewriting EVT.

### 2. Shortcodes, not raw Unicode

**Decision:** Reactions use case-sensitive names with 1–128 ASCII letters, digits,
underscores, plus signs, or hyphens, without surrounding colons. Names remain
unchanged. Aliases such as `+1` and `thumbsup` remain separate reactions. The
frontend shows known names as emoji and unknown names as `:name:`. Both support
counts, toggling, and reactor lists. Known names also accept Slack-style skin-tone
suffixes, from `::skin-tone-2` (light) to `::skin-tone-6` (dark). For example,
`thumbsup::skin-tone-4` displays as 👍🏽. Compound emoji use complete Unicode
sequences. Supported mixed-tone pairs use a suffix such as `::skin-tone-2-6`,
with tone numbers in participant order.
Unsupported tone combinations are rejected. The picker and autocomplete popup offer yellow and five
skin tones. Its default is saved in the browser, shared across servers, and
applies to supported picker and autocomplete choices. Recent choices retain
their exact tone and select immediately. The picker opens a separate, headerless
tone list after selecting an emoji that supports skin tones. Back or Escape restores the
previous search, scroll position, and source button focus. Accepting an autocomplete suggestion with Enter, Tab, or a
click opens a separate skin-tone popup when variants are available. It highlights
the saved tone. Up and Down select a variant; Enter or Tab inserts it and saves
its tone. Escape returns to the selected suggestion. Emoji without tone variants
insert immediately. Explicit shortcodes and existing reactions remain unchanged.
Separate tone controls for mixed-tone pairs are not available.
Plain names and aliases keep their existing identity.
Reactor lists and removal use the exact stored name, even when current validation
rejects that name for new reactions. Removal still requires the actor's own
reaction and normal room permissions; an absent reaction is an idempotent no-op.
Hover tooltips show the emoji and a localized sentence with up to five account
names. Locale-aware list formatting supplies punctuation and conjunctions.
**Why:** Imports must retain reactions whose names are not in the bundled emoji
catalogue. Clients can render these names without custom emoji assets.
**Tradeoff:** Custom assets and name lifecycle rules are not implemented. Unknown
names have no image. Existing data and replay semantics remain unchanged.

### 3. Durable events, in-memory projection is source of truth

**Decision:** Reaction add/remove changes append durable room-aggregate events to EVT (`evt.room.{roomId}.reaction_added` / `reaction_removed`). Current reaction state is derived by an in-memory projection keyed by canonical message event ID, emoji shortcode, and actor/user ID. The projection consumes the room aggregate namespace so mutation snapshots can pair current reaction state with the room's applied OCC sequence and so replay can resolve echo aliases from prior message facts. Live subscribers receive reactions through the EVT stream's `live.evt.>` republish path after projection readiness and authorization checks.
**Why:** Reactions are durable room facts. Keeping them in the room stream makes add/remove ordering explicit, gives replayable state, removes the old KV bucket from the hot read/write path, and lets duplicate add/remove decisions retry safely under multi-replica contention.
**Tradeoff:** The first projection version keeps all current reaction state in RAM and consumes more room facts than it derives. That is simple and correct; bounded or demand-loaded projections can follow once the rest of the event-sourcing architecture is in place and real access patterns are measured.

### 4. Public APIs expose reactor names as a bounded preview

**Decision:** `ReactionSummary.count` is the total current count, while bounded reactor previews expose only a small set of reacting users. ConnectRPC room timeline responses expose hydrated reaction summaries with bounded preview semantics. Reaction writes use ConnectRPC `MessageService.AddReaction` and `RemoveReaction` in the web client and call the shared core operation model.
**Why:** Reaction pills need a quick hover tooltip, not an unbounded user directory embedded in every message event. Keeping the full count separate preserves the main signal while preventing popular reactions from inflating timeline payloads.
**Tradeoff:** A complete reactor list needs a separate `MessageService.ListReactionUsers` paged read for each emoji. The web client loads a list only when a reader opens the reaction details view and selects that emoji.

### 5. Quick-reaction recents are per-device, not per-user

**Decision:** The recent-reactions list lives in `localStorage`, not on the server.
**Why:** Server-side recents would mean a "your recents" query on every message hover (frequent and small) and a new write per reaction. Local storage is free and fast. The downside — losing recents between devices — is small relative to the cost.
**Tradeoff:** Recents don't sync across devices.

### 6. Realtime sends semantic reaction changes

**Decision:** Authorized reaction additions and removals are semantic public
events. The web client applies them to its retained message projection. A short
socket gap resumes from the last in-memory cursor. A fresh or unsafe resume
uses current snapshot state for retained data.
**Why:** Bots and alternate clients need the exact reaction transition, while
the frontend needs current aggregate reaction state. One semantic event can
provide the transition and the authorized resource context without exposing a
frontend-only upsert operation. See ADR-091 and FDR-045.
**Tradeoff:** A snapshot restores current reaction state but does not recreate
every add and remove transition from a long offline interval. Reactions on
older messages remain available through normal timeline pagination.

For an echoed thread reply, the reaction event identifies the original reply.
Clients refresh the linked rows they have loaded. Each read resolves the same
canonical reaction set for the original and its visible echo.

### 7. Web client reaction clicks are optimistic

**Decision:** The web client applies add/remove reaction clicks to the visible message store immediately, then reconciles the touched emoji from the ConnectRPC response. The server remains authoritative: semantic realtime reaction events reconcile the local row with current aggregate state.
**Why:** Reaction clicks should feel instant without changing the durable event model or public API.
**Tradeoff:** Reactor-name tooltips are best-effort during the optimistic window and become exact after the projected row refresh.

### 8. Each user can add at most 20 reactions per message

**Decision:** One user may have at most 20 distinct emoji reactions on one canonical message. The cap applies equally to members, moderators, administrators, and owners. Historical messages that already exceed the cap keep all their reactions, but affected users cannot add another until removals bring them below the limit.
**Why:** A fixed upper bound prevents one account from creating an unbounded number of reaction facts or overwhelming the message UI while remaining generous for ordinary use. Applying the rule to the canonical message also prevents thread-reply echoes from becoming a second allowance.
**Tradeoff:** Operators cannot tune or bypass the limit. A future tier-aware configuration system can revisit that choice if communities demonstrate materially different needs.

### 9. Reaction authorization is request-time and room-scoped

**Decision:** Every user-facing add/remove attempt captures the room aggregate
tail, waits the projections used by membership, applicable channel-room
message-read authority, `message.react`, room state, message aliasing, and
reaction-limit decisions, and evaluates the complete operation-level gate. A concurrent room change
rejects the append and reruns the decision. A cross-aggregate authorization
change does not retroactively cancel an already-authorized, otherwise
conflict-free attempt.

**Why:** Reactions are low-risk, high-frequency room mutations. Request-time
authorization matches normal command semantics and avoids serializing reaction
traffic with every unrelated EVT fact. Room OCC still protects message
identity, archive state, duplicate state, and the per-user limit from stale
decisions.

**Tradeoff:** A revocation can commit immediately before a previously
authorized reaction commits. Subsequent attempts observe the new authorization
state. A command with a genuine stream-wide invariant must select and document
a stronger OCC boundary explicitly.

## Permissions

- `message.react` — add or remove a reaction on a message at the applicable scope.
- `message.read` — read any target message and its aggregate
  reaction state.
- `message.read-interactions` — read the target and reaction state when its
  thread has an interaction relationship.

## Related

- **ADRs:** ADR-026 (event identity via NanoID), ADR-033 (event-sourced state with projections), ADR-034 (single event stream), ADR-035 (per-aggregate migration), ADR-042 (protobuf-first public API), ADR-044 (ConnectRPC service conventions), ADR-048 (frontend optimistic UI), ADR-068 (selectable event mutation consistency boundaries), ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list), ADR-080 (explicit message-read permissions), ADR-082 (derived thread interactions), ADR-087 (request-time authorization with aggregate OCC), ADR-089 (server content view), ADR-091 (semantic realtime events)
- **FDRs:** FDR-003 (Thread Reply Echo), FDR-012 (Notifications), FDR-039 (Message Access & Interactions), FDR-045 (Realtime Event Stream)
