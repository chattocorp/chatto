/** Exercise the real MCP client with an HTTP fixture; no sockets or model requests. */
import { afterEach, expect, test, vi } from 'vitest';
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager
} from '@earendil-works/pi-coding-agent';
import { connectChattoMcp } from './mcp.ts';
import { parseSettings } from './settings.ts';

const settings = parseSettings(
  '[chatto]\nurl="https://chat.example"\napi_key="test-key"\nmcp=true\n[agent]\nmodel="test/test"',
  'botto.toml'
);
afterEach(() => vi.unstubAllGlobals());

test('discovers every page, preserves schemas, authenticates calls, and reports tool errors', async () => {
  const calls: string[] = [];
  const schema = { type: 'object', properties: { text: { type: 'string' } }, required: ['text'] };
  const fetch = vi.fn(async (url: string | URL, init?: RequestInit) => {
    expect(String(url)).toBe('https://chat.example/mcp');
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer test-key');
    if (init?.method !== 'POST') return new Response(null, { status: 405 });
    expect(init.redirect).toBe('error');
    const request = JSON.parse(String(init.body));
    calls.push(request.method);
    if (request.id === undefined) return new Response(null, { status: 202 });
    let result: unknown;
    switch (request.method) {
      case 'initialize':
        result = {
          protocolVersion: request.params.protocolVersion,
          capabilities: { tools: {} },
          serverInfo: { name: 'fixture', version: '1' }
        };
        break;
      case 'tools/list':
        result = request.params?.cursor
          ? { tools: [{ name: 'post_message', inputSchema: schema }] }
          : {
              tools: [{ name: 'list_rooms', inputSchema: { type: 'object' } }],
              nextCursor: 'page2'
            };
        break;
      case 'tools/call':
        expect(request.params.name).toBe('post_message');
        result =
          request.params.arguments.text === 'fail'
            ? { isError: true, content: [{ type: 'text', text: 'Denied' }] }
            : {
                content: [{ type: 'text', text: 'Posted' }],
                structuredContent: { id: 'new-message' }
              };
        break;
      default:
        throw new Error('Unexpected request');
    }
    return Response.json({ jsonrpc: '2.0', id: request.id, result });
  });
  vi.stubGlobal('fetch', fetch);
  const mcp = await connectChattoMcp(settings, new AbortController().signal);
  try {
    expect(mcp.tools.map((tool) => tool.name)).toEqual(['list_rooms', 'post_message']);
    const tool = mcp.tools[1]!;
    expect(tool.parameters).toEqual(schema);
    // Check the real Pi registry: an empty tools selection would silently hide custom tools.
    const root = resolve('../../.context');
    await mkdir(root, { recursive: true });
    const directory = await mkdtemp(join(root, 'botto-mcp-'));
    try {
      const modelRuntime = await ModelRuntime.create({
        authPath: join(directory, 'auth.json'),
        modelsPath: null,
        refreshOnCreate: false,
        allowModelNetwork: false
      });
      const settingsManager = SettingsManager.inMemory();
      const resourceLoader = new DefaultResourceLoader({
        cwd: directory,
        agentDir: directory,
        settingsManager,
        noExtensions: true,
        noSkills: true,
        noPromptTemplates: true,
        noThemes: true,
        noContextFiles: true
      });
      await resourceLoader.reload();
      const { session } = await createAgentSession({
        cwd: directory,
        agentDir: directory,
        modelRuntime,
        settingsManager,
        resourceLoader,
        sessionManager: SessionManager.inMemory(directory),
        tools: mcp.tools.map((tool) => tool.name),
        customTools: mcp.tools
      });
      try {
        expect(session.getActiveToolNames().sort()).toEqual(['list_rooms', 'post_message']);
      } finally {
        session.dispose();
      }
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
    const execute = (text: string) =>
      tool.execute(
        'call',
        { text },
        undefined,
        undefined,
        {} as Parameters<typeof tool.execute>[4]
      );
    expect(await execute('hello')).toMatchObject({
      details: { structuredContent: { id: 'new-message' } }
    });
    await expect(execute('fail')).rejects.toThrow('Denied');
    expect(calls.filter((method) => method === 'tools/call')).toHaveLength(2);
  } finally {
    await mcp.close();
  }
});

test('rejects an aborted turn before contacting the server', async () => {
  const fetch = vi.fn();
  vi.stubGlobal('fetch', fetch);
  await expect(connectChattoMcp(settings, AbortSignal.abort())).rejects.toThrow();
  expect(fetch).not.toHaveBeenCalled();
});

test('cancellation aborts an HTTP request in progress', async () => {
  const controller = new AbortController();
  let requestSignal: AbortSignal | undefined;
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: unknown, init: RequestInit) => {
      requestSignal = init.signal ?? undefined;
      return new Promise<Response>((_resolve, reject) => {
        requestSignal?.addEventListener('abort', () => reject(requestSignal?.reason), {
          once: true
        });
      });
    })
  );
  const pending = connectChattoMcp(settings, controller.signal);
  const rejected = expect(pending).rejects.toThrow();
  await vi.waitFor(() => expect(requestSignal).toBeDefined());
  controller.abort();
  await rejected;
  expect(requestSignal?.aborted).toBe(true);
});

test('MCP is opt-in and requires a TOML boolean', () => {
  const source = '[chatto]\nurl="https://chat.example"\napi_key="key"\n[agent]\nmodel="test/test"';
  expect(parseSettings(source, 'botto.toml').mcp).toBe(false);
  expect(() =>
    parseSettings(source.replace('[agent]', 'mcp="true"\n[agent]'), 'botto.toml')
  ).toThrow('boolean');
});
