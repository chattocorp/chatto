---
type: feature
---

People can now sign in with Bluesky and other AT Protocol accounts, and their linked handle is shown as verified on their profile.

- Operators enable sign-in with a new `atproto` login provider type. It needs no client registration or secret, and the server's `webserver.url` must use HTTPS without a port. See the login providers guide.
- The app signs in to the person's AT Protocol server once per device. It then signs in to every Chatto server with that identity, usually with one click. Chatto servers get no access to the person's data.
- A linked identity is published on the profile. The app writes a link record to the person's AT Protocol account, and other members' apps verify the handle before they show it.
- The `User` resource has a new `atproto_did` field, and `StartExternalIdentityLinkResponse` has a new `link_token` field for AT Protocol links.
