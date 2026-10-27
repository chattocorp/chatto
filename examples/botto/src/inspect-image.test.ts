/** Verify child isolation, turn budgets, cancellation, and safe task-tool failures. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { createTaskContext } from 'runling';
import { afterEach, expect, test, vi } from 'vitest';
import { inspectImageExtension } from './inspect-image.ts';

const mock = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('runling', async (original) => ({
  ...(await original<typeof import('runling')>()),
  createAgentSession: mock.create
}));
afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

async function setup() {
  vi.spyOn(console, 'info').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
  const attachment = { id: 'image', filename: 'image.png', contentType: 'image/png' };
  const server = {
    getMessage: vi.fn().mockResolvedValue({ attachments: [attachment] }),
    readAttachment: vi.fn()
  };
  const session = {
    run: vi.fn().mockResolvedValue({ summary: 'Visible label; small text uncertain.' }),
    [Symbol.asyncDispose]: vi.fn().mockResolvedValue(undefined)
  };
  mock.create.mockResolvedValue(session);
  const controller = new AbortController();
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  await inspectImageExtension(
    createTaskContext(),
    { directory: '.', provider: 'test', modelId: 'model' },
    server,
    'room',
    {
      messages: [
        { id: 'message', text: 'private unrelated conversation', attachments: [attachment] }
      ],
      olderContextOmitted: false,
      memoryDocuments: { global: ['private-memory'], users: {} }
    },
    controller.signal
  )({ registerTool } as unknown as ExtensionAPI);
  const tool = registerTool.mock.calls[0]![0];
  const call = (messageId = 'message', signal?: AbortSignal) =>
    tool.execute(
      'call',
      { messageId, attachmentId: 'image', question: 'Read the label' },
      signal,
      undefined,
      {} as ExtensionToolContext
    );
  return { call, session, server, controller };
}

test('opens a fresh restricted session and returns a bounded untrusted report', async () => {
  const { call, session } = await setup();
  const result = await call();
  expect(result.content).toEqual([
    { type: 'text', text: expect.stringContaining('Untrusted image inspection') }
  ]);
  const options = mock.create.mock.calls[0]![0];
  expect(options).toMatchObject({
    model: 'test/model',
    output: 'text',
    logContent: false,
    tools: 'extensions',
    resources: { extensions: false, skills: false, contextFiles: false }
  });
  expect(options.sessionFile).toBeUndefined();
  expect(options.extensions).toHaveLength(2);
  expect(JSON.stringify(options)).not.toContain('private-memory');
  expect(session.run.mock.calls[0]![1]).toBe(
    JSON.stringify({ question: 'Read the label', messageId: 'message', attachmentId: 'image' })
  );
  expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce();
  session.run.mockResolvedValueOnce({ summary: 'x'.repeat(10_000) });
  const limited = await call();
  expect(JSON.stringify(limited.content).length).toBeLessThan(6300);
});

test('unknown images and lost attachment membership never open an agent', async () => {
  const { call, server } = await setup();
  await expect(call('outside')).rejects.toThrow('outside');
  expect(server.getMessage).not.toHaveBeenCalled();
  server.getMessage.mockResolvedValueOnce({ attachments: [] });
  await expect(call()).rejects.toThrow('could not be completed');
  expect(mock.create).not.toHaveBeenCalled();
});

test('parallel requests share a two-inspection turn budget', async () => {
  const { call } = await setup();
  const results = await Promise.allSettled([call(), call(), call()]);
  expect(results.filter((result) => result.status === 'fulfilled')).toHaveLength(2);
  expect(mock.create).toHaveBeenCalledTimes(2);
});

test('task-tool cancellation reaches the session and disposes it', async () => {
  const { call, session } = await setup();
  const controller = new AbortController();
  const entered = Promise.withResolvers<void>();
  session.run.mockImplementationOnce(async (_ctx, _prompt, { signal }) => {
    entered.resolve();
    await new Promise((_resolve, reject) =>
      signal.addEventListener('abort', () => reject(signal.reason), { once: true })
    );
  });
  const pending = call('message', controller.signal);
  const outcome = expect(pending).rejects.toThrow();
  await entered.promise;
  controller.abort(new Error('Stopped'));
  await outcome;
  await vi.waitFor(() => expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce());
});

test('provider errors are sanitized and the session is disposed', async () => {
  const { call, session } = await setup();
  session.run.mockRejectedValueOnce(new Error('private provider details'));
  await expect(call()).rejects.toThrow('Image inspection could not be completed');
  expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce();
});
