# Web Push Notifications

## Overview

- Users can opt in to browser push notifications for DMs, mentions, and replies.
- Push notifications are delivered via the W3C Web Push Protocol using VAPID keys.
- Multiple devices per user are supported — each device registers its own subscription.

## Opt-In Flow

1. The frontend checks if the server has push notifications enabled (VAPID keys configured).
2. If enabled, the user is prompted for browser notification permission.
3. On granting permission, the browser creates a push subscription with the server's VAPID public key.
4. The subscription (endpoint, keys) is sent to the server and stored.
5. If the server save fails, the browser subscription is cleaned up.

## What Triggers a Push

- Push notifications piggyback on the persistent notification system.
- When a persistent notification is created (DM, mention, reply, all-messages), a push is sent to all of the user's subscriptions.
- If no persistent notification is created (e.g., room is muted), no push is sent.
- Dismissing a notification sends a "dismiss" action push to close the system notification on other devices.

## Payload

- Push payloads include: title, a truncated message preview (max 100 chars, smart word-boundary breaking), and a navigation URL.
- Notifications are tagged by event type and ID so they can be grouped/replaced.
- Clicking a push notification navigates to the relevant room/thread/DM.

## Subscription Lifecycle

- Subscriptions are identified by a hash of the push endpoint URL (allowing multiple devices).
- Expired or invalid subscriptions (404/410 from the push service) are automatically cleaned up.
- Account deletion removes all push subscriptions.
- Browser subscription changes (revocation/expiration) are detected and communicated to open tabs for re-subscription.

## Configuration

- Push notifications are opt-in at the server level. Requires VAPID key pair and subject (contact URL) in the server config.
- If not configured, the push notification UI is hidden entirely.
