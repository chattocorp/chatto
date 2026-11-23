---
type: performance
---

Chatto makes fewer notification list requests while a realtime connection is open. While the connection is live and caught up, the bundled app now reads the notification list on its own only when no read occurred for five minutes, not every minute. New notifications still appear immediately. Integrations that use `@chatto/client` can call `ServerStateStore.reconcileNotifications()` once per `NOTIFICATION_RECONCILIATION_INTERVAL_MS` to get the same recovery for lost notification updates.
