---
type: fix
---

Security: when you sign in to another server, Chatto now confirms that the sign-in response comes from that server. Before, a malicious server could pass your sign-in to a different server where you have an account and get access to that account. Sign-in now works only through a server's public address (`webserver.url`), so add a server again if you added it through a different address. Existing sessions stay signed in.
