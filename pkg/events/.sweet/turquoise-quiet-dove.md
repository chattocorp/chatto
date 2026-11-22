---
type: fix
---

Update the NATS Go client to v1.56.0. Projector ordered consumers now report missed heartbeats after about 10 seconds instead of 30, and completed durable worker pulls stop their timers.
