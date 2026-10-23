/** Check the Pi boundary: tool isolation, transcript identity, errors, and cancellation. */
import { mkdtemp, mkdir, rm } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';

const pi = vi.hoisted(() => ({
  create: vi.fn(),
  open: vi.fn(),
  getModel: vi.fn(),
  refresh: vi.fn()
}));
vi.mock('@earendil-works/pi-coding-agent', () => ({
  createAgentSession: pi.create,
  ModelRuntime: { create: async () => ({ getModel: pi.getModel, refresh: pi.refresh }) },
  SettingsManager: { inMemory: () => ({}) },
  SessionManager: { open: pi.open },
  DefaultResourceLoader: class {
    constructor(readonly options: unknown) {}
    async reload() {}
  }
}));

import { createAgent, transcriptName } from './agent.ts';
import { parseSettings } from './settings.ts';
import { log } from './log.ts';
import { connectChattoMcp } from './mcp.ts';
import { createMemory } from './memory.ts';
vi.mock('./memory.ts', () => ({ createMemory: vi.fn(() => vi.fn()) }));
vi.mock('./mcp.ts', () => ({ connectChattoMcp: vi.fn() }));
vi.mock('./log.ts', () => ({ log: vi.fn() }));

const directories: string[] = [];
beforeEach(() => {
  pi.getModel.mockReset().mockReturnValue({ id: 'test-model' });
  pi.refresh.mockReset().mockResolvedValue(undefined);
});
afterEach(async () => {
  vi.unstubAllEnvs();
  vi.clearAllMocks();
  await Promise.all(
    directories.splice(0).map((directory) => rm(directory, { recursive: true, force: true }))
  );
});

test('refreshes only the selected provider when its model is missing locally', async () => {
  vi.stubEnv('PI_OFFLINE', undefined);
  pi.getModel.mockReturnValueOnce(undefined);
  await setup();
  expect(pi.refresh).toHaveBeenCalledWith(
    expect.objectContaining({
      providers: ['test'],
      allowNetwork: true,
      force: true
    })
  );
  expect(pi.getModel).toHaveBeenCalledTimes(2);
});

test('reports an unknown model and respects offline mode', async () => {
  vi.stubEnv('PI_OFFLINE', '1');
  pi.getModel.mockReturnValue(undefined);
  await expect(setup()).rejects.toThrow('agent.model was not found');
  expect(pi.refresh).not.toHaveBeenCalled();
});

async function setup(mcp = false, memory = false) {
  const root = resolve('../../.context');
  await mkdir(root, { recursive: true });
  const directory = await mkdtemp(join(root, 'botto-test-'));
  directories.push(directory);
  const session = {
    sessionId: 'persisted-session',
    prompt: vi.fn(async () => {}),
    abort: vi.fn(async () => {}),
    dispose: vi.fn(),
    messages: [{ role: 'assistant', stopReason: 'stop' }],
    getLastAssistantText: () => 'Hello'
  };
  pi.create.mockResolvedValue({ session });
  const agent = await createAgent({
    ...parseSettings(
      '[chatto]\nurl="https://chat.example.com"\napi_key="${BOTTO_CHATTO_API_KEY}"\n[agent]\nmodel="test/test"\nname="Test Bot"\nsystem_prompt="Be helpful."',
      'botto.toml',
      { BOTTO_CHATTO_API_KEY: 'unused' }
    ),
    directory,
    mcp,
    memory
  });
  return { agent, session, directory };
}

test('uses an isolated, tool-free session and a stable transcript path', async () => {
  const { agent, session, directory } = await setup();
  const identity = ['server', 'bot', 'room', 'thread'];
  expect(await agent.reply(identity, 'hello', new AbortController().signal)).toBe('Hello');
  expect(pi.create).toHaveBeenCalledWith(
    expect.objectContaining({
      tools: [],
      resourceLoader: expect.objectContaining({
        options: expect.objectContaining({
          noExtensions: true,
          noSkills: true,
          noContextFiles: true,
          noPromptTemplates: true
        })
      })
    })
  );
  expect(pi.open).toHaveBeenCalledWith(
    join(directory, transcriptName(identity)),
    directory,
    directory
  );
  expect(transcriptName(identity)).not.toBe(
    transcriptName(['other-server', 'bot', 'room', 'thread'])
  );
  expect(session.dispose).toHaveBeenCalledOnce();
  expect(connectChattoMcp).not.toHaveBeenCalled();
  expect(log).toHaveBeenCalledWith('info', 'Pi session resumed', {
    threadId: 'thread',
    sessionId: 'persisted-session',
    historyMessages: 1
  });
  const options = pi.create.mock.calls[0]![0];
  expect(options.resourceLoader.options.systemPromptOverride()).toContain('Your name is Test Bot.');
  expect(options.resourceLoader.options.systemPromptOverride()).toContain('Be helpful.');
});

