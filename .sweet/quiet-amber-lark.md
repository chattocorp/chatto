---
type: fix
---

Run the periodic check for leftover Web Push subscriptions of deleted accounts one time each hour instead of every 15 seconds. On a server with many push subscriptions, the check sent more than a thousand requests to NATS on each run. The cleanup that runs when an account is deleted does not change.
