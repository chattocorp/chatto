# FDR-010: Typing Indicators

**Status:** Active
**Last reviewed:** 2026-10-10

## Overview

When a user composes a message, others see avatars, names, and animated dots in the room or thread. The indicator disappears shortly after typing stops or when the message is sent.

## Behavior

- Typing in the composer publishes a typing event to other room members. A
  receiver needs `message.read` for a room indicator. A thread
  indicator also permits `message.read-interactions` with a relationship to
  that thread. Room membership is also required.
- Current clients refresh typing state through ConnectRPC
  `RoomService.RefreshTypingIndicator`.
- The bundled client shows up to three avatars and two names. Larger groups show the two names and a count of the other people. If a typing user's profile is missing, the client loads it and replaces the translated unknown-user label when the name arrives. The unknown user still counts toward the group size. A missing display name falls back to the login.
- The indicator is a pill on the top edge of the composer input, at the inline-end side. It shows only when the reader can read the messages of the room or thread. It does not cover messages, and it does not move the messages or the composer when it appears or disappears. The complete block fades and scales between 96% and 100% over 150 ms with easing. Long labels truncate to fit the composer width. The complete label stays available to assistive technology.
- A bright dot moves clockwise around a 3×3 grid with a fading trail beside the label. A persistent polite status region announces text changes. Avatars and dots are decorative. Reduced motion disables the fade, scale, and dot animation.
- The indicator is removed immediately when the user actually posts a message.
- Room typing and thread typing are tracked separately. The room view only shows indicators for users typing in the room timeline (not in any thread). A thread pane only shows indicators for users typing in that specific thread.

## Design Decisions

### 1. Live-only events, never persisted

**Decision:** Typing events publish as transient live messages on the live-event channel. They are not written to JetStream.
**Why:** Typing has zero audit value — it's interesting only in the moment. Storing a stream of "X is typing" events would bloat the event log without ever being read back. See ADR-012.
**Tradeoff:** A client that misses the cursorless realtime event briefly does not see the indicator. This is acceptable because the indicator is decoration, not state.

### 2. 2-second send debounce, 6-second display TTL

**Decision:** The sender debounces typing events to at most one every 2 seconds. Receivers display the indicator for 6 seconds after the last received event, then clear it.
**Why:** Without a send debounce, every keystroke would publish — wasteful at scale. The 6-second display TTL is long enough that a typing user looks continuously active, but short enough that an abandoned compose doesn't leave a stuck indicator.
**Tradeoff:** Up to 6 seconds of "ghost" typing indicator if a user closes the composer abruptly. The cost of that is just visual.

### 3. Debounce resets after a message is sent

**Decision:** When the user posts, the next keystroke immediately fires a new typing event without waiting for the debounce window.
**Why:** Posting is a strong signal that the next typing burst is a _new_ message, not a continuation. Making the next typing event instant means the indicator shows up promptly for the next message.
**Tradeoff:** None worth noting.

### 4. Room and thread typing are independently scoped

**Decision:** A user typing in a thread does not appear as "typing" in the room timeline, and vice versa.
**Why:** Otherwise the room timeline would show typing indicators for every active thread inside it, which would be noisy. Each location only shows the people typing _there_.
**Tradeoff:** A user typing in a thread isn't visible to people who haven't opened the thread. Matches expectations.

### 5. The indicator sits on the composer edge

**Decision:** The indicator is positioned absolutely on the top edge of the composer input. It is not an overlay on the message viewport, and it does not have reserved space.
**Why:** An overlay on the message viewport covered the newest message, mostly in narrow thread panes (#2798). Reserved space adds a permanent empty gap. Space that appears only during typing moves the messages. An indicator in the input row makes the draft narrower, or it must hide the names in narrow panes. The composer edge does not show message content.
**Tradeoff:** The pill can cover the lower edge of the row above the input, such as the formatting toolbar, a composer hint, or a link preview. These controls stay usable because the pill does not receive pointer input.

## Permissions

Room membership is required to send a typing indicator. A receiver
needs effective `message.read` authority for a room indicator. A thread
indicator also permits `message.read-interactions` with a relationship to that
thread. Sending remains independent of read authority so a write-only account can compose messages
without receiving other users' message activity.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-080 (explicit message-read
  permissions), ADR-082 (derived thread interactions)
- **FDRs:** FDR-002 (Replies & Threads), FDR-039 (Message Access &
  Interactions)
