import {
  readBotThread,
  type BotClient,
  type BotThreadMessage as ThreadMessage
} from '@chatto/bot-client';
import type { Delivery } from './chatto/routing.ts';

export type { BotThreadMessage as ThreadMessage } from '@chatto/bot-client';
export type ReadThread = (delivery: Delivery, signal: AbortSignal) => Promise<ThreadMessage[]>;

/** Adapt the application's webhook-compatible delivery to a client thread location.
 * A root message, including a DM root, starts its own thread. */
export function threadLocation(delivery: Delivery) {
  return { roomId: delivery.room_id, threadRootId: delivery.thread_root_id ?? delivery.message.id };
}

/** Read a delivery's complete thread with bot/human roles for this bot identity. */
export function createThreadReader(
  client: Pick<BotClient, 'readThread'>,
  botId: string
): ReadThread {
  return (delivery, signal) => readBotThread(client, botId, threadLocation(delivery), signal);
}
