/** Exercise nonblocking delegation through real Runling task channels and mocked model sessions. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { createTaskContext } from 'runling';
import { afterEach, expect, test, vi } from 'vitest';
import { delegateExtension, Subagents } from './delegate.ts';
import { parseSettings, type Settings } from './settings.ts';

const mock = vi.hoisted(() => ({ create: vi.fn(), mcp: vi.fn(() => () => {}) }));
vi.mock('runling', async (original) => ({
  ...(await original<typeof import('runling')>()),
  createAgentSession: mock.create
}));
vi.mock('./mcp.ts', () => ({ mcpExtension: mock.mcp }));
const owners: Subagents[] = [];
afterEach(async () => {
  await Promise.all(owners.splice(0).map((owner) => owner.close()));
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

async function setup(web = false, subagents: Settings['subagents'] = {}) {
  vi.spyOn(console, 'info').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
  const session = {
    runOutcome: vi.fn().mockResolvedValue({ outcome: 'completed', summary: 'A useful answer.' }),
    steer: vi.fn().mockResolvedValue(true),
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
    subagents,
    systemPrompt: 'You are a patient gardening expert.',
    reasoningLevel: 'low' as const
  };
  const notify = vi.fn();
  const children = new Subagents(createTaskContext(), notify);
  owners.push(children);
  const complete = vi.fn().mockResolvedValue(undefined);
  await delegateExtension(
    children,
    settings,
    controller.signal,
    complete
  )({ registerTool } as unknown as ExtensionAPI);
  const invoke = (name: string, input: Record<string, unknown>) =>
    registerTool.mock.calls
      .find(([tool]) => tool.name === name)![0]
      .execute('call', input, undefined, undefined, {} as ExtensionToolContext);
  const call = async (agent?: string) => {
    const reply = await invoke('delegate', {
      task: 'Compare these approaches',
      context: 'Explicit private context',
      ...(agent ? { agent } : {})
    });
    const text = reply.content[0];
    if (text?.type !== 'text') throw new Error('Expected text');
    return JSON.parse(text.text).id as string;
  };
  const finish = async () => {
    await vi.waitFor(() => expect(notify).toHaveBeenCalled());
    let next;
    while ((next = children.takeCompletion())) await next();
    notify.mockClear();
  };
  return { call, session, controller, settings, children, complete, finish, invoke, registerTool };
}

test('returns an ID before completion, isolates the child, and delivers its bounded result once', async () => {
  const { call, session, children, complete, finish } = await setup();
  const pending = Promise.withResolvers<{ outcome: string; summary: string }>();
  session.runOutcome.mockReturnValueOnce(pending.promise);
  const id = await call();
  expect(children.inspect(id)[0]?.status).toBe('running');
  expect(complete).not.toHaveBeenCalled();
  await vi.waitFor(() => expect(session.runOutcome).toHaveBeenCalled());
  expect(mock.create.mock.calls[0]![0]).toMatchObject({
    model: 'test/model',
    thinkingLevel: 'low',
    output: 'text',
    logContent: false,
    tools: 'extensions',
    resources: {
      extensions: false,
      skills: [expect.stringContaining('.agents/skills')],
      contextFiles: false
    }
  });
  expect(mock.create.mock.calls[0]![0].sessionFile).toBeUndefined();
  expect(mock.create.mock.calls[0]![0].systemPrompt).not.toContain('patient gardening expert');
  expect(JSON.parse(session.runOutcome.mock.calls[0]![1])).toEqual({
    task: 'Compare these approaches',
    context: 'Explicit private context'
  });
  pending.resolve({ outcome: 'completed', summary: 'x'.repeat(18000) });
  await finish();
  expect(complete).toHaveBeenCalledOnce();
  expect(complete.mock.calls[0]![0].result).toHaveLength(16000 + '\n[Result truncated.]'.length);
  expect(children.takeCompletion()).toBeUndefined();
  expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce();
});

test('selects named profiles without the root personality and retains generic delegation', async () => {
  const { call, finish, registerTool } = await setup(false, {
    researcher: {
      description: 'Compare sources.',
      model: 'test/research',
      reasoningLevel: 'high',
      systemPrompt: 'Report supported findings.'
    },
    reviewer: {
      description: 'Find flaws.',
      model: 'test/review',
      reasoningLevel: 'off',
      systemPrompt: 'Review evidence.'
    }
  });
  expect(registerTool.mock.calls[0]![0].description).toContain('researcher: Compare sources.');
  expect(registerTool.mock.calls[0]![0].description).not.toContain('Report supported findings.');
  await call('researcher');
  await finish();
  expect(mock.create.mock.calls[0]![0]).toMatchObject({
    model: 'test/research',
    thinkingLevel: 'high',
    label: 'Subagent: researcher'
  });
  expect(mock.create.mock.calls[0]![0].systemPrompt).toContain('Report supported findings.');
  expect(mock.create.mock.calls[0]![0].systemPrompt).not.toContain('Review evidence.');
  await call('reviewer');
  await finish();
  expect(mock.create.mock.calls[1]![0]).toMatchObject({
    model: 'test/review',
    thinkingLevel: 'off'
  });
  await call();
  await finish();
  expect(mock.create.mock.calls[2]![0]).toMatchObject({
    model: 'test/model',
    thinkingLevel: 'low'
  });
});

test.each(['missing', 'constructor', '__proto__'])(
  'rejects unknown profile %s before session creation',
  async (name) => {
    const { call } = await setup();
    await expect(call(name)).rejects.toThrow('Unknown subagent profile');
    expect(mock.create).not.toHaveBeenCalled();
  }
);

test('enables only web MCP and no memory or Chatto tools', async () => {
  const { call, finish } = await setup(true);
  await call();
  await finish();
  expect(mock.mcp).toHaveBeenCalledWith(expect.objectContaining({ mcp: false, web: true }));
  expect(mock.mcp.mock.calls[0]).toHaveLength(1);
  expect(mock.create.mock.calls[0]![0].extensions).toHaveLength(2);
});

test('buffers bounded progress, excludes sensitive event fields, and steers only owned children', async () => {
  const { call, session, children, invoke, finish } = await setup();
  const pending = Promise.withResolvers<{ outcome: string; summary: string }>();
  session.runOutcome.mockImplementationOnce(async (_ctx, _prompt, { onText }) => {
    for (let i = 0; i < 20; i++) onText(`Update ${i}`);
    return pending.promise;
  });
  const id = await call();
  await vi.waitFor(() => expect(children.inspect(id)[0]?.progress).toHaveLength(12));
  const onEvent = mock.create.mock.calls[0]![0].onEvent;
  onEvent({ type: 'tool_execution_start', toolName: 'search', args: 'private arguments' });
  onEvent({ type: 'thinking_delta', delta: 'private reasoning' });
  await vi.waitFor(() => expect(children.inspect(id)[0]?.progress.at(-1)).toContain('search'));
  expect(JSON.stringify(children.inspect())).not.toContain('private');
  await invoke('steer_subagent', { id, message: 'Focus on costs.' });
  await vi.waitFor(() => expect(session.steer).toHaveBeenCalledWith('Focus on costs.'));
  await expect(invoke('steer_subagent', { id: 'foreign', message: 'secret' })).rejects.toThrow(
    'Unknown subagent'
  );
  pending.resolve({ outcome: 'completed', summary: 'Done' });
  await finish();
  await expect(children.steer(id, 'Late')).rejects.toThrow('finished');
});

test('parent turn cancellation does not cancel children, but root cleanup cancels and joins them', async () => {
  const { call, session, controller, children, complete } = await setup();
  session.runOutcome.mockImplementationOnce(
    async (_ctx, _prompt, { signal }) =>
      new Promise((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(signal.reason), { once: true });
      })
  );
  const id = await call();
  await vi.waitFor(() => expect(session.runOutcome).toHaveBeenCalled());
  controller.abort();
  expect(children.inspect(id)[0]?.status).toBe('running');
  await children.close();
  expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce();
  expect(complete).not.toHaveBeenCalled();
  await expect(call()).rejects.toThrow();
});

test('deadline cancellation and provider failures produce safe completion results', async () => {
  const { call, session, settings, complete, finish } = await setup();
  settings.turnTimeoutMs = 10;
  session.runOutcome.mockImplementationOnce(
    async (_ctx, _prompt, { signal }) =>
      new Promise((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(signal.reason), { once: true });
      })
  );
  await call();
  await finish();
  expect(complete.mock.calls[0]![0].status).toBe('cancelled');
  session.runOutcome.mockRejectedValueOnce(new Error('private provider details'));
  await call();
  await finish();
  expect(complete.mock.calls[1]![0]).toMatchObject({
    status: 'failed',
    result: 'Subagent did not complete. Do not infer findings.'
  });
  expect(JSON.stringify(complete.mock.calls)).not.toContain('private');
});

test('bounds pending completions as well as running work and retains only recent records', async () => {
  const { call, children, finish } = await setup();
  const first = await call();
  await call();
  await call();
  await call();
  await vi.waitFor(() =>
    expect(children.inspect().every((child) => child.status === 'completed')).toBe(true)
  );
  await expect(call()).rejects.toThrow('Four subagent results are pending');
  await finish();
  for (let i = 0; i < 16; i++) {
    await call();
    await finish();
  }
  expect(children.inspect()).toHaveLength(16);
  expect(() => children.inspect(first)).toThrow('Unknown subagent');
});
