---
type: fix
---

Reduce the NATS requests that the server sends to validate a session or access token from about 25 to about 9, and send them at the same time. This makes authenticated requests faster and decreases the load on NATS.
