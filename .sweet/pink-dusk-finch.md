---
type: breaking
---

Require webserver.url for every server configuration. Before upgrading, set webserver.url or CHATTO_WEBSERVER_URL to the public HTTP or HTTPS URL of the server. Chatto rejects missing or blank values at startup.
