# FDR-014: Jump to Present

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

When a user is reading older messages in a room — either because they scrolled up or because they navigated to a historical message — a "Jump to Present" button appears, letting them return to the latest messages with one click. The button also surfaces context: it shows the date of the first visible message, and switches its label to "New messages" when new ones arrive while the user is scrolled away.

## Behavior

- The button appears in two situations:
  1. **Scrolled up from the bottom** — the user manually scrolled away from the latest messages in a room that has enough history to overflow the viewport.
  2. **Jumped mode** — the user clicked a reply-attribution link, a search result, or a notification, and the room loaded a window of messages around that historical target.
- In either case, clicking the button returns the user to the latest messages.
- The button shows the date of the first visible message ("Yesterday | Jump to Present", "March 12 | Jump to Present").
- When new messages arrive while the user is scrolled up, the button label switches from "Jump to Present" to "New messages".
- The button auto-dismisses when the user is back near the bottom (within ~50px). In jumped mode, it also dismisses if the user has scrolled all the way to the bottom and all newer messages have loaded.
- The button fades in and out smoothly to avoid flicker.

## Design Decisions

### 1. One button, two triggers

**Decision:** The "scrolled away" case and the "jumped mode" case share the same button UI and date display.
**Why:** Behaviorally they're the same affordance — "I'm not looking at the latest, get me there". Two separate buttons would mean redundant UI and divergent logic for the same user need.
**Tradeoff:** The click handler has to branch internally on which mode is active (scroll-to-bottom vs reload-latest). Acceptable; the modes are easy to distinguish.

### 2. Date display alongside the action

**Decision:** The button shows the date of the first visible message inline with the "Jump to Present" label.
**Why:** When users are scrolled deep in history, the date is the most useful piece of context — "where am I in time, and how far is it to the present?". Adding it to the button keeps that context one glance away.
**Tradeoff:** The button is wider than a pure-action button would be. Worth it; the date is a low-clutter element with high informational value.

### 3. "New messages" label when scrolled-away gets new traffic

**Decision:** When new messages arrive while the user is scrolled up, the button's label switches from "Jump to Present" to "New messages".
**Why:** It's a stronger affordance than the badge-and-counter pattern: the same button the user was going to click anyway now carries an urgency signal. Reduces UI surface.
**Tradeoff:** Users who don't realize "New messages" is the same button as "Jump to Present" might be briefly confused. The button position and date display stay constant, which mitigates this.

### 4. Jumped-mode exit reloads from the server

**Decision:** Clicking the button in jumped mode doesn't scroll — it reloads the latest page of messages from the server.
**Why:** In jumped mode the loaded message window is a slice around the target message, with no guarantee that it extends to "now". Scrolling to the bottom of the loaded window would land somewhere arbitrary; the user actually wants to leave jumped mode entirely.
**Tradeoff:** The click triggers a network request. In practice the latest page is usually small and the round-trip is fast.

### 5. Scroll-up lock against virtua corrections

**Decision:** A short (~150ms) lock prevents the message list's internal scroll-jump corrections from immediately re-enabling auto-scroll after a user scroll-up is detected.
**Why:** Without it, the virtualized list's adjustments after a measurement update could be interpreted as the user scrolling down, immediately dismissing the button after the user just scrolled up. The lock filters out those self-induced movements.
**Tradeoff:** Real user scroll-down within the lock window is briefly ignored. The window is short enough that this is imperceptible.

## Related

- **FDRs:** FDR-002 (Replies & Threads), FDR-012 (Notifications)
