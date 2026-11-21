---
type: feature
---

When a server offers SSO providers, the sign-in page now shows only the provider buttons. If password login is also enabled, a **Continue with password** button opens a separate page with the password form, password recovery, and **Create Account**. Servers without SSO providers keep the previous sign-in page.

Direct registration now requires direct login. If `auth.direct_login` is `false`, Chatto disables direct registration and logs a warning at startup when `auth.direct_registration` is still `true`. Before, this combination let users create password accounts that could not sign in.
