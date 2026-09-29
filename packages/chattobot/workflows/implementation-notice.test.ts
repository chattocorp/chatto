import { expect, test, vi } from 'vitest';
import { createWorkflowContext } from 'runling';
import type { AgentOptions } from 'runling/agents';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
import { createChattoBot } from './chat.ts';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { deliveryConversationKey } from '../chatto/routing.ts';

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
    readThread: async () => ({ messages: [], olderOmitted: false }),
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

test('a notification turn cannot cancel a task; a person can', async () => {
  let gate: ((event: unknown) => Promise<unknown>) | undefined;
  const decisions: unknown[] = [];
  interact.mockImplementationOnce(async (_ctx, _agent, _prompt, options) => {
    await options.prepareMessage(published, 'notification');
    decisions.push(await gate!({ type: 'tool_call', toolName: 'task_cancel', input: {} }));
    await options.prepareMessage('Please stop the implementation', 'user');
    decisions.push(await gate!({ type: 'tool_call', toolName: 'task_cancel', input: {} }));
    return 'done';
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    post: async () => {},
    typing: async () => {},
    readThread: async () => ({ messages: [], olderOmitted: false }),
    timeout: 0,
    createAgent: async (options: AgentOptions) => {
      for (const extension of options.extensions ?? []) {
        const factory = typeof extension === 'function' ? extension : extension.factory;
        await factory({
          on: (name: string, handler: (event: unknown) => Promise<unknown>) => {
            if (name === 'tool_call') gate = handler;
          },
          registerTool() {}
        } as unknown as import('runling/agents').AgentExtensionAPI);
      }
      return { runOutcome: vi.fn(), steer: async () => false, dispose() {} };
    },
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
  expect(decisions).toEqual([
    { block: true, reason: expect.stringContaining('A notification is not such a request') },
    undefined
  ]);
});

test('the supervisor prompt lists the unfinished implementations of this thread', async () => {
  const artifacts = await mkdtemp(join(tmpdir(), 'chattobot-resumable-'));
  const delivery = {
    version: 1 as const,
    id: 'delivery',
    type: 'message.created' as const,
    triggers: ['direct_message'],
    occurred_at: 'now',
    bot_id: 'bot',
    room_id: 'room',
    thread_root_id: null,
    message: { id: 'message', author_id: 'human', body: 'Continue the shrug work' }
  };
  const ownerKey = createHash('sha256').update(deliveryConversationKey(delivery)).digest('hex');
  await mkdir(join(artifacts, 'implementation-abc123'));
  await writeFile(
    join(artifacts, 'implementation-abc123', 'metadata.json'),
    JSON.stringify({
      branch: `chattobot/${randomUUID()}`,
      baseBranch: 'main',
      baseCommit: 'a'.repeat(40),
      repository: 'example/chatto',
      stage: 'interrupted',
      ownerKey,
      input: { request: 'Add a /shrug command' }
    })
  );
  let prompt: Record<string, unknown> | undefined;
  interact.mockImplementationOnce(async (_ctx, _agent, _prompt, options) => {
    prompt = JSON.parse(await options.prepareMessage('Continue the shrug work', 'user'));
    return 'done';
  });
  try {
    await createChattoBot({
      acknowledge: async () => {},
      post: async () => {},
      typing: async () => {},
      readThread: async () => ({ messages: [], olderOmitted: false }),
      timeout: 0,
      createAgent: async () => ({ runOutcome: vi.fn(), steer: async () => false, dispose() {} }),
      implementation: {
        directory: '/unused',
        repository: 'example/chatto',
        artifactsDirectory: artifacts
      }
    })(createWorkflowContext(), delivery);
    expect(prompt?.resumableImplementations).toEqual([
      {
        artifactId: 'implementation-abc123',
        request: 'Add a /shrug command',
        stage: 'interrupted',
        updatedAt: expect.any(Number),
        sessionSaved: false
      }
    ]);
  } finally {
    await rm(artifacts, { recursive: true, force: true });
  }
});
