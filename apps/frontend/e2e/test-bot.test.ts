import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createInterface } from 'node:readline';
import type { TestInfo } from '@playwright/test';
import { getRoomIdByNameViaConnect, postMessageViaConnect } from './fixtures/connectHelpers';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import { expect, test } from './setup';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BOT_SCRIPT = path.resolve(__dirname, '../../../examples/test-bot/dist/index.js');
const BOT_KEY_PATTERN = /cht_BK_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/;
const BOT_KEY_IN_TEXT_PATTERN = /cht_BK_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g;

type BotLogRecord = Record<string, boolean | number | string>;

class TestBotProcess {
  readonly records: BotLogRecord[] = [];
  readonly output: string[] = [];
  readonly #process: ChildProcessWithoutNullStreams;
  readonly #waiters = new Set<{
    predicate: (record: BotLogRecord) => boolean;
    resolve: (record: BotLogRecord) => void;
    reject: (error: Error) => void;
    timer: NodeJS.Timeout;
  }>();

  private constructor(process: ChildProcessWithoutNullStreams) {
    this.#process = process;
    const lines = createInterface({ input: process.stdout });
    lines.on('line', (line) => {
      this.output.push(line);
      let record: BotLogRecord;
      try {
        record = JSON.parse(line) as BotLogRecord;
      } catch {
        return;
      }
      this.records.push(record);
      for (const waiter of this.#waiters) {
        if (!waiter.predicate(record)) continue;
        clearTimeout(waiter.timer);
        this.#waiters.delete(waiter);
        waiter.resolve(record);
      }
    });
    process.stderr.on('data', (chunk) => this.output.push(chunk.toString()));
    process.once('exit', (code) => {
      for (const waiter of this.#waiters) {
        clearTimeout(waiter.timer);
        waiter.reject(new Error(`test bot exited with code ${String(code)}`));
      }
      this.#waiters.clear();
    });
  }

  static start(config: {
    serverUrl: string;
    apiKeyFile: string;
    stateFile: string;
  }): TestBotProcess {
    return new TestBotProcess(
      spawn(process.execPath, [BOT_SCRIPT], {
        env: {
          ...process.env,
          CHATTO_TEST_BOT_SERVER_URL: config.serverUrl,
          CHATTO_TEST_BOT_API_KEY_FILE: config.apiKeyFile,
          CHATTO_TEST_BOT_STATE_FILE: config.stateFile
        },
        stdio: ['ignore', 'pipe', 'pipe']
      })
    );
  }

  waitFor(predicate: (record: BotLogRecord) => boolean, timeoutMs = 10_000): Promise<BotLogRecord> {
    const existing = this.records.find(predicate);
    if (existing) return Promise.resolve(existing);
    return new Promise((resolve, reject) => {
      const waiter = {
        predicate,
        resolve,
        reject,
        timer: setTimeout(() => {
          this.#waiters.delete(waiter);
          reject(new Error(`timed out waiting for test bot record; output: ${this.safeOutput()}`));
        }, timeoutMs)
      };
      this.#waiters.add(waiter);
    });
  }

  safeOutput(): string {
    return this.output.join('\n').replaceAll(BOT_KEY_IN_TEXT_PATTERN, '[REDACTED]');
  }

  containsCredential(): boolean {
    return this.output.some((line) => BOT_KEY_PATTERN.test(line));
  }

  async stop(): Promise<void> {
    if (this.#process.exitCode !== null) return;
    this.#process.kill('SIGTERM');
    await new Promise<void>((resolve) => {
      const timer = setTimeout(() => {
        this.#process.kill('SIGKILL');
        resolve();
      }, 5_000);
      this.#process.once('exit', () => {
        clearTimeout(timer);
        resolve();
      });
    });
  }
}

async function attachBotOutput(testInfo: TestInfo, name: string, process: TestBotProcess) {
  await testInfo.attach(name, {
    body: process.safeOutput(),
    contentType: 'text/plain'
  });
}

test.describe('public API test bot', () => {
  test.use({ serverOptions: { bootstrapTestBot: true } });

  test('reads resources, receives live events, and resumes a disconnected gap', async ({
    page,
    server,
    serverURL
  }, testInfo) => {
    test.setTimeout(60_000);
    const credentialFile = server.bootstrapBotCredentialFile;
    if (!credentialFile) throw new Error('test server did not expose the bot credential file');
    const stateFile = testInfo.outputPath('test_bot.state.json');
    await loginAsAdminAndUsePrimaryServer(page);
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    const first = TestBotProcess.start({
      serverUrl: serverURL,
      apiKeyFile: credentialFile,
      stateFile
    });
    let second: TestBotProcess | undefined;
    try {
      const ready = await first.waitFor((record) => record.status === 'api_ready');
      expect(Number(ready.visible_rooms)).toBeGreaterThanOrEqual(1);
      await first.waitFor((record) => record.status === 'caught_up' && record.resumed === false);

      const firstEventId = await postMessageViaConnect(
        page,
        roomId,
        'First message for the public API test bot'
      );
      await first.waitFor(
        (record) =>
          record.status === 'event' &&
          record.event === 'messagePosted' &&
          record.event_id === firstEventId
      );
      await expect
        .poll(async () => {
          const state = JSON.parse(await readFile(stateFile, 'utf8')) as {
            resumeCursor?: string;
            processedEventIds?: string[];
          };
          return Boolean(state.resumeCursor && state.processedEventIds?.includes(firstEventId));
        })
        .toBe(true);

      await first.stop();
      const missedEventId = await postMessageViaConnect(
        page,
        roomId,
        'Message created while the public API test bot is disconnected'
      );

      second = TestBotProcess.start({
        serverUrl: serverURL,
        apiKeyFile: credentialFile,
        stateFile
      });
      await second.waitFor(
        (record) =>
          record.status === 'event' &&
          record.event === 'messagePosted' &&
          record.event_id === missedEventId
      );
      await second.waitFor((record) => record.status === 'caught_up' && record.resumed === true);

      expect(
        second.records.filter(
          (record) => record.status === 'event' && record.event_id === firstEventId
        )
      ).toHaveLength(0);
      expect(first.containsCredential()).toBe(false);
      expect(second.containsCredential()).toBe(false);
    } finally {
      await first.stop();
      if (second) await second.stop();
      await attachBotOutput(testInfo, 'test-bot-first-session.log', first);
      if (second) await attachBotOutput(testInfo, 'test-bot-resumed-session.log', second);
    }
  });
});
