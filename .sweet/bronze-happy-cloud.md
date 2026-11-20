---
type: fix
---

Marking a room or thread as read retries when another server replica is still saving the same notification. Both replicas can now report the saved read state.
