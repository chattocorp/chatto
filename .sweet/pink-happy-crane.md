---
type: fix
---

A burst of realtime updates, such as the first message in a room with several hundred members, no longer closes every realtime connection on a server process. Clients no longer reconnect all at the same time in this case.
