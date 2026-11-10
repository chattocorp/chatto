/** Exercise Botto, Runling, Pi, and the journal with only the provider stream replaced. */
import { mkdir, mkdtemp, readFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { expect, test, vi } from 'vitest';
import { task, Type } from 'runling';
import { RunStore } from '../../../packages/runling/src/lib/server/run-store.ts';
import { parseSettings } from './settings.ts';
import sharp from 'sharp';
import { viewImageExtension } from './view-image.ts';

const cropFixture = vi.hoisted(() => ({ data: '', calls: 0, terminal: '', requests: 0 }));

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
      expect(result.session.thinkingLevel).toBe('low');
      await result.session.modelRuntime.setRuntimeApiKey('anthropic', 'offline-test');
      const agent = result.session.agent;
      agent.getApiKey = () => 'offline-test';
      agent.streamFunction = (_model, context) => {
        cropFixture.requests++;
        const terminal = cropFixture.terminal;
        const lastMessage = context.messages.at(-1);
        const needsUpdate = !terminal && lastMessage?.role === 'user';
        const needsCrop =
          !terminal && lastMessage?.role === 'toolResult' && lastMessage.toolName === 'post_update';
        if (!terminal && !needsCrop && !needsUpdate) {
          expect(lastMessage?.role).toBe('toolResult');
          expect(
            lastMessage?.role === 'toolResult' &&
              lastMessage.content.some(
                (part) => part.type === 'image' && part.data === cropFixture.data
              )
          ).toBe(true);
        } else {
          expect(
            context.messages
              .filter((message) => message.role === 'toolResult')
              .every((message) => message.content.every((part) => part.type !== 'image'))
          ).toBe(true);
        }
        type Stream = Awaited<ReturnType<typeof agent.streamFunction>>;
        const message: Awaited<ReturnType<Stream['result']>> = {
          role: 'assistant',
          api: 'anthropic-messages',
          provider: 'anthropic',
          model: 'claude-sonnet-4-6',
          timestamp: Date.now(),
          stopReason: needsCrop || needsUpdate || terminal ? 'toolUse' : 'stop',
          content: terminal
            ? [
                { type: 'text', text: 'This terminal text must not be posted.' },
                {
                  type: 'toolCall',
                  id: `terminal-${++cropFixture.calls}`,
                  name: terminal,
                  arguments: terminal === 'stay_silent' ? {} : { emoji: 'heart' }
                },
                // A malformed batch must not post after selecting a terminal response.
                {
                  type: 'toolCall',
                  id: `late-${cropFixture.calls}`,
                  name: 'post_update',
                  arguments: { text: 'Must not be posted' }
                }
              ]
            : needsUpdate
              ? [
                  { type: 'text', text: 'Unposted tool narration' },
                  {
                    type: 'toolCall',
                    id: `update-${++cropFixture.calls}`,
                    name: 'post_update',
                    arguments: { text: 'I will take a closer look.' }
                  }
                ]
              : needsCrop
                ? [
                    {
                      type: 'toolCall',
                      id: `crop-${++cropFixture.calls}`,
                      name: 'view_image',
                      arguments: {
                        attachmentId: 'image',
                        crop: { x: 0, y: 0, width: 0.5, height: 1 }
                      }
                    }
                  ]
                : [{ type: 'text', text: 'private-answer' }],
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
            yield { type: 'done', reason: message.stopReason, message };
          },
          result: async () => message
        } as unknown as Stream;
      };
      return result;
    }
  };
});

