/** Connect a debounced message batch to one Pi turn and one Chatto reply. */
import { replyDestination, type AddressedMessage, type Server } from '@chatto/client';
import type { createAgent } from './agent.ts';
import type { Settings } from './settings.ts';
import { log, logged } from './log.ts';
import { readContext, type ContextSource } from './context.ts';
import { reactionExtension } from './reactions.ts';
import { hasTranscript } from './messages.ts';

/** Own per-turn deadlines and routing. The inbox must serialize batches for the same thread. */
export function createTurnHandler(
  settings: Settings,
  botId: string,
  server: Pick<Server, 'postMessage' | 'withTyping' | 'addReaction'> & ContextSource,
  agent: Awaited<ReturnType<typeof createAgent>>,
  shutdown: AbortSignal,
  beforeEffects: (messages: readonly AddressedMessage[]) => void = () => {}
) {
  let turnCounter = 0;
  return async (messages: readonly AddressedMessage[]): Promise<void> => {
    shutdown.throwIfAborted();
    const message = messages.at(-1);
    if (!message) return;
    const destination = replyDestination(message);
    const identity = [
      new URL(settings.serverUrl).origin,
      botId,
      destination.roomId,
      destination.threadRootId
    ];
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
        // Read-only failures remain queued and can be retried after reconnect.
        const context = await logged(
          'Thread context read',
          () => readContext(server, destination, settings.contextMessages, signal),
          metrics
        );
        signal.throwIfAborted();
        beforeEffects(messages);
        // Existing transcripts identify ongoing conversations, including after a restart.
        if (settings.acknowledgementEmoji && !(await hasTranscript(settings.directory, identity))) {
          signal.throwIfAborted();
          const first = messages[0]!;
          // A failed acknowledgement must not stop the answer or consume its whole deadline.
          await logged(
            'Acknowledgement',
            () =>
              server.addReaction(
                { roomId: first.roomId, messageId: first.id },
                settings.acknowledgementEmoji,
                { signal: AbortSignal.any([signal, AbortSignal.timeout(2000)]) }
              ),
            metrics
          ).catch(() => {});
        }
        signal.throwIfAborted();
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
              identity,
              prompt,
              signal,
              context,
              [...new Set(messages.map((item) => item.authorId))],
              [
                reactionExtension(
                  server,
                  destination.roomId,
                  destination.threadRootId,
                  new Set([
                    ...messages.map((item) => item.id),
                    ...context.messages.map((item) => item.id)
                  ]),
                  signal
                )
              ]
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
