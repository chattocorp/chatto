import { expect, test, vi } from 'vitest';
import { createWorkflowContext, type WorkflowContext } from 'runling';
import type { AgentExtensionAPI, AgentOptions, AgentTasks, AgentTaskUpdate } from 'runling/agents';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
// A stand-in implementation tool: it announces the work and starts a child task that stops with
// a blocked result. The child reports only to its parent.
vi.mock('./implement.ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./implement.ts')>()),
  implementationExtension:
    (
      ctx: WorkflowContext<string, string>,
      _settings: unknown,
      announce: (text: string, signal: AbortSignal) => Promise<void>,
      tasks: AgentTasks
    ) =>
    (pi: AgentExtensionAPI) =>
      pi.registerTool({
        name: 'fakeImplement',
        execute: async () => {
          await announce('Starting the change.', ctx.signal);
          const run = ctx.spawn(async (_child: WorkflowContext<string, AgentTaskUpdate>) => ({
            outcome: 'blocked',
            summary: 'unfinished edits.',
            notes: [],
            branch: 'chattobot/x',
            baseCommit: 'abc',
            worktree: '/private/worktree',
            checks: [],
            workerChecks: []
          }));
          tasks.observe('Chatto implementation', run);
        }
      } as never)
}));
import { createChattoBot } from './chat.ts';

test('the supervisor task posts a stopped implementation without waking the model', async () => {
  const post = vi.fn(async () => {});
  const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
  const createAgent = async (options: AgentOptions) => {
    for (const extension of options.extensions ?? []) {
      const factory = typeof extension === 'function' ? extension : extension.factory;
      await factory({
        on() {},
        registerTool(tool: { name: string }) {
          tools.set(tool.name, tool as never);
        }
      } as unknown as AgentExtensionAPI);
    }
    return { runOutcome: vi.fn(), steer: async () => false, dispose() {} };
  };
  const woken: string[] = [];
  interact.mockImplementationOnce(async (ctx, _agent, _prompt, options) => {
    options.onBusy(true);
    await tools.get('fakeImplement')!.execute('call', {} as never);
    // The announcement already answered this turn.
    await ctx.emit('A duplicate supervisor message');
    options.onBusy(false);
    // Consume notifications like the conversation does, until the host posts the result.
    void (async () => {
      for await (const notification of options.notifications) woken.push(notification);
    })();
    await vi.waitFor(() => expect(post).toHaveBeenCalledTimes(2));
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
  expect(post.mock.calls.map((call) => (call as unknown[])[1])).toEqual([
    'Starting the change.',
    'The implementation stopped: unfinished edits. The worktree was kept for review. Please tell me how you want to proceed.'
  ]);
  expect(woken).toEqual([]);
});
