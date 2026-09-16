# FDR-014: Site Identity

**Status:** Experimental
**Last reviewed:** 2026-09-16

## Overview

Operators give their account service a public name independent of the Authling
software name. People see the service they use, such as chatto.id, throughout
account pages and transactional emails.

## Behavior

- The operator can set a site name and an optional description through TOML or
  environment variables. Environment values override TOML values.
- An omitted or blank name uses the hostname from the configured public URL.
  The local listener default supplies the hostname during local development.
- Headers, page titles, account messages, verification emails, recovery emails,
  and email-change notices use the site name. The description appears on the
  home page and in page metadata. An empty description is omitted.
- Settings are plain text. Names allow up to 120 Unicode characters and
  descriptions up to 500, without control characters. Edge spaces are removed.
- Changes take effect after a restart. They do not change issuer identity,
  account IDs, connected-app names, credentials, or the SMTP sender address.
- Short forms use a compact layout. Account settings separate profile,
  security, connected apps, sessions, and account deletion. Light and dark
  appearance, keyboard navigation, and use without JavaScript remain supported.

## Design decisions

### Site identity is display text

**Decision:** Configure the service name and description, but keep action labels
and security explanations in the application.
**Why:** People need a consistent identity and clear instructions throughout a
sign-in flow. Display changes must not change protocol identity.
**Tradeoff:** Operators cannot replace every sentence or supply custom HTML.

### No new external assets

**Decision:** Retain the self-hosted font and icons.
**Why:** Branding must not cause browsers to contact a third-party asset host or
reveal a user's IP address to one.
**Tradeoff:** Custom logos, fonts, and themes are outside this feature.

## Related

- [FDR-002: Verified Email Signup](FDR-002-verified-email-signup.md)
- [FDR-004: OpenID Connect Provider](FDR-004-openid-connect-provider.md)
- [FDR-013: Account Deletion](FDR-013-account-deletion.md)
