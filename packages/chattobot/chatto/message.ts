/** An addressed Chatto message or accepted report, as the conversation task receives it. */
import { Type, type Static } from 'runling';
import type { AddressedMessage, Destination, ThreadLocation, RealtimeEvent } from '@chatto/client';

export const IncomingMessage = Type.Object({
  id: Type.String(),
  roomId: Type.String(),
  /** The thread root. A message at the room level starts its own thread. */
  threadRootId: Type.String(),
  /** The server-authenticated author. Never a display name. */
  authorId: Type.String(),
  body: Type.String(),
  /** Host connection identity. Used to bind recovered workflows to the same server and bot. */
  connection: Type.Optional(Type.String()),
  /** Host-owned provenance. Absent means an addressed message for existing callers. */
  trigger: Type.Optional(Type.Union([Type.Literal('intake'), Type.Literal('follow-up')]))
});
export type IncomingMessage = Static<typeof IncomingMessage>;

/** Recovery opens the existing session and waits for new input; it never repeats this message. */
export const ConversationStart = Type.Intersect([
  IncomingMessage,
  Type.Object({
    resume: Type.Optional(Type.Boolean()),
    /** The complete first addressed batch, including this message, with each author's provenance. */
    messages: Type.Optional(Type.Array(IncomingMessage, { minItems: 1 }))
  })
]);

/** Convert an addressed message from the realtime client. */
export function incomingMessage(message: AddressedMessage): IncomingMessage {
  return {
    id: message.id,
    roomId: message.roomId,
    threadRootId: message.threadRootId ?? message.id,
    authorId: message.authorId,
    body: message.body
  };
}

/** The thread of a message. */
export const threadOf = (message: IncomingMessage): ThreadLocation => ({
  roomId: message.roomId,
  threadRootId: message.threadRootId
});

/** A reply in the message's thread that references the message. */
export const replyTo = (message: IncomingMessage): Destination => ({
  ...threadOf(message),
  inReplyTo: message.id
});

/** What the parent sends to a conversation: one message or a batch of addressed or accepted replies.
 * Each message retains its authenticated author and host-owned provenance. */
export const ConversationInput = Type.Union([
  IncomingMessage,
  Type.Array(IncomingMessage, { minItems: 1 })
]);
export type ConversationInput = Static<typeof ConversationInput>;

/** Opted-in room-level posts only. Replies, echoes, shredded messages, and own posts cannot start intake. */
export function intakeMessage(
  event: RealtimeEvent,
  rooms: ReadonlySet<string>,
  botId: string
): IncomingMessage | undefined {
  if (
    event.event.case !== 'messagePosted' ||
    !event.id ||
    !event.actorId ||
    event.actorId === botId
  )
    return;
  const post = event.event.value;
  if (
    !rooms.has(post.roomId) ||
    post.threadRootEventId ||
    post.inReplyTo ||
    post.echoOfEventId ||
    post.bodyPlaintext === undefined
  )
    return;
  return {
    id: event.id,
    roomId: post.roomId,
    threadRootId: event.id,
    authorId: event.actorId,
    body: post.bodyPlaintext,
    trigger: 'intake'
  };
}

/** A realtime reply can wake an existing thread workflow. It is never a request to the
 * conversation agent: the workflow reads and classifies the current thread first. */
export function followUpMessage(event: RealtimeEvent, botId: string): IncomingMessage | undefined {
  if (
    event.event.case !== 'messagePosted' ||
    !event.id ||
    !event.actorId ||
    event.actorId === botId
  )
    return;
  const post = event.event.value;
  if (!post.threadRootEventId || post.echoOfEventId || post.bodyPlaintext === undefined) return;
  return {
    id: event.id,
    roomId: post.roomId,
    threadRootId: post.threadRootEventId,
    authorId: event.actorId,
    body: post.bodyPlaintext,
    trigger: 'follow-up'
  };
}
