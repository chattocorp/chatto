---
type: fix
---

Reduce idle NATS traffic: each idle durable worker now sends one JetStream pull request every 30 seconds instead of about one each second. On an idle server, durable worker pull requests drop from about 27 to about 1 each second for each replica. Workers with pending work, and shutdown handoff to another replica, behave as before. If a replica loses its network connection without closing it, background work that arrives in the next 30 seconds can wait for its acknowledgement timeout (one to two minutes) before another replica runs it.