test('records fresh usage and cost for reopened Botto sessions without logging content', async () => {
  const { createAgent, transcriptName } = await import('./agent.ts');
  await mkdir(resolve('../../.context'), { recursive: true });
  const directory = await mkdtemp(resolve('../../.context/botto-usage-'));
  const store = new RunStore(join(directory, 'runs'));
  const original = await sharp({
    create: { width: 20, height: 10, channels: 3, background: 'blue' }
  })
    .png()
    .toBuffer();
  cropFixture.data = (
    await sharp(original).extract({ left: 0, top: 0, width: 10, height: 10 }).png().toBuffer()
  ).toString('base64');
  cropFixture.calls = 0;
  cropFixture.terminal = '';
  cropFixture.requests = 0;
  const attachment = { id: 'image', filename: 'image.png', contentType: 'image/png' };
  const imageContext = {
    messages: [{ id: 'message', text: '', attachments: [attachment] }],
    olderContextOmitted: false
  };
  const server = {
    getMessage: vi.fn().mockResolvedValue({ attachments: [attachment] }),
    readAttachment: vi.fn().mockResolvedValue({ data: original, contentType: 'image/png' })
  };
  try {
    await store.init();
    const settings = {
      ...parseSettings(
        '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="anthropic/claude-sonnet-4-6"',
        'botto.toml'
      ),
      directory,
      reasoningLevel: 'low' as const,
      memory: false,
      mcp: false,
      web: false
    };
    const agent = await createAgent(settings);
    const root = task(
      { name: 'Usage fixture', input: Type.Null(), result: Type.Null() },
      async (ctx) => {
        for (let turn = 0; turn < 2; turn++) {
          const texts: string[] = [];
          expect(
            await agent.reply(ctx, {
              identity: ['server', 'bot', 'room', 'thread'],
              prompt: 'private-prompt',
              signal: ctx.signal,
              postUpdate: (text) => {
                texts.push(text);
              },
              tools: [viewImageExtension(server, 'room', imageContext, ctx.signal)],
              context: {
                messages: [],
                olderContextOmitted: false
              },
              loadImages: async () => ({
                imageNotes: [],
                images: [
                  {
                    messageId: 'message',
                    attachmentId: 'image',
                    mimeType: 'image/png',
                    data: 'AQID'
                  }
                ]
              })
            })
          ).toEqual({ action: 'reply', text: 'private-answer' });
          expect(texts).toEqual(['I will take a closer look.']);
        }
        for (const terminal of ['stay_silent', 'react_to_message']) {
          cropFixture.terminal = terminal;
          const before = cropFixture.requests;
          const update = vi.fn();
          const response = await agent.reply(ctx, {
            identity: ['server', 'bot', 'room', 'thread'],
            prompt: 'private-terminal',
            signal: ctx.signal,
            postUpdate: update
          });
          expect(response).toEqual(
            terminal === 'stay_silent' ? { action: 'silent' } : { action: 'react', emoji: 'heart' }
          );
          expect(cropFixture.requests - before).toBe(1);
          expect(update).not.toHaveBeenCalled();
        }
        return null;
      }
    );
    const handle = await store.start('fixture', root, null, 'source');
    expect(await handle.completion).toMatchObject({ ok: true });
    const detail = (await store.get(handle.id))!;
    expect(detail.usage).toEqual({
      input: 88,
      output: 56,
      cacheRead: 40,
      cacheWrite: 24,
      cost: expect.closeTo(0.8)
    });
    expect(detail.events.filter((event) => event.type === 'agent.usage')).toHaveLength(8);
    const journal = await readFile(join(store.directory, `${handle.id}.jsonl`), 'utf8');
    expect(journal).toContain('usage.updated');
    expect(journal).not.toContain('private-prompt');
    expect(journal).not.toContain('private-answer');
    expect(journal).not.toContain('AQID');
    expect(journal).not.toContain(cropFixture.data);
    expect(journal).not.toContain('private-task');
    expect(journal).not.toContain('private-context');
    const transcript = await readFile(
      join(directory, transcriptName(['server', 'bot', 'room', 'thread'])),
      'utf8'
    );
    expect(transcript).not.toContain('AQID');
    expect(transcript).not.toContain(cropFixture.data);
    expect(transcript).not.toContain('private-result');
    expect(transcript).toContain('Image view prepared');
    expect(server.readAttachment).toHaveBeenCalledTimes(2);
  } finally {
    await store.close();
    await rm(directory, { recursive: true, force: true });
  }
});
