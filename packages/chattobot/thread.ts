import type { ChattoClient } from '@chatto/client';
import { readBotThread, type BotThreadRead } from '@chatto/bot-client';
import type { Delivery } from './chatto/routing.ts';

export type {
  BotThreadMessage as ThreadMessage,
  BotThreadRead as ThreadRead
} from '@chatto/bot-client';

/** Read a delivery's thread: without `after`, the root and the newest replies; with `after`, a
 * cursor from an earlier read, only newer messages. Messages carry bot/human roles and the
 * authors' names. */
export type ReadThread = (
  delivery: Delivery,
  signal: AbortSignal,
  after?: string
) => Promise<BotThreadRead>;

/** Adapt the application's webhook-compatible delivery to a client thread location.
 * A root message, including a DM root, starts its own thread. */
export function threadLocation(delivery: Delivery) {
  return { roomId: delivery.room_id, threadRootId: delivery.thread_root_id ?? delivery.message.id };
}

/** Read a delivery's thread with bot/human roles for this bot identity. */
export function createThreadReader(
  client: Pick<ChattoClient, 'readThread'>,
  botId: string
): ReadThread {
  return (delivery, signal, after) =>
    readBotThread(client, botId, threadLocation(delivery), signal, after ? { after } : undefined);
}
