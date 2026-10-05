# FDR-030: Inline Message Timestamps

**Status:** Active
**Last reviewed:** 2026-10-05

## Overview

Users can insert an absolute timestamp into a message so every reader sees the
same instant formatted for their own locale, timezone, and 12/24-hour
preference. This is for lightweight coordination in chat, not for scheduling
events or reminders.

## Behavior

- The bundled composer includes a timestamp picker with a date field, a time
  field, and an IANA timezone field.
- The picker time field uses the viewer's 12/24-hour preference. When the
  preference is automatic, it uses the same clock as rendered message times.
- Inserting a timestamp writes a stable text token into the message body:
  `<t:UNIX_SECONDS:F>`.
- Rendered messages replace valid timestamp tokens with localized date-time
  text using the viewer's display preferences.
- Rendered timestamps include a small clock icon and are clickable. Clicking
  opens a compact details popover showing local time and a live-updating
  relative time.
- Invalid timestamp tokens stay visible as literal message text.
- Timestamp tokens inside inline code, code blocks, and blockquotes stay
  literal.
- Editing a message exposes and preserves the raw timestamp token.
- Older clients that do not understand timestamp tokens show the raw token.

## Design Decisions

### 1. Timestamp tokens are body text

**Decision:** Inline timestamps are stored as message-body text instead of new
protobuf fields or structured message spans.
**Why:** Message bodies are already mutable, encrypted content. A text token is
compatible with old clients and does not change persisted EVT payloads or public
ConnectRPC message shapes.
**Tradeoff:** Integrations that want native timestamp rendering must understand
the token syntax themselves.

### 2. V1 supports one exact format

**Decision:** V1 supports `<t:UNIX_SECONDS:F>` only, rendered as an exact
localized date and time.
**Why:** The immediate use case is removing manual timezone conversions from
announcements. Exact date-time rendering avoids ambiguity in old threads.
**Tradeoff:** Compact and relative timestamp styles can be added later, but are
not part of the first supported syntax.

### 3. The picker time field is a custom control

**Decision:** The picker uses a native date input and a custom time field with
hour and minute segments, plus a day-period selector for the 12-hour clock.
**Why:** Native time inputs use the clock of the browser or operating system
locale. Pages cannot change this clock (whatwg/html#6698), so a native time
input ignores the viewer's 12/24-hour preference.
**Tradeoff:** The time field does not have the native time picker. Replace it
with a native input when browsers let pages set the clock.

## Permissions

No dedicated permission. Anyone who can post or edit a message can include a
timestamp token in that message body.

## Related

- **ADRs:** ADR-011, ADR-043, ADR-065
- **FDRs:** FDR-004, FDR-006
