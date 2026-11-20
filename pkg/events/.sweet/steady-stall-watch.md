---
type: feature
---

Detect stalled projectors. `ProjectorStatus` now reports `ObservedSeq`, `LastCommitAt`, `LastCommitDuration`, `CurrentOperation`, `CurrentOperationDuration`, `PendingFor`, and `Stalled`. A projector is stalled when matching events wait and it commits no progress for `ProjectorOptions.StallAfter` (default five minutes). An idle, caught-up projector and a slow replay that still commits are not stalled. `Run` reads the newest matching stream sequence every 30 seconds, so a consumer that stops delivering also shows pending work. `Status` does not wait for the apply barrier.
