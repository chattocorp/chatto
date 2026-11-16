---
type: feature
---

Configured OIDC clients can register plain-HTTP redirect URIs on loopback hosts (`localhost`, `*.localhost`, `127.0.0.0/8`, and `[::1]`), such as `http://localhost:5173/auth/callback`, on any issuer. Before, Authling accepted these only when its own public URL was a loopback address. A local development server can now sign in against a production issuer without local TLS. Redirect URI matching stays exact, including the port. CIMD redirect rules do not change.
