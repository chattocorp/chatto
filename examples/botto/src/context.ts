/** Select Pi's text context from a fresh thread read. */
import type { Server, ThreadLocation } from '@chatto/client';
import type { ThreadContext } from './context-messages.ts';

/** Only the client's thread read is needed. */
export type ContextSource = Pick<Server, 'readThread'>;

/** Read the root and up to limit recent replies, and keep the messages with text.
 * Each turn reads again, so the context includes edits and removals. */
export async function readContext(
  server: ContextSource,
  location: ThreadLocation,
  limit: number,
  signal: AbortSignal
): Promise<ThreadContext> {
  const read = await server.readThread(location, { limit, signal });
  return {
    messages: read.messages.flatMap((message) =>
      message.body
        ? [
            {
              id: message.id,
              author: message.authorId,
              text: message.body,
              fromViewer: message.fromViewer
            }
          ]
        : []
    ),
    olderContextOmitted: read.olderOmitted
  };
}
