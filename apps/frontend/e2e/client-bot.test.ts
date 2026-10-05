import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  connectPost,
  getRoomIdByNameViaConnect,
  postMessageViaConnect
} from './fixtures/connectHelpers';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import { expect, test } from './setup';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');

test.describe('@chatto/client bot', () => {
  test.use({ serverOptions: { bootstrapTestBot: true } });

  test('answers a mention over realtime in Node', async ({ page, server, serverURL }, testInfo) => {
    test.setTimeout(60_000);
    const keyFile = server.bootstrapBotCredentialFile;
    if (!keyFile) throw new Error('Missing bootstrap bot credential file');
    const cwd = testInfo.outputPath('client-bot');
    await mkdir(cwd, { recursive: true });
    const script = path.join(cwd, 'bot.mjs');
    // The bot uses the built package, as a Node host outside the workspace would.
    await writeFile(
      script,
      `
      import { readFile } from 'node:fs/promises';
      import { createClient } from ${JSON.stringify(path.join(root, 'packages/chatto-client/dist/index.js'))};
      const apiKey = (await readFile(process.env.KEY_FILE, 'utf8')).trim();
      const client = createClient();
      const connection = client.connect({ serverUrl: process.env.SERVER_URL, apiKey });
      process.on('SIGTERM', () => client.close());
      await connection.run(
        async (ctx) => {
          console.log('handled');
          await ctx.withTyping(() => ctx.reply('pong: ' + ctx.message.body));
          console.log('replied');
        },
        { onStatus: (status) => status.state === 'ready' && console.log('ready') }
      );
      `
    );
    const bot = spawn(process.execPath, [script], {
      cwd,
      env: { ...process.env, SERVER_URL: serverURL, KEY_FILE: keyFile },
      stdio: ['ignore', 'pipe', 'inherit']
    });
    const exited = once(bot, 'exit');
    try {
      let output = '';
      bot.stdout.on('data', (chunk) => (output += chunk));
      await expect.poll(() => output, { timeout: 20_000 }).toContain('ready');

      await loginAsAdminAndUsePrimaryServer(page);
      const roomId = await getRoomIdByNameViaConnect(page, 'general');
      const rootId = await postMessageViaConnect(page, roomId, '@test_bot ping');

      await expect.poll(() => output, { timeout: 15_000 }).toContain('replied');
      const botId = (
        await connectPost<{ bots?: Array<{ user?: { id?: string; login?: string } }> }>(
          page,
          'chatto.api.v1.BotService/ListBots',
          {}
        )
      ).bots?.find((item) => item.user?.login === 'test_bot')?.user?.id;
      const timeline = await connectPost<{ page?: { events?: Array<{ id?: string }> } }>(
        page,
        'chatto.api.v1.ThreadService/GetThreadEvents',
        { roomId, threadRootEventId: rootId, limit: 20 }
      );
      const replies: Array<string | undefined> = [];
      for (const event of timeline.page?.events ?? []) {
        const { message } = await connectPost<{
          message?: { actorId?: string; body?: string; inReplyTo?: string };
        }>(page, 'chatto.api.v1.MessageService/GetMessage', { roomId, eventId: event.id });
        if (message?.actorId === botId && message.inReplyTo === rootId) replies.push(message.body);
      }
      expect(replies).toEqual(['pong: @test_bot ping']);
    } finally {
      bot.kill('SIGTERM');
      await exited;
    }
  });
});
