# Replies and Threads

## The Basics

- Users can post messages in rooms.
- Messages can optionally be in reply to another message, referencing them.
- Messages can be posted directly into the room, or into a thread (a collection of messages starting with a root message.)
- There is no 1:1 relationship between replies and threads. Purely from the perspective of the data model, messages can be in reply to other messages without being in a thread, or be in a thread without being in reply to any message.
- However, rooms can be configured to use the preferred threading model. Examples:
  - A room where anything goes (replies without threads, threads without replies, or replies in threads)
  - A room where threads are disabled, everything is flat, but replies are allowed (replies without threads)
  - A room where only certain users can post new messages into the room, but everyone can post replies into threads
  - And so on.
- Reply references are used purely as a way to link messages together; they do not impace the way messages are stored.

## UI

- When a message is in reply to another message, it shows a reference to the replied to message above the message content and header.
- When the user clicks this reference, they get transported to the referenced message, and the referenced message will be briefly highlighted.
- The reference byline includes the small version of the referenced message's author's avatar, their name, and a single-line truncated version of the referenced message's content.
- Clicking/long-tapping on the avatar or the name opens the user context menu/sheet.

## Permissions

Reply attribution is not separately gated — any user who can post a message in a given context can also use `inReplyTo`. The relevant permissions are:

- `message.post` — Controls posting (with or without `inReplyTo`) at the room level.
- `message.post-in-thread` — Controls all thread posting (starting new threads and replying in existing ones). This is a single permission (no separate "start thread" permission).
