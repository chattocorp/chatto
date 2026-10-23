/** Botto CLI: own the Chatto connection, inbox, and shutdown signal. */
import { createClient } from '@chatto/client';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { createAgent } from './agent.ts';
import { Inbox } from './inbox.ts';
import { readSettings, type Settings } from './settings.ts';
import { StartupError, startupStep } from './diagnostics.ts';
import { log, logged } from './log.ts';
import { createTurnHandler } from './turn.ts';
import { hasTranscript, incomingMessage } from './messages.ts';

async function run(settings: Settings) {
  const controller = new AbortController();
  const stop = () => controller.abort();
  process.once('SIGINT', stop);
  process.once('SIGTERM', stop);
  const client = createClient();
  let inbox: Inbox | undefined;
  try {
    const agent = await logged('Pi setup', () => createAgent(settings));
    controller.signal.throwIfAborted();
    log('info', 'Connecting to Chatto');
    const server = client.connect({ serverUrl: settings.serverUrl, apiKey: settings.apiKey });
    const { viewerId } = await startupStep(
      'Cannot connect to Chatto. Check chatto.url, chatto.api_key, and server compatibility.',
      () =>
        server.ready({ signal: AbortSignal.any([controller.signal, AbortSignal.timeout(30_000)]) })
    );
    log('info', 'Chatto authenticated');
    inbox = new Inbox(
      createTurnHandler(settings, viewerId, server, agent, controller.signal),
      () => {
        if (!controller.signal.aborted) log('error', 'Botto could not complete a turn');
      },
      settings
    );
    const activeInbox = inbox;
    await server.consumeEvents({
      signal: controller.signal,
      onStatus(status) {
        if (status.state === 'ready')
          log(
            status.gap ? 'warn' : 'info',
            status.gap ? 'Botto connected; some messages may have been missed.' : 'Botto connected.'
          );
        else log('info', 'Realtime connection is not ready');
      },
      async onEvent(event) {
        if (settings.allowedUserId && event.actorId !== settings.allowedUserId) return;
        try {
          const message = await incomingMessage(
            server,
            event,
            async (location) =>
              activeInbox.isActive(location) ||
              hasTranscript(settings.directory, [
                new URL(settings.serverUrl).origin,
                viewerId,
                location.roomId,
                location.threadRootId
              ]),
            controller.signal
          );
          if (!message) return;
          // Other bots must not create an endless reply loop with Botto.
          const { users } = await server
            .service(UserService)
            .batchGetUsers(
              { userIds: [message.authorId] },
              { signal: controller.signal, timeoutMs: 10_000 }
            );
          const author = users.find((item) => item.user?.id === message.authorId)?.user;
          if (!author || author.bot || author.deleted) return;
          const result = activeInbox.accept(message);
          const thread = { threadId: message.threadRootId ?? message.id };
          if (result === 'accepted')
            log('info', 'Message accepted', {
              ...thread,
              waitMs: activeInbox.debounceMs(message)
            });
          else if (result === 'full') log('warn', 'Botto inbox is full; message skipped', thread);
          else if (result === 'duplicate') log('info', 'Duplicate message skipped', thread);
        } catch {
          if (!controller.signal.aborted)
            log('error', 'Botto could not verify an incoming message');
        }
      }
    });
  } catch (error) {
    if (!controller.signal.aborted) throw error;
  } finally {
    log('info', 'Botto stopping');
    controller.abort();
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
