/** Connect a debounced message batch to one Pi turn and its ordered Chatto messages. */
import { replyDestination, type Server } from '@chatto/client';
import type { IncomingMessage } from './messages.ts';
import type { createAgent } from './agent.ts';
import type { Settings } from './settings.ts';
import { safeStep } from './steps.ts';
import { log } from 'runling';
import {
  includeCurrentMessages,
  readContext,
  readParticipants,
  type ParticipantSource,
  type ContextSource
} from './context.ts';
import { hasTranscript } from './messages.ts';
import { Code, ConnectError } from '@connectrpc/connect';
import type { TaskContext } from 'runling';
import { conversationIdentity } from './threads.ts';
import { readImages } from './images.ts';
import { viewImageExtension } from './view-image.ts';
import { createTitleWriter } from './title.ts';
import type { ConversationResponse } from './response.ts';

/** Own per-turn deadlines and routing. The inbox must serialize batches for the same thread. */
export function createTurnHandler(
  settings: Settings,
  botId: string,
  server: Pick<
    Server,
    'postMessage' | 'withTyping' | 'addReaction' | 'getMessage' | 'readAttachment'
  > &
    ContextSource &
    ParticipantSource,
  agent: Awaited<ReturnType<typeof createAgent>>,
  shutdown: AbortSignal,
  beforeEffects: (messages: readonly IncomingMessage[]) => void = () => {}
) {
  const writeTitle = createTitleWriter(settings);
  return async function turn(
    ctx: TaskContext,
    messages: readonly IncomingMessage[]
  ): Promise<void> {
    shutdown.throwIfAborted();
    const message = messages.at(-1);
    if (!message) return;
    const destination = replyDestination({
      ...message,
      id: message.reaction?.messageId ?? message.id
    });
    const identity = conversationIdentity(settings.serverUrl, botId, destination);
    const addressed = messages.some((item) => item.reasons.length > 0);
    const metrics = {
      threadId: destination.threadRootId,
      messages: messages.length
    };
    log.info('Batch ready', metrics);
    const deliveryFailure = new AbortController();
    const signal = AbortSignal.any([
      shutdown,
      ctx.signal,
      deliveryFailure.signal,
      AbortSignal.timeout(settings.turnTimeoutMs)
    ]);
    const postReply = (body: string) =>
      safeStep('Reply post', () => server.postMessage(destination, body, { signal }), metrics);
    let naming: Promise<void> | undefined;
    let response: ConversationResponse | undefined;
    const work = async () => {
      // Read-only failures remain queued and can be retried after reconnect.
      const currentIds = messages.map((item) => item.reaction?.messageId ?? item.id);
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
      // Targets outside the recent window were fetched separately. Preserve the
      // bot-author marker for the conversation agent.
      context.messages = context.messages.map((item) => ({
        ...item,
        fromViewer: item.author === botId
      }));
      // Resolve reply authors before starting effects. Targets can be outside
      // the recent context window; unavailable targets have an unknown author.
      const currentMessages = await Promise.all(
        messages.map(async (item) => {
          if (item.reaction) return { author: item.authorId, reaction: item.reaction };
          const message = {
            id: item.id,
            author: item.authorId,
            text: item.body,
            reasons: item.reasons
          };
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
      context.participants = await safeStep(
        'Participant names read',
        () =>
          readParticipants(
            server,
            [
              ...context.messages.map((item) => item.author),
              ...messages.map((item) => item.authorId),
              ...currentMessages.flatMap((item) =>
                'inReplyTo' in item && item.inReplyTo?.author ? [item.inReplyTo.author] : []
              )
            ],
            signal
          ),
        metrics
      );
      signal.throwIfAborted();
      beforeEffects(messages);
      // Existing transcripts identify ongoing conversations, including after a restart.
      if (
        addressed &&
        settings.acknowledgementEmoji &&
        !(await hasTranscript(settings.directory, identity))
      ) {
        signal.throwIfAborted();
        const first = messages.find((item) => item.reasons.length > 0)!;
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
      // Persist the accepted batch and its current UTC time. The fresh snapshot travels through Pi's
      // request-only context hook, so old Chatto messages do not accumulate each turn.
      const prompt = JSON.stringify({
        currentDateTime: new Date().toISOString(),
        currentMessages
      });
      // Progress is an explicit tool: await its write and stop on uncertain delivery.
      const postUpdate = async (body: string) => {
        signal.throwIfAborted();
        try {
          await postReply(body);
        } catch (error) {
          deliveryFailure.abort(error);
          throw error;
        }
      };
      naming = writeTitle(ctx, messages.map((message) => message.body).join('\n\n'), signal);
      const answer = await safeStep(
        'Model turn',
        () =>
          agent.reply(ctx, {
            identity,
            prompt,
            signal,
            postUpdate,
            allowReaction: !message.reaction,
            context,
            loadImages: context.messages.some((message) =>
              message.attachments?.some((attachment) => attachment.contentType.startsWith('image/'))
            )
              ? () =>
                  safeStep(
                    'Read images',
                    () => readImages(server, destination.roomId, context, currentIds, signal),
                    metrics
                  )
              : undefined,
            userIds: [...new Set(messages.map((item) => item.authorId))],
            capabilities: ['images'],
            tools: [viewImageExtension(server, destination.roomId, context, signal)]
          }),
        metrics
      );
      signal.throwIfAborted();
      response = answer;
    };
    // All accepted activity reaches the same conversation agent.
    try {
      await server.withTyping(destination, work, { signal });
      // End typing, including any refresh in flight, before the final message
      // clears the indicator. Title generation does not represent chat activity.
      signal.throwIfAborted();
      if (!response) throw new Error('Conversation returned no response decision.');
      log.info(`Response: ${response.action}`, metrics);
      if (response.action === 'reply') await postReply(response.text);
      else if (response.action === 'react') {
        if (message.reaction) throw new Error('Cannot react to an incoming reaction.');
        const emoji = response.emoji;
        await safeStep(
          'Reaction',
          () =>
            server.addReaction({ roomId: message.roomId, messageId: message.id }, emoji, {
              signal
            }),
          metrics
        );
      }
    } finally {
      // Naming handles its own errors. Join it even if delivery has failed.
      await naming;
    }
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
