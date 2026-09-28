import { expect, test, vi } from 'vitest';
import { createWorkflowContext } from 'runling';
import type { AgentOptions } from 'runling/agents';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
import { createChattoBot } from './chat.ts';

const prUrl = 'https://github.com/example/chatto/pull/7';
const published = JSON.stringify({
  type: 'task.notice',
  text: 'The pull request is open.',
  data: { milestone: 'published', prUrl },
  task: { id: 'implementation' }
});

/** Run one supervisor turn for a `published` notice; `reply` is the model's message, if any. */
async function publishedTurn(reply?: string) {
  const post = vi.fn(async () => {});
  interact.mockImplementationOnce(async (ctx, _agent, _prompt, options) => {
    await options.prepareMessage(published, 'notification');
    options.onBusy(true);
    if (reply) await ctx.emit(reply);
    options.onBusy(false);
    await vi.waitFor(() => expect(post).toHaveBeenCalled());
    return 'done';
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    post,
    typing: async () => {},
    readThread: async () => [],
    timeout: 0,
    createAgent: async (_options: AgentOptions) => ({
      runOutcome: vi.fn(),
      steer: async () => false,
      dispose() {}
    }),
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
  return post.mock.calls.map((call) => (call as unknown[])[1]);
}

test('the supervisor phrases a milestone; the host adds the PR URL only when it is missing', async () => {
  expect(await publishedTurn(`Der PR ist offen: ${prUrl}`)).toEqual([`Der PR ist offen: ${prUrl}`]);
  expect(await publishedTurn('Der PR ist offen.')).toEqual([`Der PR ist offen.\n\n${prUrl}`]);
});

test('when the supervisor says nothing about a new PR, the host posts its URL alone', async () => {
  expect(await publishedTurn()).toEqual([prUrl]);
});
