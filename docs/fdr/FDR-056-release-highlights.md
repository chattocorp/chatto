# FDR-056: Release Highlights

**Status:** Experimental
**Last reviewed:** 2026-11-10

## Overview

What's new gives users and administrators short, translated explanations of
changes that affect them. It connects release highlights to the Help guides.

## Behavior

- Help contains user highlights. Administration has a separate page for
  administrator and operator highlights. Administrators can also read user
  highlights.
- Include features, meaningful fixes, changed controls, security changes, and
  upgrade actions. Exclude refactoring, tests, dependency maintenance, and
  chores unless they have a concrete effect on the reader.
- A badge on the header Help action indicates applicable, undismissed user
  highlights. Selecting it opens the newest unread entry. With no unread
  highlights, the action opens the Help overview. Administration has its own
  indicator. No modal opens automatically.
- Opening the page does not dismiss entries. Readers can dismiss one entry or
  all currently unread entries. Dismissed entries remain in the history.
  Dismissals belong to the account on the selected server and synchronize
  across devices. They do not apply to accounts on other servers.
- The first visit records the newest applicable release for that surface.
  Earlier entries remain in history but do not count as unread. Later entries
  remain unread until dismissed, including when a reader skips releases.
- Highlights ship with the frontend. Server-dependent highlights appear only
  when the connected server meets their stated version requirement. Unknown
  versions do not prove that a requirement is met.
- A newer server can supply translated feature descriptions for an older
  frontend. The What's new page shows these as features available with a newer
  app, using the compatibility catalog from FDR-031. These descriptions are
  advisory: server configuration and permissions still control access.
- Older servers without dismissal support retain readable highlights. The UI
  explains that synchronized dismissal needs an updated server. This optional
  feature does not raise the minimum connectable server version.
- Reading bundled content makes no external service request. Compatibility
  descriptions and account state use the selected Chatto server. No release
  service receives the reader's IP address or account information.

## Design Decisions

### 1. Curated translated MDX

**Decision:** Author highlights separately from the generated changelog. Use
translated MDX compiled with the frontend, and link to Help for instructions.

**Why:** Readers need the effect of a change, not an implementation log. MDX
uses the same authoring and rendering tools as Help.

**Tradeoff:** Maintainers must curate and translate entries. An older frontend
cannot render newer MDX articles. The compatibility catalog can describe newer
features with plain text, without executing code from a connected server.

### 2. Stable identifiers and additive dismissal

**Decision:** Give each message a stable, surface-prefixed identifier. Save
acknowledgements as private account preferences. Concurrent dismissals merge.
Text corrections and translations preserve the identifier.

**Why:** A reader should not repeatedly dismiss an entry after a correction,
restart, or device change. Dismiss all must not dismiss entries that arrived
while the request was in flight. Durable preferences follow ADR-033.

**Tradeoff:** Acknowledgements require server support. Each request and the
stored acknowledgement set have explicit limits. Content is public product
information; dismissing an administrator entry does not grant administrator
access.

## Related

- **Issue:** [#3200](https://github.com/chattocorp/chatto/issues/3200)
- **FDRs:** [FDR-031](FDR-031-client-server-compatibility-discovery.md),
  [FDR-054](FDR-054-room-notices.md)
- **ADRs:** [ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md),
  [ADR-111](../adr/ADR-111-move-client-state-into-chatto-client.md)
