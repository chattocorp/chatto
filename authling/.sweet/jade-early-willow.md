---
type: fix
---

Accept CIMD client IDs whose host is a public IPv4 address. Authling rejected these URLs before, because it classified the address in its IPv4-mapped IPv6 form.
