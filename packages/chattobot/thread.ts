import { createChattoClient } from '@chatto/client';
import { readBotThread, type BotThreadMessage as ThreadMessage } from '@chatto/bot-client';
import type { Delivery } from './chatto/routing.ts';

export type { BotThreadMessage as ThreadMessage } from '@chatto/bot-client';
export type ReadThread = (delivery: Delivery, signal: AbortSignal) => Promise<ThreadMessage[]>;

/** Adapt the application's webhook-compatible delivery to a client thread location. */
export function threadLocation(delivery: Delivery) {
  return { roomId: delivery.room_id, threadRootId: delivery.thread_root_id ?? delivery.message.id };
}

/** Bind the shared history reader to this bot's connection. */
export function createThreadReader(serverUrl: string, apiKey: string, request = fetch): ReadThread {
  const client = createChattoClient({ serverUrl, apiKey, fetch: request });
  return (delivery, signal) =>
    readBotThread(client, delivery.bot_id, threadLocation(delivery), signal);
}

export const readChattoThread: ReadThread = (delivery, signal) => {
  const url = process.env.CHATTO_URL;
  const key = process.env.CHATTO_API_KEY;
  if (!url || !key) {
    throw new Error('Set CHATTO_URL and CHATTO_API_KEY before reading threads');
  }
  return createThreadReader(url, key)(delivery, signal);
};
