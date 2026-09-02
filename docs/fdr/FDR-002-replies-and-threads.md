# FDR-002: Replies & Threads

**Status:** Active
**Last reviewed:** 2026-09-02

## Overview

Chatto messages can link to one another via reply attribution, and channel-room messages can live inside threads — conversations branching off a root message. Replies and threads are independent concepts: a message can reply without being in a thread, or live in a thread without referencing a specific parent. Channel rooms can be configured to promote one shape over another; DMs support reply attribution but not threads.

## Behavior

- A message in a room can optionally reference another message as the one it's in reply to.
- DMs keep replies in their single room timeline and do not offer thread actions. Historical DM threads remain readable but cannot receive new replies.
- A reply renders with a byline above the message body: the referenced author's small avatar, name, and a single-line excerpt of the referenced message.
- Clicking the byline transports the user to the referenced message and briefly highlights it.
- Clicking the avatar or name in the byline opens the user's context menu.
- If the user selects text inside a message body before choosing Reply or Reply in thread, the target composer inserts that selected plain text as a Markdown blockquote while preserving any existing draft text.
- A thread is a sequence of messages starting from a root message and continuing inside a dedicated thread pane. Threads can contain plain messages or reply-attributed messages; both are valid.
- Posting a reply attempts to follow the thread for its author, even after an
  earlier unfollow. The first reply also attempts to follow the root author when
  they have never made a follow choice. These post-commit subscription writes
  are best-effort and cannot roll back the message.
- A delivered direct mention in a channel-room root or reply attempts to
  follow that thread when the recipient has no prior follow state. For a root
  mention, the root message ID identifies the thread for future replies.
- Every channel room has a Threading Mode:
  - **Required** — every new root atomically establishes its thread. The room composer keeps **Post as thread** visible, selected, and locked so the policy is explicit without presenting a false choice. The standard **Reply** action and adjacent **Reply in thread** action keep their usual order; either opens the root's thread, while **Reply** also preserves reply attribution. Inside the thread, **Reply** creates attribution in that thread. The server rejects replies to roots unless they are placed in that root's thread. Automatic root-thread creation needs `message.post`; posting an actual thread reply still needs `message.post-in-thread`.
  - **Encouraged** — both flat and threaded conversation remain valid. The standard **Reply** action opens the root's thread with reply attribution, while the adjacent **Reply in thread** action keeps its usual position. **Reply in room** remains available as a secondary expanded-menu action. **Post as thread** starts selected for each new root draft, but the author may turn it off. If a member can post in the room but cannot post in threads, the standard reply falls back to the room and the composer cannot establish a thread.
  - **Enabled** — the default and unrestricted behavior. Authors may opt into **Post as thread**, and other members may start a thread later.
  - **Disabled** — the server rejects new threads, thread replies, thread typing
    indicators, and new channel echoes from historical thread replies. Ordinary
    in-room reply attribution remains available. Existing threads and channel
    echoes stay readable after a room changes to Disabled. Authors can remove a
    historical echo while they can edit the reply. Effective `message.manage`
    permits this removal after the normal edit window.
- Outside Required rooms, a root explicitly posted as a thread is immediately marked as a thread and the author follows it, while the composer remains in the room timeline. If nobody explicitly establishes an ordinary root's thread, the first thread reply establishes one implicitly.
- Threading Mode changes are prospective: they do not backfill threads for existing roots or remove historical thread state.
- A successful Threading Mode change appears as an actor-attributed room timeline event. The same ordered realtime update also refreshes room metadata, so open composers and reply actions react immediately.
- Before a user posts another root within five minutes of their latest root in the room, the client checks whether that previous root now has a thread. If it does, the client asks whether to continue in that thread or post the prepared root as-is. This also covers a thread another user established after the root was posted. Cancelling preserves the draft. The prompt is omitted when the user cannot post in that thread or when the current room policy forbids thread replies.
- Thread badges in the room timeline are normal links to the thread URL, so users can copy or open the thread link through browser-native link actions.
- Links copied from messages inside a thread reopen that thread and focus the linked message. A root message can be opened in its thread pane before the thread has any replies.
- My Threads lists followed conversations with their root, latest visible
  reply, participant preview, reply count, activity time, reply unread state,
  and client-side decoration for a matching unread notification.
