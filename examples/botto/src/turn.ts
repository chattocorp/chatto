/** Connect a debounced message batch to one Pi turn and one Chatto reply. */
import { replyDestination, type AddressedMessage, type Server } from '@chatto/client';
import type { createAgent } from './agent.ts';
import type { Settings } from './settings.ts';
import { safeStep } from './steps.ts';
import { log } from 'runling';
import { includeCurrentMessages, readContext, type ContextSource } from './context.ts';
import { reactionExtension } from './reactions.ts';
import { hasTranscript } from './messages.ts';
import { Code, ConnectError } from '@connectrpc/connect';
import type { TaskContext } from 'runling';
import { conversationIdentity } from './threads.ts';
import { readImages } from './images.ts';
import { viewImageExtension } from './view-image.ts';

/** Own per-turn deadlines and routing. The inbox must serialize batches for the same thread. */
export function createTurnHandler(
  settings: Settings,
  botId: string,
  server: Pick<
    Server,
    'postMessage' | 'withTyping' | 'addReaction' | 'getMessage' | 'readAttachment'
  > &
    ContextSource,
  agent: Awaited<ReturnType<typeof createAgent>>,
  shutdown: AbortSignal,
  beforeEffects: (messages: readonly AddressedMessage[]) => void = () => {}
) {
  return async (ctx: TaskContext, messages: readonly AddressedMessage[]): Promise<void> => {
    shutdown.throwIfAborted();
    const message = messages.at(-1);
    if (!message) return;
    const destination = replyDestination(message);
    const identity = conversationIdentity(settings.serverUrl, botId, destination);
    const metrics = {
      threadId: destination.threadRootId,
      messages: messages.length
    };
    log.info('Batch ready', metrics);
    const signal = AbortSignal.any([
      shutdown,
      ctx.signal,
      AbortSignal.timeout(settings.turnTimeoutMs)
    ]);
    await server.withTyping(
      destination,
      async () => {
        // Read-only failures remain queued and can be retried after reconnect.
        const currentIds = messages.map((item) => item.id);
        const context = await safeStep(
          'Thread context read',
          async () =>
            includeCurrentMessages(
              server,
              destination.roomId,
              await readContext(server, destination, settings.contextMessages, signal),
              currentIds,
              signal
            ),
          metrics
        );
        // Resolve reply authors before starting effects. Targets can be outside
        // the recent context window; unavailable targets have an unknown author.
        const currentMessages = await Promise.all(
          messages.map(async (item) => {
            const message = { id: item.id, author: item.authorId, text: item.body };
            if (!item.inReplyTo) return message;
            const author =
              context.messages.find((target) => target.id === item.inReplyTo)?.author ??
              (await readReplyAuthor(server, item.roomId, item.inReplyTo, signal));
            return {
              ...message,
              inReplyTo: { id: item.inReplyTo, author }
            };
          })
        );
        signal.throwIfAborted();
        beforeEffects(messages);
        // Existing transcripts identify ongoing conversations, including after a restart.
        if (settings.acknowledgementEmoji && !(await hasTranscript(settings.directory, identity))) {
          signal.throwIfAborted();
          const first = messages[0]!;
          // A failed acknowledgement must not stop the answer or consume its whole deadline.
          await safeStep(
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
        const prompt = JSON.stringify({ currentMessages });
        const answer = await safeStep(
          'Model turn',
          () =>
            agent.reply(ctx, {
              identity,
              prompt,
              signal,
              context,
              loadImages: () =>
                safeStep(
                  'Read images',
                  () => readImages(server, destination.roomId, context, currentIds, signal),
                  metrics
                ),
              userIds: [...new Set(messages.map((item) => item.authorId))],
              tools: [
                viewImageExtension(server, destination.roomId, context, signal),
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
            }),
          metrics
        );
        signal.throwIfAborted();
        await safeStep(
          'Reply post',
          () => server.postMessage(destination, answer, { signal }),
          metrics
        );
      },
      { signal }
    );
  };
}

/** Missing or inaccessible targets have no known author. Other failures remain retryable. */
async function readReplyAuthor(
  server: Pick<Server, 'getMessage'>,
  roomId: string,
  messageId: string,
  signal: AbortSignal
): Promise<string | null> {
  try {
    return (await server.getMessage({ roomId, messageId }, { signal }))?.authorId ?? null;
  } catch (error) {
    if (
      error instanceof ConnectError &&
      [Code.NotFound, Code.PermissionDenied].includes(error.code)
    )
      return null;
    throw error;
  }
}
