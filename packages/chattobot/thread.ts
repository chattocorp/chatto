import type { MessagingRequests, ThreadMessage as ClientThreadMessage } from '@chatto/client';
import type { Delivery } from './chatto/routing.ts';

/** Thread text classified relative to this bot; other bots also have the human role. */
export interface ThreadMessage extends Omit<ClientThreadMessage, 'fromViewer'> {
  role: 'bot' | 'human';
}

/** A thread read whose messages carry roles relative to this bot. */
export interface ThreadRead {
  messages: ThreadMessage[];
  /** Pass as `after` to read only newer messages. */
  cursor?: string;
  /** True when older replies exist that this read left out. */
  olderOmitted: boolean;
}

/** Read a delivery's thread: without `after`, the root and the newest replies; with `after`, a
 * cursor from an earlier read, only newer messages. Messages carry bot/human roles and the
 * authors' names. */
export type ReadThread = (
  delivery: Delivery,
  signal: AbortSignal,
  after?: string
) => Promise<ThreadRead>;

/** Adapt the application's webhook-compatible delivery to a client thread location.
 * A root message, including a DM root, starts its own thread. */
export function threadLocation(delivery: Delivery) {
  return { roomId: delivery.room_id, threadRootId: delivery.thread_root_id ?? delivery.message.id };
}

/** Read a delivery's thread with bot/human roles; the bot is the client's viewer. */
export function createThreadReader(client: Pick<MessagingRequests, 'readThread'>): ReadThread {
  return async (delivery, signal, after) => {
    const read = await client.readThread(threadLocation(delivery), {
      signal,
      ...(after ? { after } : {})
    });
    return {
      ...read,
      messages: read.messages.map(({ fromViewer, ...message }) => ({
        ...message,
        role: fromViewer ? 'bot' : 'human'
      }))
    };
  };
}
