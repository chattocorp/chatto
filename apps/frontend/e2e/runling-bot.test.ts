import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdir, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  connectPost,
  getRoomIdByNameViaConnect,
  postMessageViaConnect,
  postThreadReplyViaConnect
} from './fixtures/connectHelpers';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import { expect, test } from './setup';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
const replyBody = 'Hello from Runling! I received your webhook and replied through the Chatto API.';

test.describe('Runling webhook bot', () => {
  test.use({ serverOptions: { bootstrapTestBot: true } });

  test('replies to root mentions, thread mentions, and direct messages', async ({
    page,
    server,
    serverURL
  }, testInfo) => {
    test.setTimeout(60_000);
    const keyFile = server.bootstrapBotCredentialFile;
    if (!keyFile) throw new Error('Missing bootstrap bot credential file');
    // Reserve an available port; the Runling CLI does not accept port zero.
    const socket = createServer();
    socket.listen(0, '127.0.0.1');
    await once(socket, 'listening');
    const address = socket.address();
    if (!address || typeof address === 'string') throw new Error('Missing listener port');
    await new Promise<void>((resolve, reject) =>
      socket.close((error) => (error ? reject(error) : resolve()))
    );
    const cwd = testInfo.outputPath('runling');
    await mkdir(cwd, { recursive: true });
    const testConfig = path.join(cwd, 'runling.config.ts');
    await writeFile(
      testConfig,
      `
      import { defineWebConfig } from ${JSON.stringify(path.join(root, 'node_modules/runling/dist/src/web-config.js'))};
      import { createReplyWorkflow } from ${JSON.stringify(path.join(root, 'examples/runling-bot/reply.ts'))};
      export default defineWebConfig({ webhooks: { chatto: { workflow:
        createReplyWorkflow(undefined, undefined, async (_r, context) => {
          if (context.message === "Trigger test model failure") throw new Error("Synthetic model failure");
          if (!context.thread?.some(message => message.body === context.message)) {
            throw new Error('Mention context did not include the current message');
          }
          await context.sender.send(${JSON.stringify(replyBody)});
        })
      } } });
    `
    );
    const bot = spawn(
      process.execPath,
      [
        path.join(root, 'node_modules/runling/bin/runling.js'),
        '--config',
        testConfig,
        '--host',
        '127.0.0.1',
        '--port',
        String(address.port)
      ],
      {
        cwd,
        env: {
          ...process.env,
          CHATTO_RUNLING_SERVER_URL: serverURL,
          CHATTO_RUNLING_API_KEY_FILE: keyFile
        },
        stdio: 'ignore'
      }
    );
    // Register before startup polling so early process failures are also handled.
    const exited = once(bot, 'exit');
    try {
      await expect
        .poll(
          async () => {
            if (bot.exitCode !== null) throw new Error('Runling exited before it was ready');
            try {
              return (await fetch(`http://127.0.0.1:${address.port}`)).ok;
            } catch {
              return false;
            }
          },
          { timeout: 15_000 }
        )
        .toBe(true);
      await loginAsAdminAndUsePrimaryServer(page);
      const listed = await connectPost<{
        bots?: Array<{ user?: { id?: string; login?: string } }>;
      }>(page, 'chatto.api.v1.BotService/ListBots', {});
      const botId = listed.bots?.find((bot) => bot.user?.login === 'test_bot')?.user?.id;
      if (!botId) throw new Error('Bootstrap bot is missing');
      const created = await connectPost<{ webhook: { id: string }; signingSecret: string }>(
        page,
        'chatto.api.v1.BotService/CreateBotOutboundWebhook',
        {
          name: 'Runling',
          botUserId: botId,
          url: `http://localhost:${address.port}/api/runs/start/chatto`,
          enabled: true
        }
      );
      const paused = await connectPost<{ webhook: { id: string }; signingSecret: string }>(
        page,
        'chatto.api.v1.BotService/CreateBotOutboundWebhook',
        {
          botUserId: botId,
          name: 'Paused integration',
          url: 'https://example.com/hook',
          enabled: false
        }
      );
      expect(paused.webhook.id).not.toBe(created.webhook.id);
      expect(paused.signingSecret).not.toBe(created.signingSecret);
      const endpoints = await connectPost<{ webhooks: Array<{ id: string }> }>(
        page,
        'chatto.api.v1.BotService/ListBotOutboundWebhooks',
        { botUserId: botId }
      );
      expect(endpoints.webhooks.map((item) => item.id)).toEqual(
        expect.arrayContaining([created.webhook.id, paused.webhook.id])
      );
      const updated = await connectPost<{
        webhook: { id: string; enabled: boolean };
        signingSecret?: string;
      }>(page, 'chatto.api.v1.BotService/UpdateBotOutboundWebhook', {
        botUserId: botId,
        webhookId: created.webhook.id,
        enabled: false
      });
      // Protobuf JSON omits scalar defaults; an absent enabled field means false.
      expect(updated.webhook.enabled ?? false).toBe(false);
      expect(updated.signingSecret).toBeUndefined();
      await connectPost(page, 'chatto.api.v1.BotService/UpdateBotOutboundWebhook', {
        botUserId: botId,
        webhookId: created.webhook.id,
        enabled: true
      });
      await connectPost(page, 'chatto.api.v1.BotService/RevokeBotOutboundWebhook', {
        botUserId: botId,
        webhookId: paused.webhook.id
      });
      const fetched = await connectPost<{ webhook: { id: string; enabled: boolean } }>(
        page,
        'chatto.api.v1.BotService/GetBotOutboundWebhook',
        { botUserId: botId, webhookId: created.webhook.id }
      );
      expect(fetched.webhook.enabled).toBe(true);
      const roomId = await getRoomIdByNameViaConnect(page, 'general');
      async function expectReply(
        roomId: string,
        rootId: string,
        sourceId: string,
        expectedBody = replyBody
      ) {
        await expect
          .poll(
            async () => {
              const timeline = await connectPost<{ page?: { events?: Array<{ id?: string }> } }>(
                page,
                'chatto.api.v1.ThreadService/GetThreadEvents',
                { roomId, threadRootEventId: rootId, limit: 20 }
              );
              for (const event of timeline.page?.events ?? []) {
                const result = await connectPost<{
                  message?: {
                    actorId?: string;
                    body?: string;
                    inReplyTo?: string;
                    threadRootEventId?: string;
                  };
                }>(page, 'chatto.api.v1.MessageService/GetMessage', { roomId, eventId: event.id });
                if (result.message?.actorId === botId && result.message.inReplyTo === sourceId) {
                  return { body: result.message.body, root: result.message.threadRootEventId };
                }
              }
              return null;
            },
            { timeout: 15_000 }
          )
          .toEqual({ body: expectedBody, root: rootId });
      }
      const rootId = await postMessageViaConnect(page, roomId, '@test_bot Hello Runling');
      await expectReply(roomId, rootId, rootId);
      const threadId = await postThreadReplyViaConnect(
        page,
        roomId,
        '@test_bot Reply in this thread',
        rootId
      );
      await expectReply(roomId, rootId, threadId);
      const dm = await connectPost<{ room?: { id?: string } }>(
        page,
        'chatto.api.v1.RoomService/StartDM',
        {
          participantIds: [botId]
        }
      );
      if (!dm.room?.id) throw new Error('Missing DM room');
      const dmId = await postMessageViaConnect(page, dm.room.id, 'Hello without a mention');
      await expectReply(dm.room.id, dmId, dmId);
      const failedId = await postThreadReplyViaConnect(
        page,
        dm.room.id,
        'Trigger test model failure',
        dmId
      );
      await expectReply(
        dm.room.id,
        dmId,
        failedId,
        "Sorry, I couldn't generate a reply. Please try again."
      );
    } finally {
      if (bot.exitCode === null && bot.signalCode === null) bot.kill('SIGTERM');
      const timer = setTimeout(() => bot.kill('SIGKILL'), 5_000);
      try {
        await exited;
      } finally {
        clearTimeout(timer);
      }
    }
  });
});
