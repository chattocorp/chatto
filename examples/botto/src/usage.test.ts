/** Exercise Botto, Runling, Pi, and the journal with only the provider stream replaced. */
import { mkdir, mkdtemp, readFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { expect, test, vi } from 'vitest';
import { task, Type } from 'runling';
import { RunStore } from '../../../packages/runling/src/lib/server/run-store.ts';
import { parseSettings } from './settings.ts';
import sharp from 'sharp';
import { inspectImageExtension } from './inspect-image.ts';

const cropFixture = vi.hoisted(() => ({ data: '', calls: 0 }));

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
      agent.streamFunction = (_model, context) => {
        const inspector = result.session.getActiveToolNames().includes('view_image');
        if (inspector) expect(result.session.getActiveToolNames()).toEqual(['view_image']);
        const needsCrop = context.messages.at(-1)?.role === 'user';
        if (!needsCrop) {
          const result = context.messages.at(-1);
          expect(result?.role).toBe('toolResult');
          expect(
            result &&
              'content' in result &&
              Array.isArray(result.content) &&
              result.content.some((part) =>
                inspector
                  ? part.type === 'image' && part.data === cropFixture.data
                  : part.type === 'text' && part.text.includes('private-inspection')
              )
          ).toBe(true);
        } else {
          // Reopened history keeps only the receipt, never a previous turn's crop.
          expect(
            context.messages
              .filter((message) => message.role === 'toolResult')
              .every((message) => message.content.every((part) => part.type !== 'image'))
          ).toBe(true);
        }
        expect(
          context.messages.some(
            (message) =>
              message.role === 'user' &&
              Array.isArray(message.content) &&
              message.content.some(
                (part) => part.type === 'image' && (inspector || part.data === 'AQID')
              )
          )
        ).toBe(true);
        type Stream = Awaited<ReturnType<typeof agent.streamFunction>>;
        const message: Awaited<ReturnType<Stream['result']>> = {
          role: 'assistant',
          api: 'anthropic-messages',
          provider: 'anthropic',
          model: 'claude-sonnet-4-6',
          timestamp: Date.now(),
          stopReason: needsCrop ? 'toolUse' : 'stop',
          content: needsCrop
            ? [
                {
                  type: 'toolCall',
                  id: `crop-${++cropFixture.calls}`,
                  name: inspector ? 'view_image' : 'inspect_image',
                  arguments: inspector
                    ? {
                        messageId: 'message',
                        attachmentId: 'image',
                        crop: { x: 0, y: 0, width: 0.5, height: 1 }
                      }
                    : { messageId: 'message', attachmentId: 'image', question: 'private-question' }
                }
              ]
            : [{ type: 'text', text: inspector ? 'private-inspection' : 'private-answer' }],
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
      memory: false,
      mcp: false,
      web: false
    };
    const agent = await createAgent(settings);
    const root = task(
      { name: 'Usage fixture', input: Type.Null(), result: Type.Null() },
      async (ctx) => {
        for (let turn = 0; turn < 2; turn++) {
          expect(
            await agent.reply(ctx, {
              identity: ['server', 'bot', 'room', 'thread'],
              prompt: 'private-prompt',
              signal: ctx.signal,
              tools: [
                inspectImageExtension(ctx, settings, server, 'room', imageContext, ctx.signal)
              ],
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
          ).toBe('private-answer');
        }
        return null;
      }
    );
    const handle = await store.start('fixture', root, null, 'source');
    expect((await handle.completion).ok).toBe(true);
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
    expect(journal).not.toContain('private-question');
    expect(journal).not.toContain('private-inspection');
    expect(
      detail.events.filter(
        (event) => event.type === 'step.started' && event.label === 'Inspect image'
      )
    ).toHaveLength(2);
    const transcript = await readFile(
      join(directory, transcriptName(['server', 'bot', 'room', 'thread'])),
      'utf8'
    );
    expect(transcript).not.toContain('AQID');
    expect(transcript).not.toContain(cropFixture.data);
    expect(transcript).toContain('private-inspection');
    expect(transcript).not.toContain('Image view prepared');
    expect(server.readAttachment).toHaveBeenCalledTimes(4);
  } finally {
    await store.close();
    await rm(directory, { recursive: true, force: true });
  }
});
