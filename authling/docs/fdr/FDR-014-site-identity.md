# FDR-014: Site Identity

**Status:** Experimental
**Last reviewed:** 2026-10-30

## Overview

Operators give their account service a public name independent of the Authling
software name. People see the service they use, such as chatto.id, throughout
account pages and transactional emails.

## Behavior

- The operator can set default site name, description, and sign-in introduction
  through TOML or environment variables. Environment values override TOML.
- Administrators can edit these values in Administration → Appearance. Preview
  updates the sign-in card as they type, without requests or publishing changes.
  Without JavaScript, a Preview button renders the draft. Save applies the text to
  subsequent pages and emails without a restart, including on other replicas.
- Saved settings override display defaults and survive restarts. Concurrent
  edits produce a conflict instead of overwriting another administrator's work.
  Failed saves keep submitted text visible. The editor works without JavaScript.
- Each save retains the acting administrator, time, and replacement settings in
  the audit history. Preview does not create an audit entry. There is no history
  viewer in the browser yet.
- An omitted or blank name uses the hostname from the configured public URL.
  The local listener default supplies the hostname during local development.
- Headers, page titles, account messages, verification emails, recovery emails,
  and email-change notices use the site name. The description appears in page
  metadata. An empty description is omitted.
- The root URL redirects signed-out visitors to sign-in and active sessions to
  their account. There is no separate welcome page.
- Settings are plain text. Names allow up to 120 Unicode characters; descriptions
  and introductions allow up to 500 each, without control characters or line
  breaks. Edge spaces are removed. The editor requires a nonblank name. A blank
  introduction uses the standard sign-in instruction.
- Changes do not change issuer identity,
  account IDs, connected-app names, credentials, or the SMTP sender address.
- Only sign-in and registration use centered cards. Account settings use
  URL-backed Profile, Security, Connected apps, and Sessions tabs. Account
  editing and recovery use full-width panels; deletion is under Security. Light and dark
  appearance, keyboard navigation, and use without JavaScript remain supported.

## Design decisions

### Site identity is display text

**Decision:** Allow administrators to edit public display text, but keep action
labels and security explanations in the application. Keep configuration changes
in the audit history described in ADR-009.
**Why:** People need a consistent identity and clear instructions throughout a
sign-in flow. Display changes must not change protocol identity.
**Tradeoff:** Operators cannot replace every sentence or supply custom HTML.

### No new external assets

**Decision:** Retain the self-hosted font and icons.
**Why:** Branding must not cause browsers to contact a third-party asset host or
reveal a user's IP address to one.
**Tradeoff:** Custom logos, fonts, and themes are outside this feature.

## Related

- [ADR-009: Live Presentation Settings](../adr/ADR-009-live-presentation-settings.md)
- [FDR-015: Administration](FDR-015-administration.md)
- [FDR-002: Verified Email Signup](FDR-002-verified-email-signup.md)
- [FDR-004: OpenID Connect Provider](FDR-004-openid-connect-provider.md)
- [FDR-013: Account Deletion](FDR-013-account-deletion.md)
