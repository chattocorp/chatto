/** Exercise Botto, Runling, Pi, and the journal with only the provider stream replaced. */
import { mkdir, mkdtemp, readFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { expect, test, vi } from 'vitest';
import { task, Type } from 'runling';
import { RunStore } from '../../../packages/runling/src/lib/server/run-store.ts';
import { parseSettings } from './settings.ts';

// Load the real source through Vitest so its Pi import uses the offline provider seam.
vi.mock('runling', () => import('../../../packages/runling/src/runtime/index.ts'));

const sdkPath = await vi.hoisted(async () => {
  const { realpath } = await import('node:fs/promises');
  return realpath(
    '../../packages/runling/node_modules/@earendil-works/pi-coding-agent/dist/index.js'
  );
});
vi.mock(sdkPath, async () => {
  const sdk = await vi.importActual<typeof import('@earendil-works/pi-coding-agent')>(sdkPath);
  return {
    ...sdk,
    createAgentSession: async (...args: Parameters<typeof sdk.createAgentSession>) => {
      const result = await sdk.createAgentSession(...args);
      await result.session.modelRuntime.setRuntimeApiKey('anthropic', 'offline-test');
      const agent = result.session.agent;
      agent.getApiKey = () => 'offline-test';
      agent.streamFunction = () => {
        type Stream = Awaited<ReturnType<typeof agent.streamFunction>>;
        const message: Awaited<ReturnType<Stream['result']>> = {
          role: 'assistant',
          api: 'anthropic-messages',
          provider: 'anthropic',
          model: 'claude-sonnet-4-6',
          timestamp: Date.now(),
          stopReason: 'stop',
          content: [{ type: 'text', text: 'private-answer' }],
          usage: {
            input: 11,
            output: 7,
            cacheRead: 5,
            cacheWrite: 3,
            totalTokens: 26,
            cost: { input: 0.01, output: 0.02, cacheRead: 0.03, cacheWrite: 0.04, total: 0.1 }
          }
        };
        return {
          async *[Symbol.asyncIterator]() {
            yield { type: 'done', reason: 'stop', message };
          },
          result: async () => message
        } as unknown as Stream;
      };
      return result;
    }
  };
});

test('records fresh usage and cost for reopened Botto sessions without logging content', async () => {
  const { createAgent } = await import('./agent.ts');
  await mkdir(resolve('../../.context'), { recursive: true });
  const directory = await mkdtemp(resolve('../../.context/botto-usage-'));
  const store = new RunStore(join(directory, 'runs'));
  try {
    await store.init();
    const agent = await createAgent({
      ...parseSettings(
        '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="anthropic/claude-sonnet-4-6"',
        'botto.toml'
      ),
      directory,
      memory: false,
      mcp: false,
      web: false
    });
    const root = task(
      { name: 'Usage fixture', input: Type.Null(), result: Type.Null() },
      async (ctx) => {
        for (let turn = 0; turn < 2; turn++) {
          expect(
            await agent.reply(ctx, {
              identity: ['server', 'bot', 'room', 'thread'],
              prompt: 'private-prompt',
              signal: ctx.signal
            })
          ).toBe('private-answer');
        }
        return null;
      }
    );
    const handle = await store.start('fixture', root, null, 'source');
    expect((await handle.completion).ok).toBe(true);
    const detail = (await store.get(handle.id))!;
    expect(detail.usage).toEqual({
      input: 22,
      output: 14,
      cacheRead: 10,
      cacheWrite: 6,
      cost: 0.2
    });
    expect(detail.events.filter((event) => event.type === 'agent.usage')).toHaveLength(2);
    const journal = await readFile(join(store.directory, `${handle.id}.jsonl`), 'utf8');
    expect(journal).toContain('usage.updated');
    expect(journal).not.toContain('private-prompt');
    expect(journal).not.toContain('private-answer');
  } finally {
    await store.close();
    await rm(directory, { recursive: true, force: true });
  }
});
