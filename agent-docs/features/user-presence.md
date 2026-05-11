# User Presence

## Overview

- Users have a presence status visible to others: **Online**, **Away**, **Do Not Disturb**, or **Offline**.
- Presence is displayed as a colored dot on user avatars throughout the UI.
- Presence is server-wide (not per-space) — a user has one status across all spaces.

## How Presence Works

- When a user connects (subscribes to `myEvents`), their status is set to Online.
- The server refreshes the presence entry every 30 seconds to keep it alive.
- If the client disconnects without cleanup, presence expires via a 60-second TTL (no explicit "offline" message needed).
- Offline is not a stored state — it's inferred from the absence of a presence entry.

## Auto-Away

- After 5 minutes of inactivity (no mouse/keyboard/touch), the client transitions to Away.
- When the browser tab is hidden for 10 seconds, the client also transitions to Away (debounced to avoid flashing on quick tab switches).
- Any user interaction returns the status to Online.
- Users can manually set Do Not Disturb, which is not overridden by auto-away.

## Real-Time Updates

- Presence changes are broadcast as live events via the space subscription.
- The frontend maintains a global presence cache so all avatar components reflect the latest status.
- Deduplication prevents broadcasting redundant heartbeat refreshes (same status repeated every 30 seconds).
- The cache is cleared on WebSocket reconnect to avoid stale indicators.

## Multi-Server

- Each connected server has its own presence tracking.
- The frontend's auto-away detection broadcasts status changes to all connected servers.

## Authorization

- Presence status is public — any authenticated user can see any other user's presence.
