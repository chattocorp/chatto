---
type: feature
---

People can now sign in with Bluesky and other AT Protocol accounts, and show their verified internet handle on their profile.

- Operators enable sign-in with a new `atproto` login provider type. It needs no client registration or secret. See the login providers guide.
- In account settings, people can connect their internet handle. Chatto writes a link record to their AT Protocol account, and other members' apps verify the handle before they show it.
- The `User` resource has a new `atproto_did` field. `MyAccountService` has new `SetAtprotoIdentity` and `ClearAtprotoIdentity` RPCs.
