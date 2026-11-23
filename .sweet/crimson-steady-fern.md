---
type: feature
---

On SSO-only servers, linking or disconnecting a sign-in provider after the 30-minute recent sign-in window no longer ends in an error. Chatto explains that you must first confirm it is you, asks you to sign in again with a linked OpenID Connect provider, and then continues the link by itself, or shows the disconnect confirmation again. `ExternalIdentityProvider.reverification_available` reports which linked providers support this for the signed-in browser.
