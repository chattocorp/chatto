---
type: fix
---

Security: when you sign in to another server, the client now makes sure that the sign-in response comes from that server. Before, a malicious server could pass your sign-in to a different server where you have an account and get access to that account. Existing sessions stay signed in.

Upgrade action for operators: a server without `webserver.url` must run this release, or set `webserver.url`, before people can sign in to it from another server's client. A server that you added through an alternative address can refuse sign-in; add it again with its main address.
