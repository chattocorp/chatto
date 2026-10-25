/** Botto CLI: own the Chatto connection, inbox, and shutdown signal. */
import './env.ts';
import { createClient } from '@chatto/client';
import { RECENT_THREAD_LIMIT } from '@chatto/client/server/roomStores';
import { createAgent } from './agent.ts';
import { Inbox } from './inbox.ts';
import { readSettings, type Settings } from './settings.ts';
import { StartupError, startupStep } from './diagnostics.ts';
import { log, logged } from './log.ts';
import { createTurnHandler } from './turn.ts';
import { hasTranscript, incomingMessage, isHumanAuthor } from './messages.ts';
import { Delivery } from './delivery.ts';
import { recover } from './recovery.ts';
import { setTimeout as delay } from 'node:timers/promises';
import { Code, ConnectError } from '@connectrpc/connect';

async function run(settings: Settings) {
  const controller = new AbortController();
  const stop = () => controller.abort();
  process.once('SIGINT', stop);
  process.once('SIGTERM', stop);
  // Keep at least one thread timeline for each thread that can have a turn.
  const client = createClient({
    keepRecent: { threads: Math.max(RECENT_THREAD_LIMIT, settings.maxThreads) }
  });
  let inbox: Inbox | undefined;
  let recovery: Promise<void> | undefined;
  try {
    const agent = await logged('Pi setup', () => createAgent(settings));
    controller.signal.throwIfAborted();
    log('info', 'Connecting to Chatto');
    const server = client.connect({ serverUrl: settings.serverUrl, apiKey: settings.apiKey });
    if (settings.reportPresence) server.reportPresence();
    const { viewerId } = await startupStep(
      'Cannot connect to Chatto. Check chatto.url, chatto.api_key, and server compatibility.',
      () => server.ready({ signal: controller.signal })
    );
    log('info', 'Chatto authenticated');
    const delivery = new Delivery(settings.directory, [
      new URL(settings.serverUrl).origin,
      viewerId
    ]);
    if (delivery.uncertain)
      log('warn', 'Interrupted work needs operator review in the delivery state file', {
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
        await turn(messages);
        delivery.complete(messages);
      },
      () => {
        if (!controller.signal.aborted)
          log(
            'error',
            'Botto could not complete a turn; queued work will retry, running work needs operator review'
          );
      },
      settings
    );
    const activeInbox = inbox;
    const isActive = async (location: { roomId: string; threadRootId: string }) =>
      activeInbox.isActive(location) ||
      delivery.queued.some(
        (item) =>
          item.roomId === location.roomId &&
          (item.threadRootId ?? item.id) === location.threadRootId
      ) ||
      hasTranscript(settings.directory, [
        new URL(settings.serverUrl).origin,
        viewerId,
        location.roomId,
        location.threadRootId
      ]);
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
          if (!controller.signal.aborted) log('warn', 'Message recovery failed; Botto will retry');
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
          log(
            status.gap ? 'warn' : 'info',
            status.gap
              ? 'Botto connected; checking retained notifications for missed messages.'
              : 'Botto connected.'
          );
        } else log('info', 'Realtime connection is not ready');
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
              log('info', 'Message accepted', {
                ...thread,
                waitMs: activeInbox.debounceMs(message)
              });
            else if (result === 'full')
              log('warn', 'Botto inbox is full; message saved for later', thread);
            else if (result === 'duplicate') log('info', 'Duplicate message skipped', thread);
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
              log('warn', 'Incoming message is no longer available or permitted');
              return;
            }
            if (!signal.aborted)
              log('error', 'Botto could not verify or save an incoming message; retrying');
            await delay(5000, undefined, { signal }).catch(() => {});
          }
        }
      }
    });
  } catch (error) {
    if (!controller.signal.aborted) throw error;
  } finally {
    log('info', 'Botto stopping');
    controller.abort();
    await recovery;
    await inbox?.close();
    client.close();
    process.removeListener('SIGINT', stop);
    process.removeListener('SIGTERM', stop);
    log('info', 'Botto stopped');
  }
}

if (process.argv.includes('--help')) {
  console.info(
    'Botto: start [--config PATH]. Defaults to botto.toml. Configure chatto.api_key with an environment reference and keep secrets in .env. See README.md.'
  );
} else {
  // Newly created transcripts are private, even when the operator has a permissive umask.
  process.umask(0o077);
  let settings: Settings | undefined;
  try {
    const args = process.argv.slice(2);
    if (args.length !== 0 && (args.length !== 2 || args[0] !== '--config' || !args[1]?.trim()))
      throw new Error('Usage: start [--config PATH]');
    settings = readSettings(args[1]);
  } catch (error) {
    log('error', error instanceof Error ? error.message : 'Invalid Botto configuration.');
    process.exitCode = 1;
  }
  if (settings)
    await run(settings).catch((error: unknown) => {
      // Provider and transport errors can include credentials or conversation text.
      log(
        'error',
        error instanceof StartupError
          ? error.message
          : 'Botto stopped after an unexpected runtime failure.'
      );
      process.exitCode = 1;
    });
}
