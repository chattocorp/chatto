# FDR-015: Administration

**Status:** Experimental
**Last reviewed:** 2026-10-30

## Overview

Administrators can inspect registered accounts through a read-only directory.
Authling uses one administrator flag per account and an operator-controlled
email override. There are no roles, groups, or configurable permissions.

## Behavior

- An administrator sees an Administration link in the account-page header.
- The directory shows verified email, profile names, account ID, registration
  time, and the source of administrator access. Pages contain at most 50 users.
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
- The directory does not let administrators change users, administrator flags,
  or site settings. Administrator access has no effect on connected apps.

## Design decisions

### One administrator flag

**Decision:** All administrative pages use the same administrator check.
**Why:** This product does not yet need delegated administration.
**Tradeoff:** Administrators have equal access to the current admin surface.

### Operator access remains independent

**Decision:** The configured email list grants access without a stored assignment.
**Why:** Operators need an access path that browser-managed flags cannot remove.
**Tradeoff:** Operators must protect the listed mailboxes and maintain the list.

## Open questions

- Before adding flag-editing actions, define recent-authentication checks,
  last-administrator protection, and an audit record of the acting administrator.
- User suspension, account edits, and editable site settings remain future work.

## Related

- [FDR-002: Verified Email Signup](FDR-002-verified-email-signup.md)
- [FDR-007: Verified Email Change](FDR-007-verified-email-change.md)
- [FDR-013: Account Deletion](FDR-013-account-deletion.md)
- [ADR-003: Server-rendered UI](../adr/ADR-003-server-rendered-templ-ui.md)
