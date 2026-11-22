---
type: fix
---

Projectors now apply events in stream order and exactly once across an ordered consumer reset. Before, after a reconnect, missed heartbeats, or a NATS server change, the next event could be applied while the previous event was still in decode, so the projection applied the two events in the wrong order and `WaitFor` for the previous event could return before that event was applied. A projector now ignores a message whose sequence it already handled, fails when its consumer stops before `Run` ends (for example after the connection closes), and `Run` returns only after the message in progress is handled.
