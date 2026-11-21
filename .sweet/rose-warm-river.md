---
type: fix
---

Reduce idle NATS traffic: each idle durable worker now sends one JetStream pull request every 30 seconds instead of about one each second. On an idle server, durable worker pull requests drop from about 27 to about 1 each second for each replica. Shutdown still hands active work to another replica promptly.
