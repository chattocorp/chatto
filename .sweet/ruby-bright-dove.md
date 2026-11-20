---
type: feature
---

Member profile cards and full profiles show a green self-verification pill for each linked AT Protocol handle.

Integrations can request public profile identities with `UserService.GetUser` by setting `include_verified_identities`. Each entry in `verified_identities` includes its verification method, method label, and public value.
