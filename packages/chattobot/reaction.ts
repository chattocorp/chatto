import type { MessagingRequests } from '@chatto/client';
import type { Delivery } from './chatto/routing.ts';

export type Acknowledge = (delivery: Delivery, signal?: AbortSignal) => Promise<void>;

/** React with eyes to the message that addressed the bot, not to its thread root.
 * The reaction is bot policy; transport belongs to the shared client. */
export function createEyesReaction(client: Pick<MessagingRequests, 'addReaction'>): Acknowledge {
  return (delivery, signal) =>
    client.addReaction({ roomId: delivery.room_id, messageId: delivery.message.id }, 'eyes', {
      signal
    });
}
