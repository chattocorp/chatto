---
type: fix
---

Security: when you sign in to another server, Chatto now confirms that the sign-in response comes from that server. Before, a malicious server could pass your sign-in to a different server where you have an account and get access to that account. Existing sessions stay signed in.

Action for self-hosters: clients with this fix can sign in to your server from the frontend of another server only when your server has `webserver.url` set. This also applies to a server that still runs 0.5, which starts without it in a minimal configuration. Set `webserver.url` to the public address that people use to add your server; you do not have to upgrade the server for this. Sign-in through a different address for the same server now fails, so tell members to add the server with that public address.
