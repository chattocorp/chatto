import { expect, test, vi } from 'vitest';
import { createWorkflowContext } from 'runling';
import type { AgentExtensionAPI, AgentOptions } from 'runling/agents';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
vi.mock('./implement.ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./implement.ts')>()),
  implementationExtension:
    (
      _ctx: unknown,
      _settings: unknown,
      _announce: unknown,
      _tasks: unknown,
      callbacks: { onStopped: (message: string) => Promise<void> }
    ) =>
    (pi: AgentExtensionAPI) =>
      pi.registerTool({
        name: 'fakeImplement',
        execute: async () =>
          callbacks.onStopped(
            'The implementation stopped: unfinished edits. The worktree was kept for review. Please tell me how you want to proceed.'
          )
      } as never)
}));
import { createChattoBot } from './chat.ts';

test('a stopped implementation reaches Chatto even when the supervisor says nothing', async () => {
  const post = vi.fn(async () => {});
  const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
  const createAgent = async (options: AgentOptions) => {
    for (const extension of options.extensions ?? []) {
      const factory = typeof extension === 'function' ? extension : extension.factory;
      await factory({
        registerTool(tool) {
          tools.set(tool.name, tool as never);
        }
      } as AgentExtensionAPI);
    }
    return { runOutcome: vi.fn(), steer: async () => false, dispose() {} };
  };
  interact.mockImplementationOnce(async (ctx, _agent, _prompt, options) => {
    options.onBusy(true);
    await tools.get('fakeImplement')!.execute('call', {} as never);
    await ctx.emit('A duplicate supervisor message');
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
    implementation: { directory: '/unused', repository: 'example/chatto' }
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
    message: { id: 'message', author_id: 'human', body: 'Implement the fix' }
  });
  expect(post).toHaveBeenCalledOnce();
  expect(post).toHaveBeenCalledWith(
    { roomId: 'room', threadRootId: 'message' },
    'The implementation stopped: unfinished edits. The worktree was kept for review. Please tell me how you want to proceed.',
    expect.any(AbortSignal)
  );
});
