/** Verify generic child isolation, cancellation, concurrency, and safe failures. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { createTaskContext } from 'runling';
import { afterEach, expect, test, vi } from 'vitest';
import { delegateExtension } from './delegate.ts';
import { parseSettings } from './settings.ts';

const mock = vi.hoisted(() => ({ create: vi.fn(), mcp: vi.fn(() => () => {}) }));
vi.mock('runling', async (original) => ({
  ...(await original<typeof import('runling')>()),
  createAgentSession: mock.create
}));
vi.mock('./mcp.ts', () => ({ mcpExtension: mock.mcp }));
afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

async function setup(web = false) {
  vi.spyOn(console, 'info').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
  const session = {
    run: vi.fn().mockResolvedValue({ summary: 'A useful answer.' }),
    dispose: vi.fn().mockResolvedValue(undefined),
    [Symbol.asyncDispose]: vi.fn().mockResolvedValue(undefined)
  };
  mock.create.mockResolvedValue(session);
  const controller = new AbortController();
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  const settings = {
    ...parseSettings(
      '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="test/model"',
      'botto.toml'
    ),
    web,
    systemPrompt: 'You are a patient gardening expert.',
    reasoningLevel: 'low' as const
  };
  await delegateExtension(
    createTaskContext(),
    settings,
    controller.signal
  )({ registerTool } as unknown as ExtensionAPI);
  const tool = registerTool.mock.calls[0]![0];
  const call = (signal?: AbortSignal) =>
    tool.execute(
      'call',
      {
        task: 'Compare these approaches',
        context: 'Explicit private context'
      },
      signal,
      undefined,
      {} as ExtensionToolContext
    );
  return { call, session, controller };
}

test('opens an isolated session with explicit context and a bounded result', async () => {
  const { call, session } = await setup();
  expect(JSON.stringify((await call()).content)).toContain('A useful answer.');
  expect(mock.create.mock.calls[0]![0]).toMatchObject({
    model: 'test/model',
    thinkingLevel: 'low',
    output: 'text',
    logContent: false,
    tools: 'extensions',
    extensions: [expect.any(Function)],
    resources: {
      extensions: false,
      skills: [expect.stringContaining('.agents/skills')],
      contextFiles: false
    }
  });
  expect(mock.create.mock.calls[0]![0].sessionFile).toBeUndefined();
  expect(mock.create.mock.calls[0]![0].systemPrompt).toContain(
    '# Instance instructions\n\nYou are a patient gardening expert.'
  );
  expect(mock.create.mock.calls[0]![0].systemPrompt).not.toContain(
    'posted to this thread automatically'
  );
  expect(JSON.parse(session.run.mock.calls[0]![1])).toEqual({
    task: 'Compare these approaches',
    context: 'Explicit private context'
  });
  expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce();
  session.run.mockResolvedValueOnce({ summary: 'x'.repeat(18000) });
  const result = JSON.stringify((await call()).content);
  expect(result).toContain('[Result truncated.]');
  expect(result.length).toBeLessThan(16200);
});

test('enables only web MCP, with no memory directory or Chatto tools', async () => {
  const { call } = await setup(true);
  await call();
  expect(mock.mcp).toHaveBeenCalledWith(expect.objectContaining({ mcp: false, web: true }));
  expect(mock.mcp.mock.calls[0]).toHaveLength(1);
  expect(mock.create.mock.calls[0]![0].extensions).toHaveLength(2);
});

test('parallel assignments create independent sessions and return to their callers', async () => {
  const { call, session } = await setup();
  const second = {
    ...session,
    run: vi.fn().mockResolvedValue({ summary: 'Second result' }),
    [Symbol.asyncDispose]: vi.fn()
  };
  mock.create.mockResolvedValueOnce(session).mockResolvedValueOnce(second);
  const results = await Promise.all([call(), call()]);
  expect(JSON.stringify(results[0].content)).toContain('A useful answer.');
  expect(JSON.stringify(results[1].content)).toContain('Second result');
  expect(mock.create).toHaveBeenCalledTimes(2);
  expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce();
  expect(second[Symbol.asyncDispose]).toHaveBeenCalledOnce();
});

test.each(['tool', 'turn'] as const)(
  '%s cancellation closes and disposes the child',
  async (source) => {
    const { call, session, controller } = await setup();
    const tool = new AbortController();
    const entered = Promise.withResolvers<void>();
    session.run.mockImplementationOnce(async (_ctx, _prompt, { signal }) => {
      entered.resolve();
      await new Promise((_resolve, reject) =>
        signal.addEventListener('abort', () => reject(signal.reason), { once: true })
      );
    });
    const pending = expect(call(tool.signal)).rejects.toThrow();
    await entered.promise;
    (source === 'tool' ? tool : controller).abort();
    await pending;
    await vi.waitFor(() => expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce());
    expect(session.dispose).toHaveBeenCalledOnce();
  }
);

test('provider errors and empty reports are sanitized and disposed', async () => {
  const { call, session } = await setup();
  session.run.mockRejectedValueOnce(new Error('private provider details'));
  await expect(call()).rejects.toThrow('Delegated task could not be completed.');
  session.run.mockResolvedValueOnce({ summary: ' ' });
  await expect(call()).rejects.toThrow('Delegated task could not be completed.');
  expect(session[Symbol.asyncDispose]).toHaveBeenCalledTimes(2);
});