- An open thread overlays the dimmed, inactive room timeline by default. A user can instead select a side-by-side layout in App Preferences. The side-by-side layout keeps both panes interactive when the room area is wide enough and uses the overlay when the area becomes too narrow. The side-by-side thread pane is resizable, and the app remembers its width on the device.
- Within the room's Threading Mode, a user can post a plain message into a room, a reply into the room timeline, a plain message into a thread, or a reply inside a thread. Location permissions still gate the allowed operations independently.

## Design Decisions

### 1. Replies and threads are orthogonal in the data model

**Decision:** A message's reply target and its containing thread remain independent fields. Threading Mode may constrain which combinations can be newly written in a room, without changing the persisted message shape or rewriting history.
**Why:** Different communities want different conversation shapes. Keeping the primitives orthogonal preserves reply attribution and historical readability, while the room policy can still provide strict thread-everything, a gentle thread preference, unrestricted threads, or no new threads.
**Tradeoff:** Every message write must validate the current room policy as well as its location permissions. A mode change can therefore reject an in-flight post rather than silently relocating it.

### 2. Posting permissions are split by location only, not by reply attribution

**Decision:** Two posting permissions: `message.post` (room timeline) and `message.post-in-thread` (inside a thread). Reply attribution (`inReplyTo`) is **not** separately gated — anyone who can post can reply.
**Why:** Operators want to express patterns like "everyone can reply in threads, but only certain roles can post root messages" — that's the room-vs-thread axis, which the two permissions cover. Reply attribution is message presentation and notification semantics, not a distinct posting location or moderation boundary. A reply may create its own notification occurrence independently of a direct mention in the same message; the recipient controls that reply cause through notification policy.
**Tradeoff:** Operators who genuinely want to disable reply attribution as a UI affordance cannot do so through permissions. Recipients can suppress reply notifications without disabling the reply feature itself.

### 3. Reply attribution doesn't change storage

**Decision:** A reply is a normal message with an extra `inReplyTo` field. It's not stored differently.
**Why:** Reply attribution is a presentation concern. Special-casing the storage would mean every read path has to handle two flavors of message.
**Tradeoff:** Bulk operations (deleting a message, etc.) need to consider whether replies still make sense after the target is gone. The UI handles this by gracefully degrading the byline.

### 4. Thread timelines use cursor pagination

**Decision:** Thread timelines load chronological pages through opaque cursors. The initial page includes the root and the latest replies. Continuation pages load older or newer replies without repeating the root.
**Why:** Threads can grow large, so bounded pages keep reads predictable and use the same navigation model as room timelines.
**Tradeoff:** Clients combine the root-bearing initial page with reply-only continuation pages.

### 5. Anchored thread reads preserve the visible window

**Decision:** An anchored thread read returns a chronological window that includes the root and the requested root or reply, and identifies the requested event's position. Anchoring on the root loads the top of the thread.
**Why:** Reconnects and message links preserve the reader's visible location instead of jumping to the latest replies.
**Tradeoff:** Thread clients support both ordinary cursor paging and anchored windows.

### 6. Thread message links identify both the thread and focused message

**Decision:** A link copied from the thread pane preserves the thread root separately from the message it focuses. Opening the link shows the thread pane even when the focused message is the root and no replies exist.
**Why:** A message identifier alone can locate a reply's thread after a lookup, but it cannot express that a root message should open as an empty thread. Carrying both identities makes the intended view explicit and directly shareable.
**Tradeoff:** Thread message links contain two event identifiers, making them longer than ordinary room message links.

### 7. Root authors can establish a thread before the first reply

**Decision:** In Enabled and Encouraged rooms, a channel-room root post can explicitly create its thread when the author has both `message.post` and `message.post-in-thread`. In Required rooms, the same atomic thread creation is an automatic consequence of every root post and therefore needs only `message.post`. The root message, `ThreadCreatedEvent`, and root-author `ThreadFollowedEvent` are one atomic room-aggregate write. The durable thread exists even with zero replies, and public messages expose that state by including `Message.thread`; ordinary roots without an established thread omit it.
**Why:** The author can signal the intended conversation shape at posting time instead of leaving the decision to the first person who replies. Atomic creation prevents a visible root from briefly or permanently losing that intent. Keeping the room view stable makes **Post as thread** a posting choice rather than an unexpected navigation action.
**Tradeoff:** Clients must distinguish an established empty thread from an ordinary root with zero replies by checking `Message.thread` presence. Required rooms create an empty thread for every root even when nobody replies.

