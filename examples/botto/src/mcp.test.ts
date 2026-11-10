/** Test native Pi MCP discovery, tool conversion, hooks, and shutdown without an LLM. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager,
  type McpTransportFactory
} from '@earendil-works/pi-coding-agent';
import { EXA_MCP_URL, mcpExtension, memoryLockExtension, mcpServers } from './mcp.ts';
import { mcpFixture } from './mcp.fixture.ts';
import { parseSettings } from './settings.ts';
import { createAgentSession as createRunlingSession } from 'runling';
import { adminOnlyBatch } from './authorization.ts';

test.each([
  { authors: ['admin'], expectedAdmin: true },
  { authors: ['reader'], expectedAdmin: false },
  { authors: ['admin', 'reader'], expectedAdmin: false },
  { authors: [], expectedAdmin: false },
  { authors: [''], expectedAdmin: false },
  { authors: ['admin', 'other-admin'], expectedAdmin: true }
])('MCP enforces current batch authority for $authors', async ({ authors, expectedAdmin }) => {
  const directory = await mkdtemp(resolve('../../.context/botto-mcp-access-'));
  const settings = parseSettings(
    '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="test/model"\n[mcp.servers.github]\ncommand="unused"\ntools=["issue_read"]\nadmin_tools=["issue_write"]',
    'botto.toml'
  );
  const privileged = adminOnlyBatch(authors, ['admin', 'other-admin']);
  expect(privileged).toBe(expectedAdmin);
  const call = vi.fn(async () => ({ content: [] }));
  const transport = mcpFixture(
    ['issue_read', 'issue_write', 'new_write'].map((name) => ({
      name,
      inputSchema: { type: 'object' }
    })),
    call
  );
  const modelRuntime = await ModelRuntime.create({
    authPath: join(directory, 'auth.json'),
    modelsPath: null,
    refreshOnCreate: false,
    allowModelNetwork: false
  });
  const settingsManager = SettingsManager.inMemory();
  const loader = new DefaultResourceLoader({
    cwd: directory,
    agentDir: directory,
    settingsManager,
    noExtensions: true,
    noSkills: true,
    noPromptTemplates: true,
    noThemes: true,
    noContextFiles: true,
    extensionFactories: [mcpExtension(settings, undefined, () => transport, privileged)]
  });
  await loader.reload();
  const { session } = await createAgentSession({
    cwd: directory,
    agentDir: directory,
    modelRuntime,
    settingsManager,
    resourceLoader: loader,
    sessionManager: SessionManager.inMemory(directory),
    noTools: 'builtin'
  });
  try {
    await session.bindExtensions({});
    const expected = [
      'mcp__github__issue_read',
      ...(privileged ? ['mcp__github__issue_write'] : [])
    ];
    await vi.waitFor(() => expect(session.getActiveToolNames().sort()).toEqual(expected));
    for (const name of ['issue_read', 'issue_write', 'new_write']) {
      const toolName = `mcp__github__${name}`;
      const result = await session.extensionRunner.emitToolCall({
        type: 'tool_call',
        toolCallId: name,
        toolName,
        input: {}
      });
      if (expected.includes(toolName)) {
        expect(result?.block).not.toBe(true);
        const tool = session.agent.state.tools.find((tool) => tool.name === toolName)!;
        await tool.execute(name, {}, new AbortController().signal);
      } else {
        expect(result?.block).toBe(true);
        expect(session.agent.state.tools.some((tool) => tool.name === toolName)).toBe(false);
      }
    }
    expect(call).toHaveBeenCalledTimes(expected.length);
  } finally {
    await session.extensionRunner.emit({ type: 'session_shutdown', reason: 'quit' });
    session.dispose();
    await rm(directory, { recursive: true, force: true });
  }
});

afterEach(() => vi.restoreAllMocks());

test('Runling starts native MCP extensions and awaits their shutdown without a model request', async () => {
  await mkdir(resolve('../../.context'), { recursive: true });
  const directory = await mkdtemp(resolve('../../.context/botto-runling-mcp-'));
  const settings = parseSettings(
    '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="anthropic/claude-sonnet-4-6"\n[mcp.servers.custom]\nurl="https://example.com/mcp"',
    'botto.toml'
  );
  const transport = mcpFixture([{ name: 'read', inputSchema: { type: 'object' } }], async () => ({
    content: []
  }));
  const close = vi.spyOn(transport, 'close');
  const create = vi.fn<McpTransportFactory>(() => transport);
  try {
    const session = await createRunlingSession({
      cwd: directory,
      model: 'anthropic/claude-sonnet-4-6',
      output: 'text',
      tools: 'extensions',
      toolExecution: 'sequential',
      logContent: false,
      resources: {
        agentDir: directory,
        extensions: false,
        skills: false,
        contextFiles: false,
        promptTemplates: false,
        themes: false
      },
      extensions: [mcpExtension(settings, undefined, create)]
    });
    try {
      await vi.waitFor(() => expect(create).toHaveBeenCalledOnce());
      expect(create.mock.calls[0]?.[0]).toMatchObject({ name: 'custom' });
      expect(close).not.toHaveBeenCalled();
    } finally {
      await session.dispose();
    }
    expect(close).toHaveBeenCalled();
    const calls = close.mock.calls.length;
    await session.dispose();
    expect(close).toHaveBeenCalledTimes(calls);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
test.each([false, true])(
  'native MCP opt-in (%s), structured results, logs, and shutdown',
  async (enabled) => {
    const info = vi.spyOn(console, 'info').mockImplementation(() => {});
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const root = resolve('../../.context');
    await mkdir(root, { recursive: true });
    const directory = await mkdtemp(join(root, 'botto-native-mcp-'));
    const settings = parseSettings(
      '[chatto]\nurl="https://example.com"\napi_key="private-key"\nmcp=true\n[agent]\nmodel="test/test"',
      'botto.toml'
    );
    settings.mcp = enabled;
    const call = vi.fn(async (_name: string, args: Record<string, unknown>) =>
      args.fail
        ? { isError: true, content: [{ type: 'text' as const, text: 'private failure' }] }
        : { content: [], structuredContent: { body: 'private memory' } }
    );
    const transports: Array<ReturnType<typeof mcpFixture>> = [];
    const createTransport = vi.fn((entry) => {
      expect(entry.config.exposure).toBe('direct');
      if (entry.name === 'chatto')
        expect(entry.config.headers).toEqual({ Authorization: 'Bearer private-key' });
      const transport = mcpFixture(
        ['read', 'post_message'].map((name) => ({
          name,
          inputSchema: { type: 'object' as const }
        })),
        call
      );
      vi.spyOn(transport, 'close');
      transports.push(transport);
      return transport;
    });
    try {
      const modelRuntime = await ModelRuntime.create({
        authPath: join(directory, 'auth.json'),
        modelsPath: null,
        refreshOnCreate: false,
        allowModelNetwork: false
      });
      const settingsManager = SettingsManager.inMemory();
      const controller = new AbortController();
      let pending = Promise.resolve();
      const release = vi.fn();
      const acquire = vi.fn(async () => {
        const previous = pending;
        const gate = Promise.withResolvers<void>();
        pending = previous.then(() => gate.promise);
        await previous;
        return () => {
          release();
          gate.resolve();
        };
      });
      const loader = new DefaultResourceLoader({
        cwd: directory,
        agentDir: directory,
        settingsManager,
        noExtensions: true,
        noSkills: true,
        noPromptTemplates: true,
        noThemes: true,
        noContextFiles: true,
        extensionFactories: [
          mcpExtension(settings, enabled ? directory : undefined, createTransport),
          memoryLockExtension(controller.signal, acquire)
        ]
      });
      await loader.reload();
      const { session } = await createAgentSession({
        cwd: directory,
        agentDir: directory,
        modelRuntime,
        settingsManager,
        resourceLoader: loader,
        sessionManager: SessionManager.inMemory(directory),
        noTools: 'builtin'
      });
      try {
        await session.bindExtensions({});
        if (!enabled) {
          expect(session.getActiveToolNames()).toEqual([]);
          expect(createTransport).not.toHaveBeenCalled();
          return;
        }
        await vi.waitFor(() =>
          expect(session.getActiveToolNames().sort()).toEqual([
            'mcp__chatto__post_message',
            'mcp__chatto__read',
            'mcp__memory__post_message',
            'mcp__memory__read'
          ])
        );
        const execute = async (id: string, fail = false) => {
          const toolName = 'mcp__memory__read';
          const input = { fail, secret: 'private-argument' };
          await session.extensionRunner.emitToolCall({
            type: 'tool_call',
            toolCallId: id,
            toolName,
            input
          });
          const tool = session.agent.state.tools.find((tool) => tool.name === toolName)!;
          const result = await tool.execute(id, input, controller.signal);
          await session.extensionRunner.emitToolResult({
            type: 'tool_result',
            toolCallId: id,
            toolName,
            input,
            content: result.content,
            details: result.details,
            isError: result.isError === true
          });
          return result;
        };
        expect(JSON.stringify(await execute('success'))).toContain('private memory');
        expect((await execute('failure', true)).isError).toBe(true);
        expect(acquire).toHaveBeenCalledTimes(2);
        expect(release).toHaveBeenCalledTimes(2);
        expect(info).not.toHaveBeenCalled();
        expect(error).not.toHaveBeenCalled();
        expect(JSON.stringify([info.mock.calls, error.mock.calls])).not.toContain('private');
        expect(call).toHaveBeenCalledTimes(2);

        // Exercise Pi's actual batch loop. Its parallel mode prepares all calls
        // before execution, which deadlocks a lock acquired in tool_call hooks.
        session.agent.toolExecution = 'sequential';
        const model = modelRuntime.getModel('anthropic', 'claude-sonnet-4-6')!;
        session.agent.state.model = model;
        session.agent.getApiKey = () => 'unused-offline-fixture';
        let requests = 0;
        session.agent.streamFunction = () => {
          type Stream = Awaited<ReturnType<typeof session.agent.streamFunction>>;
          type Reply = Awaited<ReturnType<Stream['result']>>;
          const first = requests++ === 0;
          const message: Reply = {
            role: 'assistant',
            api: 'anthropic-messages',
            provider: model.provider,
            model: model.id,
            timestamp: Date.now(),
            stopReason: first ? 'toolUse' : 'stop',
            content: first
              ? ['batch-one', 'batch-two'].map((id) => ({
                  type: 'toolCall' as const,
                  id,
                  name: 'mcp__memory__read',
                  arguments: {}
                }))
              : [{ type: 'text', text: 'Done' }],
            usage: {
              input: 0,
              output: 0,
              cacheRead: 0,
              cacheWrite: 0,
              totalTokens: 0,
              cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 }
            }
          };
          // Only the provider stream is synthetic; Pi owns batching and every hook.
          return {
            async *[Symbol.asyncIterator]() {
              yield { type: 'done', reason: message.stopReason, message };
            },
            result: async () => message
          } as unknown as Stream;
        };
        const running = session.agent.prompt('Read both memory documents.');
        try {
          await vi.waitFor(() => expect(call).toHaveBeenCalledTimes(4), { timeout: 1000 });
          await running;
        } finally {
          session.agent.abort();
        }
        expect(requests).toBe(2);
        expect(release).toHaveBeenCalledTimes(4);
        // Cancellation can leave a call without a result event. Shutdown must release it.
        await session.extensionRunner.emitToolCall({
          type: 'tool_call',
          toolCallId: 'unfinished',
          toolName: 'mcp__memory__read',
          input: {}
        });
        expect(acquire).toHaveBeenCalledTimes(5);
        expect(release).toHaveBeenCalledTimes(4);
        controller.abort();
      } finally {
        await session.extensionRunner.emit({ type: 'session_shutdown', reason: 'quit' });
        session.dispose();
      }
      for (const transport of transports) expect(transport.close).toHaveBeenCalled();
      expect(release).toHaveBeenCalledTimes(5);
      expect(error).not.toHaveBeenCalled();
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  }
);

test('Exa web tools are opt-in and send an optional key only in a header', () => {
  const settings = parseSettings(
    '[chatto]\nurl="https://example.com"\napi_key="private-key"\n[agent]\nmodel="test/test"',
    'botto.toml'
  );
  expect(mcpServers(settings)).toEqual([]);
  const exa = (exaApiKey?: string) =>
    mcpServers({ ...settings, web: true, exaApiKey }).find((server) => server.name === 'exa');
  expect(exa()?.config).toEqual({ url: EXA_MCP_URL, headers: {}, exposure: 'direct' });
  expect(exa('exa-secret')?.config).toMatchObject({
    url: EXA_MCP_URL,
    headers: { 'x-api-key': 'exa-secret' }
  });
});
