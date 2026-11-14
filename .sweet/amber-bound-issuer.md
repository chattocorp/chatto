---
type: fix
---

Security: when you sign in to another server, the client now makes sure that the sign-in response comes from that server. Before, a malicious server could pass your sign-in to a different server where you have an account and get access to that account. A server that you added through an alternative address can refuse sign-in now; add it again with its main address.
