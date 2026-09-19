# FDR-011: User Presence

**Status:** Active
**Last reviewed:** 2026-09-19

## Overview

Each account has one saved availability choice on each Chatto server. All of
its devices share that choice. Other users see Online, Away, Do Not Disturb,
or Offline on presence indicators.

## Behavior

- The menu changes only the account on the current server.
- Online, Away, Do Not Disturb, and "Look offline" sync across devices. A new
  connection reads the saved choice before it reports presence.
- Only an explicit selection changes the saved choice. Heartbeats, input
  activity, tab visibility, and reconnects cannot change it.
- When all devices disconnect, public presence becomes Offline after at most
  60 seconds. The saved choice remains for the next connection.
- "Look offline" hides presence across all devices. Other users receive ordinary
  Offline presence. No public flag, private event, heartbeat, or repeated Offline
  event reveals the choice.
- The server suppresses typing indicators while hidden. Read-state updates are
  private to the account. Deliberate messages and call participation remain visible.
- Hidden accounts can still receive messages and realtime updates.
- DND suppresses notification sounds and push alerts across devices, including
  while disconnected. Notifications remain available to read.
- Concurrent selections use revisions. Stale or failed selections show an error
  and reload the current choice instead of silently replacing it.
- Upgrade initializes an absent shared choice from the local choice. Existing
  local hidden choices are preserved on each device's first migration.
- Presence dots update immediately. Member-list grouping waits for a short quiet
  period for other users; the current user's group updates immediately.
- A connection that falls behind presence changes reconnects and reloads current
  presence instead of silently keeping stale indicators.

## Design Decisions

### 1. Separate choices from liveness

**Decision:** Save the private account choice; expire live heartbeats separately.
**Why:** Refreshing a device must not overwrite another device's selection.
**Tradeoff:** The server retains the choice, including the hidden choice.

### 2. Enforce privacy on the server

**Decision:** Only the account can read its choice or receive its private change
event. Public reads, counts, and live events use effective presence.
**Why:** Hiding a dot does not protect against inspection of API or realtime data.
**Tradeoff:** The server operator can observe authenticated traffic. Deliberate
messages and calls still expose activity; this is not an anonymity feature.

### 3. Keep servers independent

**Decision:** A choice applies to one account on one server across its devices.
**Why:** A change in one community must not expose the user in another.
**Tradeoff:** Users change each server separately. No third party is contacted;
devices contact only their configured Chatto server to synchronize the choice.

### 4. Recover without replaying stale choices

**Decision:** Reread after change events, reconnects, conflicts, and lost replies.
**Why:** Delayed requests must not undo newer privacy choices.
**Tradeoff:** The client cannot confirm a selection until the server responds.

### 5. Keep presence fresh without moving member rows on every update

**Decision:** Update indicators immediately, but let other users' member-list
groups settle briefly. A delivery gap forces a reconnect and current-state read.
**Why:** Users need current indicators and a stable list they can scan.
**Tradeoff:** A row can briefly remain in its previous group. A slow connection
can reconnect during a large burst of changes. See ADR-049 and ADR-093.

## Permissions

Authenticated users can read public presence. Only the account's own sessions
can read or change its private choice.

## Related

- **ADRs:** ADR-025, ADR-033, ADR-049, ADR-091
- **FDRs:** FDR-012 (Notifications), FDR-022 (User Profile), FDR-045 (Realtime Event Stream)
