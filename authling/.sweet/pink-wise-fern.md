---
type: fix
---

Queue signup, password-reset, email-change codes, and old-address security notices for delivery in the background. Accepted mail survives restarts and retries SMTP failures without blocking the request. Code delivery stops at the original expiry; security notices retry for up to 24 hours.
