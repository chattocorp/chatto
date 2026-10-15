import type { MessagingRequests } from '@chatto/client';
import type { IncomingMessage } from './chatto/message.ts';

export type Acknowledge = (message: IncomingMessage, signal?: AbortSignal) => Promise<void>;

/** React with eyes to a message that addressed the bot, to show that the bot read it.
 * The reaction is bot policy; transport belongs to the shared client. */
export function createEyesReaction(client: Pick<MessagingRequests, 'addReaction'>): Acknowledge {
  return (message, signal) =>
    client.addReaction({ roomId: message.roomId, messageId: message.id }, 'eyes', { signal });
}
