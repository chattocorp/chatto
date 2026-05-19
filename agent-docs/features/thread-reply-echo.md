# Thread Reply Echo ("Also Send to Channel")

## Overview

When posting a reply in a thread, users can optionally echo that reply to the main channel for visibility. This is similar to Slack's "Also send to #channel" feature. The echo appears as a regular `MessagePostedEvent` in the room timeline (with `echoOfEventId` set) while sharing the same message body as the original thread reply.

## Data Model

- An echo is a `MessagePostedEvent` with `echoOfEventId` and `threadRootEventId` set (regular messages have these fields empty).
- The echo is published to the room's root message subject (making it visible in `GetRoomEvents`).
- The echo shares the same `messageBodyId` as the original thread reply. This means edits propagate to both the original and the echo automatically.
- The echo carries:
  - `echoOfEventId`: The event ID of the original thread reply.
  - `threadRootEventId`: The thread root event ID, used for "View thread" navigation.
  - `inReplyTo`: The event ID of the message the original reply was responding to (for reply attribution). May be empty if the thread reply was not in reply to a specific message.
  - `mentionedUserIds`: Copied from the original thread reply for independent notification delivery.
- Reactions are independent per event (the echo and the original have different event IDs, so they accumulate reactions separately).
- The echo does NOT increment `reply_count` on the thread root message, because it represents the same reply, not an additional one.

## Permissions

- **`message.echo`**: Gates the "Also send to channel" feature. Scoped at the server-role and per-room levels. Granted to the `everyone` role by default. Checked in the GraphQL resolver when `alsoSendToChannel` is true on a `PostMessage` mutation. The `Room` type exposes `viewerCanEchoMessage` for frontend permission gating.
- **`message.post-in-thread`**: Required for the thread reply itself. This is the single permission for all thread posting (no separate "start thread" permission), and it covers replies with `inReplyTo` attribution as well.

## Backend

- The `PostMessage` core function accepts an `alsoSendToChannel bool` parameter.
- When `inThread` is set and `alsoSendToChannel` is true:
  1. The original thread reply is published to the thread subject as usual.
  2. A second `MessagePostedEvent` is created with `echoOfEventId` and `threadRootEventId` set, and published to the root message subject (`space.{s}.room.{r}.msg.{echoEventId}`).
  3. Mention notifications fire independently for the echo event.
- If the echo publish fails, a warning is logged but the original thread reply still succeeds (best-effort).
- `alsoSendToChannel` without `inThread` is rejected with an error.

## Frontend

### Identifying Echoes

Echoes are identified by checking `echoOfEventId != null` on a `MessagePostedEvent`. There is no separate event type — echoes are just a "flavor" of posted message.

### Composer

- The thread pane's `MessageComposer` shows an "Also send to channel" checkbox when the user has `message.echo` permission.
- The checkbox state (`alsoSendToChannel`) is included in the `PostMessage` mutation input.
- The checkbox resets to unchecked after each successful send.
- The main room's composer never shows this checkbox (it only makes sense in a thread context).

### Rendering in Room Timeline

- Echo events are rendered using the same `MessageEvent` component as regular messages, with `isEcho` derived from `echoOfEventId != null`.
- Below the message content, a "Thread" button indicator is shown. Clicking it opens the thread pane for the echo's `threadRootEventId`.
- If the echo has `inReplyTo` data, the reply attribution header ("in reply to [author] [excerpt]") is shown, just like for regular reply messages.
  - Clicking the reply attribution on an echo opens the thread and highlights the referenced message within it (via `?highlight=eventId` query param on the thread URL).
- Echoes with `inReplyTo` break message grouping (they always render with full avatar/name header to provide reply attribution context).

### Rendering in Thread Pane

- The original thread reply (not the echo) appears in the thread pane. The echo is a room-level event and does not appear in the thread.
- Reply attribution in threads scrolls within the thread via a thread-scoped `JumpToMessageState`.

### Real-time Updates

- Echo events arrive via the `mySpaceEvents` WebSocket subscription as `MessagePostedEvent` with `echoOfEventId` set.
- Edit/delete propagation is automatic. When a `MessageUpdatedEvent` or `MessageDeletedEvent` arrives with a `messageEventId`, the frontend matches it against loaded events by `e.id` (direct match) or `echoOfEventId` (echo match) and refetches them.

### Message Grouping

- Echoes are `MessagePostedEvent` events and follow the same grouping rules (same actor within 10 minutes can be grouped).
- Echoes with `inReplyTo` always break groups (render full header) to show the reply attribution context.

## Protobuf

Echoes use `MessagePostedEvent` (defined in `proto/chatto/core/v1/space_event.proto`) with additional echo fields:

| Field | Tag | Type | Description |
|-------|-----|------|-------------|
| `echo_of_event_id` | 7 | string | Event ID of the original thread reply (empty = not an echo) |
| `thread_root_event_id` | 8 | string | Thread root for navigation (empty = not an echo) |

All other `MessagePostedEvent` fields (`space_id`, `room_id`, `message_body_id`, `in_reply_to`, `mentioned_user_ids`) are shared with regular messages.

## GraphQL

Echoes are `MessagePostedEvent` in the `SpaceEventType` union with nullable echo fields:

- `echoOfEventId: ID` — null for regular messages, set for echoes
- `threadRootEventId: ID` — null for regular messages, set for echoes
- All other `MessagePostedEvent` fields are available (body, attachments, reactions, etc.)
- `PostMessageInput` includes `alsoSendToChannel: Boolean`
- `Room` type includes `viewerCanEchoMessage: Boolean!`
