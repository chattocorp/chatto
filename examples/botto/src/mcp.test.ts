/** Test native Pi MCP discovery, tool conversion, hooks, and shutdown without an LLM. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager
} from '@earendil-works/pi-coding-agent';
import { mcpExtension, mcpObserver } from './mcp.ts';
import { mcpFixture } from './mcp.fixture.ts';
import { parseSettings } from './settings.ts';

afterEach(() => vi.restoreAllMocks());
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
      const release = vi.fn();
      const acquire = vi.fn(async () => release);
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
          mcpObserver(controller.signal, 'thread-test', acquire)
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
        expect(info).toHaveBeenCalledTimes(1);
        expect(error).toHaveBeenCalledTimes(1);
        expect(JSON.stringify(info.mock.calls)).toContain('mcp__memory__read');
        expect(JSON.stringify(info.mock.calls)).toContain('thread-test');
        expect(JSON.stringify([info.mock.calls, error.mock.calls])).not.toContain('private');
        expect(call).toHaveBeenCalledTimes(2);
        // Cancellation can leave a call without a result event. Shutdown must release it.
        await session.extensionRunner.emitToolCall({
          type: 'tool_call',
          toolCallId: 'unfinished',
          toolName: 'mcp__memory__read',
          input: {}
        });
        expect(acquire).toHaveBeenCalledTimes(3);
        expect(release).toHaveBeenCalledTimes(2);
        controller.abort();
      } finally {
        await session.extensionRunner.emit({ type: 'session_shutdown', reason: 'quit' });
        session.dispose();
      }
      for (const transport of transports) expect(transport.close).toHaveBeenCalled();
      expect(release).toHaveBeenCalledTimes(3);
      expect(error).toHaveBeenCalledTimes(2);
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  }
);
