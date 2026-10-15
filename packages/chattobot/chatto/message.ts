/** A Chatto message addressed to the bot, as the conversation task receives it. */
import { Type, type Static } from 'runling';
import type { AddressedMessage, Destination, ThreadLocation } from '@chatto/client';

export const IncomingMessage = Type.Object({
  id: Type.String(),
  roomId: Type.String(),
  /** The thread root. A message at the room level starts its own thread. */
  threadRootId: Type.String(),
  /** The server-authenticated author. Never a display name. */
  authorId: Type.String(),
  body: Type.String()
});
export type IncomingMessage = Static<typeof IncomingMessage>;

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

/** What the router sends to a running conversation: authenticated addressed messages. */
export const ConversationInput = IncomingMessage;
export type ConversationInput = Static<typeof ConversationInput>;
