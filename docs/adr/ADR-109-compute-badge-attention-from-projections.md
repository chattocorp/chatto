# ADR-109: Compute Badge Attention from Projections

**Date:** 2026-09-27

## Status

Accepted. Amends [ADR-076](ADR-076-deterministic-notification-occurrences.md).

## Context

ADR-076 stored Badge attention as one latest-value marker for each user, room,
and thread in `RUNTIME_STATE`. The notification materializer wrote the marker
for every Badge recipient of every source. Room messages use Badge by default,
so each root message wrote one marker for each room member. The marker was
written even when the member already had unread Badge attention.

On a production server with rooms of up to 1,193 members, these writes added
about 150,000 to 220,000 `RUNTIME_STATE` stream sequences each day. The bucket
held 119,000 live keys across 14.9 million sequences. The sparse message
blocks slowed startup and increased memory while watchers loaded them.

The marker only answered one question: does this user have Badge attention in
this room or thread? The notification decision projection already consumes the
facts that answer it: messages, mentions, reactions, retractions, thread
follows, membership, notification policy, and RBAC.

## Decision

Compute Badge attention when it is read. Do not store per-user Badge state.

The notification decision projection keeps a Badge source index. It records
root messages, thread replies, and the sources that address one user: direct,
role, `@here`, and `@all` mentions, replies to the user's messages, reactions
to the user's messages, and the first reply in a thread that the user started.
It also records when each membership, account, universal room, and thread
follow began. It stores IDs as handles and keeps no message content.

A source gives a user Badge attention when all of these are true:

- The source's cause resolves to Badge under the user's current notification
  policy.
- The user can see the source now, with the same visibility rules as
  occurrences. A direct mention also accepts interaction-scoped read access.
- The source is after the start of the user's current membership. For a
  followed thread, it is also after the follow began.
- The read boundary of the source's room or thread scope does not cover it.
- The user did not create it. It is not retracted, and it is younger than 90
  days. An echo or historical import is never a source.

A room query includes the room's threads. The first reply in a thread counts
for the root author unless the author explicitly unfollowed the thread,
because posting that reply follows the thread for the author only afterwards.

Read boundaries remain in `RUNTIME_STATE`. Badge attention does not use the
visibility boundary. It uses current visibility only, so an unread message
counts again when the user can read it again. Recording a boundary for every
user who cannot see a source would reintroduce per-message writes.

The index keeps each list in stream order and drops sources older than 90
days when it appends to that list. A query scans each list from the newest
source. It stops at the first source that gives attention, or at the first
source at or below the membership start, the follow start, or the read
boundary of the list's scope.

Realtime hints remain content-free:

- For each Badge recipient of a new source, the materializer compares the
  attention without and with the source. It sends a hint only when the source
  turns attention on.
- A retraction compares each possible recipient's attention with the message
  counted as not retracted against the current attention. It hints the users
  whose attention ended. A reaction removal hints the message author if the
  author can see the room.
- A notification policy change compares the actor's room attention in the
  policy scope before and after the change and hints the changed rooms. A
  manual follow or unfollow hints the thread.
- A user-scoped visibility change hints the user's rooms. A room-scoped
  visibility change hints the room's members. Server-wide and room-group-wide
  changes send no hints; clients converge when they next read their rooms.
  Hints go only to users who belong to the room or can join it.

## Consequences

- A root message no longer writes one `RUNTIME_STATE` key for each room
  member. The materializer writes nothing for Badge.
- Badge attention follows current state. Changing a notification policy clears
  or restores attention at once. Unfollowing a thread clears its attention.
  Following a thread shows only replies posted after the follow.
- After a user regains read access, unread messages from before and during
  the loss can give attention again.
- `has_unread` evaluates in memory, in about one microsecond for a room on a
  production copy.
- The notification decision projection uses more memory. It keeps one compact
  record for every message, because a later reply or reaction needs the
  message's author. Its source lists keep the sources of the last 90 days. A
  periodic sweep removes expired sources from threads that get no new
  activity. On a production copy
  the projection grew from about 3 MB to about 12 MB. Its snapshot contract
  changes, so the first start cold-replays it.
- Existing `notification_unread_marker.*` keys are no longer read or written.
  They expire through their 90-day TTL.
