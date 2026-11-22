---
type: fix
---

Projectors now keep events in stream order across a consumer reset. Before, after a reconnect, missed heartbeats, or another ordered consumer reset, the next event could be applied while the previous event was still being decoded. The projection then applied the two events in the wrong order, and `WaitFor` for the previous event could return before that event was applied. The projector now reads its ordered consumer with the `Messages` iterator and handles one message at a time. It also ignores a message with a sequence that it already handled, and it fails when its consumer stops before `Run` ends.
