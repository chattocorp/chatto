import type {
  AttachmentContent,
  AttachmentReadOptions,
  MessagingRequests,
  ThreadLocation,
  ThreadMessage as ClientThreadMessage
} from '@chatto/client';

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

/** Read a thread: without `after`, the root and the newest replies; with `after`, a cursor from
 * an earlier read, only newer messages. Messages carry bot/human roles and the authors' names. */
export type ReadThread = (
  thread: ThreadLocation,
  signal: AbortSignal,
  after?: string
) => Promise<ThreadRead>;

/** Read a thread with bot/human roles; the bot is the client's viewer. */
export function createThreadReader(client: Pick<MessagingRequests, 'readThread'>): ReadThread {
  return async (thread, signal, after) => {
    const read = await client.readThread(thread, { signal, ...(after ? { after } : {}) });
    return {
      ...read,
      messages: read.messages.map(({ fromViewer, ...message }) => ({
        ...message,
        role: fromViewer ? 'bot' : 'human'
      }))
    };
  };
}

/** Read the content of an attachment in a room. */
export type ReadAttachment = (
  roomId: string,
  attachmentId: string,
  options: AttachmentReadOptions & { signal: AbortSignal }
) => Promise<AttachmentContent>;

/** Read attachments as the client's viewer. */
export function createAttachmentReader(
  client: Pick<MessagingRequests, 'readAttachment'>
): ReadAttachment {
  return (roomId, attachmentId, options) =>
    client.readAttachment({ roomId, attachmentId }, options);
}
