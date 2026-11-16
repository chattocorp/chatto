# FDR-015: Administration

**Status:** Experimental
**Last reviewed:** 2026-11-16

## Overview

Administrators can inspect registered accounts through a read-only directory
and edit public site text in Appearance.
Authling uses one administrator flag per account and an operator-controlled
email override. There are no roles, groups, or configurable permissions.

## Behavior

- An administrator sees an Administration link in the account-page header.
- The directory shows verified email, profile names, account ID, registration
  time, and the source of administrator access. The directory loads 50 users
  at a time. When the end of the table scrolls into view, the next 50 users
  load below it. Without JavaScript, a link at the end of the table opens the
  next 50 users.
- Each column header sorts the directory. The first page shows the newest
  registrations first. Select a header to sort by that column; select it again
  to reverse the order. Registration starts with the newest, other columns in
  ascending order. Email and name sorting ignore case. The name column uses the
  full name, or the preferred username if there is no full name. Access sorting
  puts administrators first. Account ID breaks ties.
- The sort, order, and page number are URL query parameters. The URL contains
  no personal data. Unknown values return an error.
- The directory uses one compact line per user. Account IDs are truncated in
  their column; the full value remains selectable and available in its title.
  Registration times and configured-access details appear in cell titles.
  On narrow screens, the table scrolls horizontally by pointer or keyboard
  without widening the page.
- The header logo returns signed-in users to their account page. It does not
  sign them out. Signed-out visitors go directly to sign-in.
- Signed-out users must sign in. Other users cannot read the directory.
- Deleted accounts do not appear. Directory responses must not be cached.
- The stored administrator flag defaults to false for existing and new users.
- An account whose current verified email matches the configured administrator
  list has access even when its stored flag is false. Matching follows the
  account email comparison rules. The interface identifies this override.
- The operator can set the list in TOML or through an environment override.
  Changes require a restart on every replica. The browser cannot change it.
- Changing away from a configured email removes the override. A new account
  that verifies that address receives it. The override follows mailbox control,
  not a permanent account ID. It does not bypass authentication or deletion.
- The Users and Appearance sections share navigation. Appearance provides
  editing and saving for site name, description, and sign-in introduction, as
  specified in FDR-014. Only current administrators can edit or save.
- The directory does not let administrators change users or administrator flags.
  Administrator access has no effect on connected apps.

## Design decisions

### One administrator flag

**Decision:** All administrative pages use the same administrator check.
**Why:** This product does not yet need delegated administration.
**Tradeoff:** Administrators have equal access to the current admin surface.

### Operator access remains independent

**Decision:** The configured email list grants access without a stored assignment.
**Why:** Operators need an access path that browser-managed flags cannot remove.
**Tradeoff:** Operators must protect the listed mailboxes and maintain the list.

### Page numbers instead of cursors

**Decision:** The directory uses page numbers, not cursors.
**Why:** A cursor for email or name order would put personal data in the URL.
**Tradeoff:** Registrations and deletions while an administrator scrolls
shift later pages. The browser drops loaded rows for users that already show.
A user can be skipped when an account is deleted until the page reloads.

### Sort encrypted columns on request

**Decision:** Email and name sorting decrypt that field for every account on
each request. Other sorts decrypt only the selected page.
**Why:** Authling stores no plaintext or order-revealing index of personal data.
**Tradeoff:** Email and name sorting cost increases with the number of accounts.

## Open questions

- Before adding flag-editing actions, define recent-authentication checks,
  last-administrator protection, and an audit record of the acting administrator.
- User suspension and account edits remain future work.

## Related

- [ADR-009: Live Presentation Settings](../adr/ADR-009-live-presentation-settings.md)
- [FDR-014: Site Identity](FDR-014-site-identity.md)
- [FDR-002: Verified Email Signup](FDR-002-verified-email-signup.md)
- [FDR-007: Verified Email Change](FDR-007-verified-email-change.md)
- [FDR-013: Account Deletion](FDR-013-account-deletion.md)
- [ADR-003: Server-rendered UI](../adr/ADR-003-server-rendered-templ-ui.md)
