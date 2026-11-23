---
type: performance
---

Rooms with many open clients put less load on the server after each new message. A realtime `message_posted` event now sets `complete` and `viewer_state` when it holds the full message for the receiving viewer: an unedited room-level post with no attachments, link preview, reactions, pin, or thread. Clients can then show the message without `BatchGetMessages`. `@chatto/client` does this by itself. Clients that do not read the field continue to work as before.
