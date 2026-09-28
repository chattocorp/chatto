import type { ChattoClient } from '@chatto/client';
import type { Delivery } from './chatto/routing.ts';

export type Acknowledge = (delivery: Delivery, signal?: AbortSignal) => Promise<void>;

/** React with eyes to the message that addressed the bot, not to its thread root.
 * The reaction is bot policy; transport belongs to the shared client. */
export function createEyesReaction(client: Pick<ChattoClient, 'addReaction'>): Acknowledge {
  return (delivery, signal) =>
    client.addReaction(delivery.room_id, delivery.message.id, 'eyes', signal);
}
