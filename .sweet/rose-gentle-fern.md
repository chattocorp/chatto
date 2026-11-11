---
type: feature
---

People can now sign in with Bluesky and other AT Protocol accounts, and their linked handle is shown as verified on their profile.

- Operators enable sign-in with a new `atproto` login provider type. It needs no client registration or secret. See the login providers guide.
- Chatto requests only the AT Protocol identity permission and gets no access to people's posts or other data.
- New accounts start with the display name and avatar of the person's public Bluesky profile.
- A linked identity is published on the profile. The person's app writes a link record to their AT Protocol account, and other members' apps verify the handle before they show it.
- The `User` resource has a new `atproto_did` field.