test('passes MCP tools to Pi and closes the connection even when session creation fails', async () => {
  const close = vi.fn(async () => {});
  const tools: Awaited<ReturnType<typeof connectChattoMcp>>['tools'] = [
    {
      name: 'post_message',
      label: 'Post message',
      description: 'Post a message',
      parameters: { type: 'object' } as Awaited<
        ReturnType<typeof connectChattoMcp>
      >['tools'][number]['parameters'],
      execute: async () => ({ content: [{ type: 'text', text: 'Posted' }], details: {} })
    }
  ];
  vi.mocked(connectChattoMcp).mockResolvedValue({ tools, close });
  const { agent } = await setup(true);
  await agent.reply(['thread'], 'hello', new AbortController().signal);
  expect(pi.create.mock.calls.at(-1)![0].customTools).toEqual(tools);
  expect(pi.create.mock.calls.at(-1)![0].tools).toEqual(['post_message']);
  expect(close).toHaveBeenCalledOnce();
  pi.create.mockRejectedValueOnce(new Error('session failed'));
  await expect(agent.reply(['thread'], 'hello', new AbortController().signal)).rejects.toThrow(
    'session failed'
  );
  expect(close).toHaveBeenCalledTimes(2);
});

test('supplies memory identity and workflow and closes both MCP connections on Pi failure', async () => {
  const close = vi.fn(async () => {});
  const chattoClose = vi.fn(async () => {});
  const client = new Client({ name: 'memory-test', version: '1' });
  vi.spyOn(client, 'getInstructions').mockReturnValue('Read the global and user documents.');
  const open = vi.fn(async () => ({
    tools: [],
    close,
    client,
    documents: { global: 'global-doc', users: { human: 'user-doc' } }
  }));
  vi.mocked(createMemory).mockReturnValueOnce(open);
  vi.mocked(connectChattoMcp).mockResolvedValueOnce({ tools: [], close: chattoClose });
  const { agent, session } = await setup(true, true);
  session.prompt.mockRejectedValueOnce(new Error('model failed'));
  const signal = new AbortController().signal;
  await expect(
    agent.reply(['server', 'bot', 'room', 'thread'], 'hello', signal, undefined, ['human'])
  ).rejects.toThrow('model failed');
  expect(open).toHaveBeenCalledExactlyOnceWith(
    ['server', 'bot', 'room', 'thread'],
    ['human'],
    signal
  );
  const prompt = pi.create.mock.calls.at(-1)![0].resourceLoader.options.systemPromptOverride();
  expect(prompt).toContain('user-doc');
  expect(prompt).toContain('Read the global and user documents.');
  expect(close).toHaveBeenCalledOnce();
  expect(chattoClose).toHaveBeenCalledOnce();
});

test('does not post stale assistant text after a model error', async () => {
  const { agent, session } = await setup();
  session.messages[0]!.stopReason = 'error';
  await expect(agent.reply(['thread'], 'hello', new AbortController().signal)).rejects.toThrow(
    'did not complete'
  );
  expect(session.dispose).toHaveBeenCalledOnce();
});

test('passes only the addressed prompt to Pi and installs the request-only snapshot extension', async () => {
  const { agent, session } = await setup();
  const prompt = JSON.stringify({
    currentMessages: [{ id: 'new', author: 'human', text: 'Question' }]
  });
  await agent.reply(['thread'], prompt, new AbortController().signal, {
    messages: [{ id: 'old', author: 'human', text: 'Context snapshot' }],
    olderContextOmitted: false
  });
  expect(session.prompt).toHaveBeenCalledExactlyOnceWith(prompt);
  const options = pi.create.mock.calls[0]![0];
  expect(options.resourceLoader.options.extensionFactories).toHaveLength(1);
  expect(options.resourceLoader.options.noExtensions).toBe(true);
});

test('logs a new session when no messages were restored', async () => {
  const { agent, session } = await setup();
  session.messages = [];
  session.prompt.mockImplementation(async () => {
    session.messages.push({ role: 'assistant', stopReason: 'stop' });
  });
  await agent.reply(['thread'], 'hello', new AbortController().signal);
  expect(log).toHaveBeenCalledWith('info', 'Pi session created', {
    threadId: 'thread',
    sessionId: 'persisted-session',
    historyMessages: 0
  });
});

test('aborts active model work and disposes its session on shutdown', async () => {
  const { agent, session } = await setup();
  const pending = Promise.withResolvers<void>();
  session.prompt.mockImplementation(() => pending.promise);
  session.abort.mockImplementation(async () => {
    pending.resolve();
  });
  const controller = new AbortController();
  const running = agent.reply(['thread'], 'hello', controller.signal);
  const rejected = expect(running).rejects.toThrow();
  await vi.waitFor(() => expect(session.prompt).toHaveBeenCalledOnce());
  controller.abort();
  await rejected;
  expect(session.abort).toHaveBeenCalled();
  expect(session.dispose).toHaveBeenCalledOnce();
});
