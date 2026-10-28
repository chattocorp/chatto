/** Runling source: own the Chatto connection, durable delivery, and bounded inbox. */
import type { EventSourceContext } from 'runling/web';
import { runTurn, closeConversations } from './conversation.ts';
import { createClient, replyDestination } from '@chatto/client';
import { conversationIdentity, threadKey } from './threads.ts';
import { RECENT_THREAD_LIMIT } from '@chatto/client/server/roomStores';
import { createAgent } from './agent.ts';
import { Inbox } from './inbox.ts';
import { type Settings } from './settings.ts';
import { startupStep } from './diagnostics.ts';
import { log } from 'runling';
import { safeStep } from './steps.ts';
import { createTurnHandler } from './turn.ts';
import { hasTranscript, incomingMessage, isHumanAuthor } from './messages.ts';
import { Delivery } from './delivery.ts';
import { recover } from './recovery.ts';
import { setTimeout as delay } from 'node:timers/promises';
import { Code, ConnectError } from '@connectrpc/connect';

export async function runSource(settings: Settings, source: EventSourceContext) {
  const controller = new AbortController();
  const stop = () => controller.abort();
  source.signal.addEventListener('abort', stop, { once: true });
  if (source.signal.aborted) stop();
  // Keep at least one thread timeline for each thread that can have a turn.
  const client = createClient({
    keepRecent: { threads: Math.max(RECENT_THREAD_LIMIT, settings.maxThreads) }
  });
  let inbox: Inbox | undefined;
  let recovery: Promise<void> | undefined;
  try {
    const agent = await safeStep('Agent setup', () => createAgent(settings));
    controller.signal.throwIfAborted();
    log.info('Connecting to Chatto');
    const server = client.connect({ serverUrl: settings.serverUrl, apiKey: settings.apiKey });
    if (settings.reportPresence) server.reportPresence();
    const { viewerId } = await startupStep(
      'Cannot connect to Chatto. Check chatto.url, chatto.api_key, and server compatibility.',
      () => server.ready({ signal: controller.signal })
    );
    log.info('Chatto authenticated');
    const delivery = new Delivery(settings.directory, [
      new URL(settings.serverUrl).origin,
      viewerId
    ]);
    if (delivery.uncertain)
      log.warn('Interrupted work needs operator review in the delivery state file', {
        messages: delivery.uncertain
      });
    const turn = createTurnHandler(
      settings,
      viewerId,
      server,
      agent,
      controller.signal,
      (messages) => delivery.start(messages)
    );
    inbox = new Inbox(
      async (messages) => {
        const first = messages[0]!;
        const key = JSON.stringify(
          conversationIdentity(settings.serverUrl, viewerId, replyDestination(first))
        );
        await runTurn(
          source,
          key,
          (ctx, children) => turn(ctx, messages, children),
          settings.maxThreads
        );
        delivery.complete(messages);
      },
      () => {
        if (!controller.signal.aborted)
          log.error(
            'Botto could not complete a turn; queued work will retry, running work needs operator review'
          );
      },
      settings
    );
    const activeInbox = inbox;
    const isActive = async (location: { roomId: string; threadRootId: string }) =>
      activeInbox.isActive(location) ||
      delivery.queued.some((item) => threadKey(replyDestination(item)) === threadKey(location)) ||
      hasTranscript(
        settings.directory,
        conversationIdentity(settings.serverUrl, viewerId, location)
      );
    const pump = () => {
      for (const message of delivery.queued) {
        if (settings.allowedUserId && message.authorId !== settings.allowedUserId) continue;
        activeInbox.accept(message);
      }
    };
    pump();
    // Recovery also runs periodically: notification projection and the realtime
    // connection can become ready at different times. Only one scan runs at once.
    let nextRecovery = 0;
    recovery = (async () => {
      while (!controller.signal.aborted) {
        try {
          await server.ready({ signal: controller.signal });
          if (Date.now() >= nextRecovery) {
            nextRecovery = Date.now() + 30_000;
            await recover(
              server,
              delivery,
              isActive,
              controller.signal,
              settings.allowedUserId,
              settings.respondTo
            );
          }
        } catch {
          if (!controller.signal.aborted) log.warn('Message recovery failed; Botto will retry');
        }
        if (controller.signal.aborted) break;
        pump();
        await delay(5000, undefined, { signal: controller.signal }).catch(() => {});
      }
    })();
    await server.consumeEvents({
      signal: controller.signal,
      onStatus(status) {
        if (status.state === 'ready') {
          nextRecovery = 0;
          log[status.gap ? 'warn' : 'info'](
            status.gap
              ? 'Botto connected; checking retained notifications for missed messages.'
              : 'Botto connected.'
          );
        } else log.info('Realtime connection is not ready');
      },
      async onEvent(event, { signal }) {
        if (settings.allowedUserId && event.actorId !== settings.allowedUserId) return;
        if (event.event.case === 'userTyping') {
          activeInbox.typing(event, viewerId);
          return;
        }
        while (!signal.aborted) {
          try {
            const message = await incomingMessage(
              server,
              event,
              isActive,
              signal,
              settings.respondTo
            );
            if (!message) return;
            if (delivery.has(message.id)) return;
            if (!(await isHumanAuthor(server, message.authorId, signal))) return;
            delivery.accept(message);
            const result = activeInbox.accept(message);
            const thread = { threadId: message.threadRootId ?? message.id };
            if (result === 'accepted')
              log.info('Message accepted', {
                ...thread,
                waitMs: activeInbox.debounceMs(message)
              });
            else if (result === 'full')
              log.warn('Botto inbox is full; message saved for later', thread);
            else if (result === 'duplicate') log.debug('Duplicate message skipped', thread);
            return;
          } catch (error) {
            // Removed content and revoked access cannot be repaired by retrying
            // this event. Do not block later messages on a permanent rejection.
            if (
              error instanceof ConnectError &&
              [
                Code.NotFound,
                Code.PermissionDenied,
                Code.Unauthenticated,
                Code.InvalidArgument,
                Code.FailedPrecondition
              ].includes(error.code)
            ) {
              log.warn('Incoming message is no longer available or permitted');
              return;
            }
            if (!signal.aborted)
              log.error('Botto could not verify or save an incoming message; retrying');
            await delay(5000, undefined, { signal }).catch(() => {});
          }
        }
      }
    });
  } catch (error) {
    if (!controller.signal.aborted) throw error;
  } finally {
    log.info('Botto stopping');
    controller.abort();
    await recovery;
    await closeConversations(source);
    await inbox?.close();
    client.close();
    source.signal.removeEventListener('abort', stop);
    log.info('Botto stopped');
  }
}
