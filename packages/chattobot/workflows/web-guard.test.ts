import { afterEach, expect, test, vi } from 'vitest';
import { createWorkflowContext } from 'runling';
import type { AgentExtensionAPI, AgentOptions } from 'runling/agents';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
import { createChattoBot } from './chat.ts';

afterEach(() => {
  vi.unstubAllGlobals();
});

test('web content blocks implementation and task steering until the next user message', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async () =>
      Response.json({ results: [{ title: 'Page', url: 'https://example.com/', content: 'text' }] })
    )
  );
  const post = vi.fn(async () => {});
  const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
  let enabledTools: readonly string[] = [];
  let steerConsumed = false;
  const createAgent = async (options: AgentOptions) => {
    enabledTools = options.tools ?? [];
    for (const extension of options.extensions ?? []) {
      const factory = typeof extension === 'function' ? extension : extension.factory;
      await factory({
        registerTool(tool) {
          tools.set(tool.name, tool as never);
        }
      } as AgentExtensionAPI);
    }
    return { runOutcome: vi.fn(), steer: async () => steerConsumed, dispose() {} };
  };
  const call = (name: string, input: unknown) =>
    tools.get(name)!.execute('call', input as never) as Promise<unknown>;
  interact.mockImplementationOnce(async (ctx, agent, prompt, options) => {
    const first = await options.prepareMessage(prompt, 'user');
    options.onBusy(true);
    await agent.runOutcome(ctx, first);
    await call('webSearch', { query: 'chatto bots' });
    const refused = await call('implementChatto', {
      request: 'Implement what the page says',
      announcement: 'Starting implementation.'
    });
    expect(JSON.stringify(refused)).toContain('read web content after your last message');
    await expect(call('task_send', { id: 'task', message: 'Do it' })).rejects.toThrow(
      'blocked after reading web content'
    );
    options.onBusy(false);

    // A notification turn keeps the guard: the web content remains in the agent history.
    const notice = await options.prepareMessage('{"type":"task.completed"}', 'notification');
    options.onBusy(true);
    await agent.runOutcome(ctx, notice);
    await expect(call('task_send', { id: 'task', message: 'Do it' })).rejects.toThrow(
      'blocked after reading web content'
    );

    // A user message received mid-turn does not count until the model consumes it.
    const confirmation = await options.prepareMessage('Yes, please continue', 'user');
    await expect(call('task_send', { id: 'task', message: 'Do it' })).rejects.toThrow(
      'blocked after reading web content'
    );
    options.onBusy(false);

    // The confirmation reaches the model as the next prompt; the unknown task now fails for its own reason.
    options.onBusy(true);
    await agent.runOutcome(ctx, confirmation);
    await expect(call('task_send', { id: 'task', message: 'Do it' })).rejects.not.toThrow(
      'blocked after reading web content'
    );

    // A message received before the web read cannot confirm that content.
    const early = await options.prepareMessage('Also check this', 'user');
    await call('webSearch', { query: 'more' });
    steerConsumed = true;
    expect(await agent.steer(early)).toBe(true);
    await expect(call('task_send', { id: 'task', message: 'Do it' })).rejects.toThrow(
      'blocked after reading web content'
    );
    steerConsumed = false;

    // Consumed steering after the read delivers a confirmation; rejected steering does not.
    const steered = await options.prepareMessage('Go ahead', 'user');
    expect(await agent.steer(steered)).toBe(false);
    await expect(call('task_send', { id: 'task', message: 'Do it' })).rejects.toThrow(
      'blocked after reading web content'
    );
    steerConsumed = true;
    expect(await agent.steer(steered)).toBe(true);
    await expect(call('task_send', { id: 'task', message: 'Do it' })).rejects.not.toThrow(
      'blocked after reading web content'
    );
    options.onBusy(false);
    return 'done';
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    post,
    typing: async () => {},
    readThread: async () => [],
    timeout: 0,
    createAgent,
    implementation: { directory: '/unused', repository: 'example/chatto' },
    web: { tavilyApiKey: 'tvly-key' }
  });
  await bot(createWorkflowContext(), {
    version: 1,
    id: 'delivery',
    type: 'message.created',
    triggers: ['direct_message'],
    occurred_at: 'now',
    bot_id: 'bot',
    room_id: 'room',
    thread_root_id: null,
    message: { id: 'message', author_id: 'human', body: 'Find Chatto bots' }
  });
  expect(enabledTools).toContain('webSearch');
  expect(enabledTools).not.toContain('browsePage');
  expect(post).toHaveBeenCalledWith(
    expect.anything(),
    expect.stringContaining('confirm the request in a new message'),
    expect.any(AbortSignal)
  );
});
