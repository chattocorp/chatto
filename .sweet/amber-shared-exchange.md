---
type: fix
---

`@chatto/client/auth/oauth` now refuses an authorization callback without the `iss` parameter of the selected server. It also exports `randomOAuthValue`, `pkceCodeChallenge`, `isExpectedOAuthIssuer`, and `exchangeAuthorizationCode` for hosts that use their own callback.
