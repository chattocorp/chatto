---
type: fix
---

An idle `DurableWorker` now waits up to 30 seconds in one pull request instead of up to `FetchMaxWait`, which reduces idle JetStream API traffic by about 30 times. While a delivery is active, the worker still uses `FetchMaxWait` pulls, so shutdown handoff redelivers work after `FetchMaxWait` plus a small margin, as before.
