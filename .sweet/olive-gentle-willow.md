---
type: fix
---

Update the NATS Go client to v1.56.0, which fixes a data race on shared stream and consumer handles, stops the timers of completed pulls, and makes ordered consumers report missed heartbeats after about 10 seconds instead of 30.
