---
type: upgrade
---

If your integration handles its own OAuth callback with `@chatto/client/auth/oauth`, check the authorization response’s `iss` parameter with `isExpectedOAuthIssuer` before calling `exchangeAuthorizationCode`. Stop the sign-in if the issuer is missing or does not match the selected server.
