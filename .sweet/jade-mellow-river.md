---
type: fix
---

Standalone commands, including media processing, search, and backup, can start without `webserver.url`. The server still requires its public URL. The media worker again logs `Asset-processing worker started` when it finishes replaying asset events and connects to its job consumer, so existing startup probes can mark it ready.
