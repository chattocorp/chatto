/** Connect a debounced message batch to one Pi turn and one Chatto reply. */
import { replyDestination, type AddressedMessage, type Server } from '@chatto/client';
import type { createAgent } from './agent.ts';
import type { Settings } from './settings.ts';
import { log, logged } from './log.ts';
import { readContext, type ContextSource } from './context.ts';

/** Own per-turn deadlines and routing. The inbox must serialize batches for the same thread. */
export function createTurnHandler(
  settings: Settings,
  botId: string,
  server: Pick<Server, 'postMessage' | 'withTyping'> & ContextSource,
  agent: Awaited<ReturnType<typeof createAgent>>,
  shutdown: AbortSignal
) {
  let turnCounter = 0;
  return async (messages: readonly AddressedMessage[]): Promise<void> => {
    shutdown.throwIfAborted();
    const message = messages.at(-1);
    if (!message) return;
    const destination = replyDestination(message);
    const metrics = {
      threadId: destination.threadRootId,
      turn: ++turnCounter,
      messages: messages.length
    };
    log('info', 'Batch ready', metrics);
    const signal = AbortSignal.any([shutdown, AbortSignal.timeout(settings.turnTimeoutMs)]);
    await server.withTyping(
      destination,
      async () => {
        const context = await logged(
          'Thread context read',
          () => readContext(server, destination, settings.contextMessages, signal),
          metrics
        );
        // Persist only the accepted batch. The fresh snapshot travels through Pi's
        // request-only context hook, so old Chatto messages do not accumulate each turn.
        const prompt = JSON.stringify({
          currentMessages: messages.map((item) => ({
            id: item.id,
            author: item.authorId,
            text: item.body
          }))
        });
        const answer = await logged(
          'Model turn',
          () =>
            agent.reply(
              [
                new URL(settings.serverUrl).origin,
                botId,
                destination.roomId,
                destination.threadRootId
              ],
              prompt,
              signal,
              context
            ),
          metrics
        );
        signal.throwIfAborted();
        await logged(
          'Reply post',
          () => server.postMessage(destination, answer, { signal }),
          metrics
        );
      },
      { signal }
    );
  };
}
