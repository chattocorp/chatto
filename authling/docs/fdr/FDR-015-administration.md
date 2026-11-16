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
- The directory shows a masked email, a masked name, account ID, registration
  time, and the source of administrator access. It never shows a verbatim
  email address or profile name. The masked email keeps the first character of
  the local part and of the domain name, and the top-level domain, for example
  `a•••@e•••.com`. The masked name keeps the first character of the full name,
  or of the preferred username if there is no full name. The mask length is
  fixed and does not show the length of the value.
- The directory loads 50 users
  at a time. When the end of the table scrolls into view, the next 50 users
  load below it. Without JavaScript, a link at the end of the table opens the
  next 50 users.
- The Access, Registered, and Account ID headers sort the directory. The first
  page shows the newest registrations first. Select a header to sort by that
  column; select it again to reverse the order. Registration starts with the
  newest, other columns in ascending order. Access sorting puts administrators
  first. Account ID breaks ties. Email and name do not sort.
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

### Mask personal data

**Decision:** The directory shows masked emails and names, and does not sort
by them.
**Why:** Administrators browse many accounts; they need to recognize a user,
not read personal data. Sorting by a hidden value shows its alphabetical order.
**Tradeoff:** Administrators cannot find a user by email or name in the
directory. A later control can show the data after additional verification.

### Page numbers instead of cursors

**Decision:** The directory uses page numbers, not cursors.
**Why:** One page parameter works for every sort order and contains no
personal data.
**Tradeoff:** Registrations and deletions while an administrator scrolls
shift later pages. The browser drops loaded rows for users that already show.
A user can be skipped when an account is deleted until the page reloads.

## Open questions

- Before adding flag-editing actions, define recent-authentication checks,
  last-administrator protection, and an audit record of the acting administrator.
- User suspension and account edits remain future work.
- Define which administrators can show unmasked user data, the additional
  verification they need, and the audit record of each disclosure.

## Related

- [ADR-009: Live Presentation Settings](../adr/ADR-009-live-presentation-settings.md)
- [FDR-014: Site Identity](FDR-014-site-identity.md)
- [FDR-002: Verified Email Signup](FDR-002-verified-email-signup.md)
- [FDR-007: Verified Email Change](FDR-007-verified-email-change.md)
- [FDR-013: Account Deletion](FDR-013-account-deletion.md)
- [ADR-003: Server-rendered UI](../adr/ADR-003-server-rendered-templ-ui.md)
