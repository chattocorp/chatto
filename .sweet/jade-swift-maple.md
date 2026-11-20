---
type: fix
---

After account deletion, a background job removes the account's stored sessions and access tokens. Another server replica can finish interrupted cleanup. Administrators can see its status on the Jobs page.
