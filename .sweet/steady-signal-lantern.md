---
type: fix
---

Fixed network errors on a weak connection that needed a manual "Try Again" and showed no connection warning. Chatto now repeats failed reads, parts that failed to load try again without the user, and the header shows the connection warning of the current server when requests fail.
