# FDR-011: Account Profile

**Status:** Experimental
**Last reviewed:** 2026-08-21

## Overview

An Authling account has a small, user-editable profile containing a preferred
username and optional full name. These are non-unique identity hints for
authorized relying parties, not Authling login identifiers or application
profile data.

## Behavior

- Verified signup derives the initial preferred username from the normalized
  email local part, limits it to 32 characters, and falls back to `user` when
  fewer than two suitable characters remain.
- A signed-in person can edit the profile at `/account/profile`. Preferred
  usernames contain 2 through 64 characters; full names are optional and
  contain at most 128 characters. Neither field is unique.
- Saving replaces both fields atomically. It does not change the account ID,
  email credential, password, browser sessions, authentication version, or
  stable OIDC `sub`.
- ID tokens and UserInfo publish non-empty values as the standard
  `preferred_username` and `name` claims. Relying parties must not treat either
  value as a stable identifier.

## Security and storage

- Profile fields are separately authenticated-encrypted with the account's
  credential data key. Durable events contain only opaque key references,
  envelope metadata, nonces, and ciphertext.
- The projection retains encrypted fields and decrypts them only at the
  account-service read boundary.
- Profile updates use account-subject optimistic concurrency control. They do
  not create a uniqueness index or expose values in subjects, keys, URLs, or
  logs.
- Cross-origin form submissions are rejected and editing requires a valid
  Authling browser session.

## Product boundary

The profile is limited to portable identity-provider claims. Application
biographies, avatars, presence, preferences, and other generic profile data
belong to relying parties rather than Authling.

## Related

- **Signup:** [FDR-002](FDR-002-verified-email-signup.md)
- **Sessions:** [FDR-003](FDR-003-local-login-and-browser-sessions.md)
- **OpenID Connect:** [FDR-004](FDR-004-openid-connect-provider.md)
