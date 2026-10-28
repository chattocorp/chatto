/** Connect a debounced message batch to one Pi turn and its ordered Chatto messages. */
import { replyDestination, type Server } from '@chatto/client';
import type { IncomingMessage } from './messages.ts';
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
import { delegateExtension, type Subagents, type SubagentStatus } from './delegate.ts';
import { viewImageExtension } from './view-image.ts';
import { createTitleWriter } from './title.ts';
import { createParticipationClassifier } from './participation.ts';

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
  beforeEffects: (messages: readonly IncomingMessage[]) => void = () => {}
) {
  const writeTitle = createTitleWriter(settings);
  const classifyParticipation = createParticipationClassifier(settings);
  return async function turn(
    ctx: TaskContext,
    messages: readonly IncomingMessage[],
    children: Subagents,
    completed?: SubagentStatus
  ): Promise<void> {
    shutdown.throwIfAborted();
    const message = messages.at(-1);
    if (!message) return;
    const destination = replyDestination({
      ...message,
      id: message.reaction?.messageId ?? message.id
    });
    const identity = conversationIdentity(settings.serverUrl, botId, destination);
    const addressed = Boolean(completed) || messages.some((item) => item.reasons.length > 0);
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
    let finalAnswer: string | undefined;
    const work = async () => {
      // Read-only failures remain queued and can be retried after reconnect.
      const currentIds = completed
        ? []
        : messages.map((item) => item.reaction?.messageId ?? item.id);
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
      // bot-author marker for the classifier as well as for the main agent.
      context.messages = context.messages.map((item) => ({
        ...item,
        fromViewer: item.author === botId
      }));
      const participation = addressed
        ? { action: 'reply' as const }
        : await classifyParticipation(ctx, context, messages, signal);
      signal.throwIfAborted();
      if (participation.action !== 'reply') {
        beforeEffects(messages);
        if (participation.action === 'react') {
          await safeStep(
            'Reaction',
            () =>
              server.addReaction(
                { roomId: message.roomId, messageId: message.id },
                participation.emoji,
                { signal }
              ),
            metrics
          );
        }
        return;
      }
      const reply = async () => {
        // Resolve reply authors before starting effects. Targets can be outside
        // the recent context window; unavailable targets have an unknown author.
        const currentMessages = await Promise.all(
          (completed ? [] : messages).map(async (item) => {
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
        signal.throwIfAborted();
        if (!completed) beforeEffects(messages);
        // Existing transcripts identify ongoing conversations, including after a restart.
        if (
          !completed &&
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
          currentMessages,
          ...(completed
            ? {
                subagentResult: completed,
                instruction:
                  'This is an automatic subagent completion, not a new user request. If you already answered using this result, end with no assistant text. Do not acknowledge completion, repeat the answer, or say that you already answered. Otherwise, use the result to finish the original request at the requested length.'
              }
            : {})
        });
        // Pi's callback is synchronous. Serialize writes and join them before leaving
        // the turn. A failed write stops the model; never retry an uncertain post.
        let delivery = Promise.resolve();
        const onText = (text: string) => {
          const body = text.trim();
          if (!body || signal.aborted) return;
          delivery = delivery.then(async () => {
            signal.throwIfAborted();
            await postReply(body);
          });
          void delivery.catch((error) => deliveryFailure.abort(error));
        };
        if (!completed)
          naming = writeTitle(ctx, messages.map((message) => message.body).join('\n\n'), signal);
        try {
          const answer = await safeStep(
            'Model turn',
            () =>
              agent.reply(ctx, {
                identity,
                prompt,
                signal,
                onText,
                context,
                loadImages: context.messages.some((message) =>
                  message.attachments?.some((attachment) =>
                    attachment.contentType.startsWith('image/')
                  )
                )
                  ? () =>
                      safeStep(
                        'Read images',
                        () => readImages(server, destination.roomId, context, currentIds, signal),
                        metrics
                      )
                  : undefined,
                userIds: [...new Set(messages.map((item) => item.authorId))],
                capabilities: ['delegation', 'images', 'reactions'],
                tools: [
                  delegateExtension(children, settings, signal, (result) =>
                    turn(ctx, messages, children, result)
                  ),
                  viewImageExtension(server, destination.roomId, context, signal),
                  reactionExtension(
                    server,
                    destination.roomId,
                    destination.threadRootId,
                    new Set([
                      ...messages.filter((item) => !item.reaction).map((item) => item.id),
                      ...context.messages.map((item) => item.id)
                    ]),
                    signal
                  )
                ]
              }),
            metrics
          );
          signal.throwIfAborted();
          finalAnswer = answer.trim();
        } finally {
          await delivery;
        }
      };
      if (addressed) await reply();
      else await server.withTyping(destination, reply, { signal });
    };
    // Passive activity stays quiet until the classifier selects a textual reply.
    try {
      if (addressed) await server.withTyping(destination, work, { signal });
      else await work();
      // End typing, including any refresh in flight, before the final message
      // clears the indicator. Title generation does not represent chat activity.
      if (finalAnswer) {
        signal.throwIfAborted();
        await postReply(finalAnswer);
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
