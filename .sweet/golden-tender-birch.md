---
type: fix
---

Chatto sends fewer read requests to the NATS stream leader: a read-your-writes wait no longer reads the written message when the projection already shows it.
