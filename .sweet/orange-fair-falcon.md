---
type: fix
---

Fix projections that could apply two events in the wrong order after a NATS reconnect or a NATS server change. The projection could then keep wrong state until the server restarted and replayed it, and a read-your-writes wait could return before its event was applied, so a request could decide from stale state.
