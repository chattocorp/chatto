import type { Api, MessagingRequests, ThreadLocation } from '@chatto/client';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { IncomingMessage } from './chatto/message.ts';

export type Acknowledge = (message: IncomingMessage, signal?: AbortSignal) => Promise<void>;

/** Set or clear only the bot's handled reaction on a host-selected thread root. */
export type SetHandled = (
  thread: ThreadLocation,
  handled: boolean,
  signal: AbortSignal
) => Promise<void>;

/** Reactions are idempotent and scoped to the authenticated bot by the server. */
export function createHandledReaction(client: Pick<Api, 'service'>): SetHandled {
  const messages = client.service(MessageService);
  return async ({ roomId, threadRootId }, handled, signal) => {
    const input = { roomId, messageEventId: threadRootId, emoji: 'white_check_mark' };
    if (handled) await messages.addReaction(input, { signal });
    else await messages.removeReaction(input, { signal });
  };
}

/** React with eyes to a message that addressed the bot, to show that the bot read it.
 * The reaction is bot policy; transport belongs to the shared client. */
export function createEyesReaction(client: Pick<MessagingRequests, 'addReaction'>): Acknowledge {
  return (message, signal) =>
    client.addReaction({ roomId: message.roomId, messageId: message.id }, 'eyes', { signal });
}
