# ADR-100: Include Current State in Frequent Realtime Updates

**Status:** Accepted

**Date:** 2026-09-20

## Context

A message post changes a message and can change the poster's room attention
and read state. The frontend already shares one authoritative message read.
However, content-free room and notification hints also cause complete room
directory and notification reads. A Badge change does not necessarily change
a notification occurrence.

## Decision

Extend protocol 4 event payloads with optional canonical current state for
these frequent updates. Message posts, room-read changes, and notification
Badge changes can carry the affected `RoomWithViewerState`. Notification
occurrence changes can carry the first 50 occurrences and complete attention
counts, using `ListNotificationOccurrencesResponse`.

The server assembles these values for the authenticated viewer in delivery
order. It waits for the relevant current projection and runtime-state
boundaries and applies the same visibility rules as resource reads. It does
not publish viewer resources into EVT. Delayed hints resolve current values,
not the values that existed when another replica published the hint.

These fields are a narrow exception to using only resource IDs in change
hints. They do not introduce a general resource-update frame or replace
ConnectRPC pagination. The event cursor still identifies the durable source
boundary; nested resources describe current state at delivery.

Clients merge room resources by ID. A notification page replaces the retained
page and counts. A Badge event does not require a notification read; actual
occurrence changes have their own event. Clients that receive no optional
state use explicit reads. Existing clients can ignore the fields and continue
to read resources. There is no protocol-version or persisted-data migration.

Transient updates remain best effort. Clients reconcile current state after
reconnect, including successful resume. The bundled frontend retains its
thread-read acknowledgement recovery reads. Local delivery versions prevent
an older HTTP response from replacing state supplied by a newer event. These
local versions are not public cursors or broker coordinates.

## Consequences

A warm room post needs the message command and one shared message read. It
does not reload the room directory or notification list. Text posts that do
not change occurrences do not assemble notification pages.

The server does more work before delivering selected events. Room work is
limited to one room, and notification hydration is limited to 50 rows. Complete
attention counts retain the existing list API cost. Failed hydration leaves
the optional state absent so the client can use its existing retry path.

Reconnect, authorization, expiry, pagination, and overlapping HTTP reads must
remain covered by tests. No new external service receives user data.
