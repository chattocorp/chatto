---
type: fix
---

An idle `DurableWorker` now holds one pull request for up to 30 seconds (or the consumer's shorter `MaxRequestExpires`) instead of one for each `FetchMaxWait`, which reduces idle JetStream API traffic by about 30 times. The worker becomes idle only after a pull expires empty while no delivery is active, so a backlog and the shutdown handoff use `FetchMaxWait` pulls as before. A replica that disappears without closing its connection can hold its idle pull for up to about 29 seconds; work that the server assigns to that pull waits for the consumer's `AckWait`.
