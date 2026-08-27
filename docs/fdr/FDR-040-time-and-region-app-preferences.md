# FDR-040: Time & Region (App Preferences)

**Status:** Active
**Last reviewed:** 2026-08-27

## Overview

Users can pick a display timezone and a 12/24-hour time format for the Chatto
app. These choices shape how every timestamp in the interface is shown. They
are application preferences that live on the device, not per-server account
settings.

## Behavior

- The App Preferences area of the app has a "Time & Region" pane with:
  - an IANA timezone selector, with an option to follow the device timezone;
  - a 12-hour or 24-hour clock option, with an option to follow the device
    setting.
- The chosen timezone and clock format apply to every connected server in
  this browser (or desktop app). The user does not set them again per server.
- All time rendering uses these preferences: message times, date separators,
  search results, admin views, and rendered inline timestamp tokens.
- Users who never open the pane see timestamps formatted for their device
  timezone and the device clock convention.
- Server-specific settings pages no longer show time or region options.

## Design Decisions

### 1. Display preferences are app-local

**Decision:** Timezone and clock format are stored locally by the app, like
theme, language, and composer settings.
**Why:** A user sits in one timezone at any moment, no matter how many servers
they use. All consumers of these values are client-side; the server never
formats dates for this user. Storing them per server caused needless
per-server setup and duplicated state. Locale already follows this pattern.
**Tradeoff:** Users must repeat their choice on each device. This matches how
language selection already works.

### 2. No push of time preferences to servers

**Decision:** The app does not send its time preferences to servers at
connect time.
**Why:** No current server feature consumes a member's display timezone. A
push channel would add API surface, conflict rules between devices, and sync
state without a consumer. If a future feature needs server-side knowledge of
a member's timezone, we will design a focused mechanism then.
**Tradeoff:** Such future features cannot rely on this data being present.

### 3. Device defaults when unset

**Decision:** An unset preference means "follow the device": the device IANA
timezone and the device 12/24-hour setting.
**Why:** Most users never change these values; the device default is nearly
always correct and needs no configuration step.
**Tradeoff:** Travelers who keep home-timezone conventions must pick an
explicit timezone instead of relying on their old per-server setting.

### 4. No upgrade path from per-server values

**Decision:** We do not read time settings from pre-0.5 servers during
upgrade.
**Why:** The large majority of users kept default settings, so their effective
behavior stays the same. Shipping a one-time read-and-copy path for a small
number of non-default users is not worth the complexity while the public API
may still change before 0.5.
**Tradeoff:** Non-default users re-select their timezone once after upgrade.

## Related

- **ADRs:** none directly.
- **FDRs:** FDR-030 (inline timestamp tokens render with these preferences)
