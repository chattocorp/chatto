---
type: fix
---

Reduce the NATS requests that the server sends to check the auth generation of a session or access token from about 25 to about 9, and read the stream positions concurrently. Bot API key and incoming webhook authentication, OAuth consent checks, and password checks no longer read stream positions across all users. This makes authenticated requests faster and decreases the load on NATS.
