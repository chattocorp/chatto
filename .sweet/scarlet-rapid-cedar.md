---
type: fix
---

Use cacheable HTTP GET requests for public server discovery in the client and bundled frontend. Discovery omits credentials and referrers. Chatto 0.4 servers are no longer discoverable; upgrade them to Chatto 0.5.0 or later. The existing minimum connectable server version, 0.5.0-beta.9, is unchanged.
