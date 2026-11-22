---
type: fix
---

Update the NATS Go client to v1.56.0. Projectors now detect a stalled ordered consumer after about 10 seconds instead of 30, and completed durable worker pulls keep no timers.