**Compatibility:** `CreateMessageRequest.create_thread` and the room Threading Mode fields are part of the 0.5 client/server contract. The bundled client does not preserve compatibility with pre-0.5 servers. Historical channel events and snapshots that do not contain a mode normalize to Enabled without a backfill; unknown future channel values fail closed to Disabled on an older binary while remaining raw in projection snapshots, and DMs normalize to Unspecified.

### 8. Thread presentation is an App Preference

**Decision:** The overlay is the default thread presentation. A user can select a side-by-side presentation as an App Preference. The selected mode applies to all registered servers on that device. Side-by-side presentation still uses the overlay when the room area is too narrow. The width check follows the room area after surrounding sidebars, not the browser viewport. The app remembers the side-by-side thread width separately.
**Why:** The overlay keeps attention on one conversation and does not reduce the room timeline width. Users who prefer more context can opt into two interactive panes. The responsive fallback prevents surrounding sidebars from squeezing both conversations into unusable panes.
**Tradeoff:** The overlay makes the room timeline inactive while a thread is open. In side-by-side mode, resizing surrounding panes can temporarily change an open thread to an overlay. The thread remains open and keeps the same URL.

### 9. Threading Mode is enforced at the room aggregate boundary

**Decision:** Room creation persists an explicit Threading Mode, while omitted historical values behave as Enabled. Changes append `RoomThreadingModeChangedEvent`. Message preflight gives clients prompt feedback, and the atomic append rechecks the policy under room-aggregate optimistic concurrency. A conflicting policy change retries the authoritative check and rejects a now-invalid post; it never moves the message to a different timeline.
**Why:** UI steering alone cannot constrain bots, integrations, stale clients, or an in-flight message that races an administrator's setting change. The room aggregate already owns both policy changes and message facts, so it is the natural consistency boundary.
**Tradeoff:** Changing a room's mode can cause a composer submission prepared under the previous mode to fail and require the user to retry in the newly valid location.

### 10. Recent follow-up steering is a client-side destination choice

**Decision:** When the latest root authored by the current user gained a thread and is at most five minutes old, a subsequent root submission pauses before uploads or mentions are processed and offers two explicit destinations: the previous thread or a new root. Choosing the thread preserves the message contents but clears root-only thread creation intent. Choosing the room preserves the prepared root, including Encouraged or Required mode behavior.
**Why:** Rapid follow-up messages are often continuations, but automatically moving text would be surprising and could change its audience. A timely choice catches the likely mistake while leaving both valid conversation shapes available.
**Tradeoff:** The safeguard depends on the client's current retained timeline and clock, so it is guidance rather than a server invariant. The server continues to enforce only permissions and Threading Mode.

## Permissions

- `message.read` — read channel-room and thread timelines. Channel-room
  membership is also required.
- `message.read-interactions` — read a complete channel-room thread when the
  account authored its root or another account directly mentioned it in that
  thread. Channel-room membership is also required. DM membership authorizes
  historical DM thread reads without either read permission.
- `message.post` — post a root message (with or without `inReplyTo`) in a room. Explicitly establishing that root as a thread also requires `message.post-in-thread`; automatic root-thread creation in Required rooms does not.
- `message.post-in-thread` — post a message inside a channel-room thread (with or without `inReplyTo`), and—together with `message.post`—explicitly establish a root as a thread. This permission does not make threads available in DMs.

## Related

- **ADRs:** ADR-011 (message body/event split), ADR-026 (event identity via NanoID), ADR-038 (room-owned thread state), ADR-050 (ephemeral encrypted projection snapshots), ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list), ADR-080 (explicit message-read permissions), ADR-082 (derived thread interactions)
- **FDRs:** FDR-003 (Thread Reply Echo), FDR-012 (Notifications), FDR-039 (Message Access & Interactions), FDR-044 (My Threads)
