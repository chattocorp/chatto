---
type: fix
---

Queue registration, email verification, and password-reset mail for delivery in the background. Accepted mail survives restarts and retries temporary transport failures until the code or link expires. Email jobs appear in the Jobs page without addresses or message content.
