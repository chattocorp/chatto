import type { ChattoClient, UserName } from '@chatto/client';
import { readBotThread, type BotThreadMessage } from '@chatto/bot-client';
import type { Delivery } from './chatto/routing.ts';

/** A thread message with its author's public names, when Chatto returns them. */
export interface ThreadMessage extends BotThreadMessage {
  /** Display name, or the login when no display name is set. */
  authorName?: string;
  authorLogin?: string;
}
export type ReadThread = (delivery: Delivery, signal: AbortSignal) => Promise<ThreadMessage[]>;

/** How long a looked-up name stays valid. Renamed users show their new name after this. */
const NAME_CACHE_MS = 10 * 60_000;

/** Adapt the application's webhook-compatible delivery to a client thread location.
 * A root message, including a DM root, starts its own thread. */
export function threadLocation(delivery: Delivery) {
  return { roomId: delivery.room_id, threadRootId: delivery.thread_root_id ?? delivery.message.id };
}

/** Read a delivery's complete thread with bot/human roles for this bot identity, and each
 * author's public display name and login. A failed name lookup leaves the names out. */
export function createThreadReader(
  client: Pick<ChattoClient, 'readThread' | 'getUserNames'>,
  botId: string,
  now = Date.now
): ReadThread {
  const names = new Map<string, { name: UserName; at: number }>();
  return async (delivery, signal) => {
    const messages = await readBotThread(client, botId, threadLocation(delivery), signal);
    const authors = [...new Set(messages.map((message) => message.authorId))].filter(
      (id): id is string => !!id
    );
    const missing = authors.filter((id) => {
      const cached = names.get(id);
      return !cached || now() - cached.at > NAME_CACHE_MS;
    });
    if (missing.length) {
      try {
        for (const name of await client.getUserNames(missing, signal))
          names.set(name.id, { name, at: now() });
      } catch {
        signal.throwIfAborted();
        // Names help the model tell people apart; the thread is still usable without them.
        console.warn('ChattoBot could not read author names for a thread.');
      }
    }
    return messages.map((message) => {
      const name = message.authorId ? names.get(message.authorId)?.name : undefined;
      return name
        ? {
            ...message,
            authorName: name.displayName ?? name.login,
            authorLogin: name.login
          }
        : message;
    });
  };
}
