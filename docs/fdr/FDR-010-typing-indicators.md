# FDR-010: Typing Indicators

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

When a user is composing a message, others see a small typing indicator — the typer's avatar plus animated dots — appear in the room or thread. The indicator disappears shortly after typing stops or as soon as the message is sent.

## Behavior

- Typing in the composer publishes a typing event to other room members.
- Receiving clients show the indicator (avatar + animated dots) for a short duration after the last typing event.
- The indicator is removed immediately when the user actually posts a message.
- Room typing and thread typing are tracked separately. The room view only shows indicators for users typing in the room timeline (not in any thread). A thread pane only shows indicators for users typing in that specific thread.

## Design Decisions

### 1. Live-only events, never persisted

**Decision:** Typing events publish as transient live messages on the live-event channel. They are not written to JetStream.
**Why:** Typing has zero audit value — it's interesting only in the moment. Storing a stream of "X is typing" events would bloat the event log without ever being read back. See ADR-012.
**Tradeoff:** A client that misses the live event briefly doesn't see the indicator. Acceptable: the indicator is decoration, not state.

### 2. 2-second send debounce, 6-second display TTL

**Decision:** The sender debounces typing events to at most one every 2 seconds. Receivers display the indicator for 6 seconds after the last received event, then clear it.
**Why:** Without a send debounce, every keystroke would publish — wasteful at scale. The 6-second display TTL is long enough that a typing user looks continuously active, but short enough that an abandoned compose doesn't leave a stuck indicator.
**Tradeoff:** Up to 6 seconds of "ghost" typing indicator if a user closes the composer abruptly. The cost of that is just visual.

### 3. Debounce resets after a message is sent

**Decision:** When the user posts, the next keystroke immediately fires a new typing event without waiting for the debounce window.
**Why:** Posting is a strong signal that the next typing burst is a *new* message, not a continuation. Making the next typing event instant means the indicator shows up promptly for the next message.
**Tradeoff:** None worth noting.

### 4. Room and thread typing are independently scoped

**Decision:** A user typing in a thread does not appear as "typing" in the room timeline, and vice versa.
**Why:** Otherwise the room timeline would show typing indicators for every active thread inside it, which would be noisy. Each location only shows the people typing *there*.
**Tradeoff:** A user typing in a thread isn't visible to people who haven't opened the thread. Matches expectations.

## Permissions

Room membership is required to send and receive typing indicators. No additional permission gate.

## Related

- **ADRs:** ADR-012 (two-tier real-time events)
- **FDRs:** FDR-002 (Replies & Threads)
