---
type: fix
---

Chatto no longer shows a "realtime minimum cursor expired" error when a room notice or another resource loads more than 15 minutes after it changed. Requests with an expired `Chatto-Realtime-Minimum-Cursor` now run at once instead of failing, in ConnectRPC and MCP. Room notices and the message timeline show a translated error when they cannot load, not the server error text.
