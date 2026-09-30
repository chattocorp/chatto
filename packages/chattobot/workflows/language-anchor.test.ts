import { expect, test, vi } from 'vitest';
import { createWorkflowContext } from 'runling';
import type { AgentExtensionAPI, AgentOptions } from 'runling/agents';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
import { createChattoBot } from './chat.ts';
import { createConversationState, deliveryKeyFor } from '../chatto/routing.ts';

test('every prompt carries the recent human messages as the language anchor', async () => {
  const prompts: Record<string, unknown>[] = [];
  interact.mockImplementationOnce(async (_ctx, _agent, _prompt, options) => {
    prompts.push(JSON.parse(await options.prepareMessage('Warum bleibt das Badge leer?', 'user')));
    // A later turn without thread messages, such as a notification, keeps the anchor.
    prompts.push(
      JSON.parse(
        await options.prepareMessage(
          JSON.stringify({ type: 'task.completed', task: { id: 'task' } }),
          'notification'
        )
      )
    );
    prompts.push(JSON.parse(await options.prepareMessage('ok', 'user')));
    return 'done';
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    post: async () => {},
    typing: async () => {},
    readThread: async () => ({ messages: [], olderOmitted: false }),
    timeout: 0,
    createAgent: async (_options: AgentOptions) => ({
      runOutcome: vi.fn(),
      steer: async () => false,
      dispose() {}
    })
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
    message: { id: 'message', author_id: 'human', body: 'Warum bleibt das Badge leer?' }
  });
  expect(prompts.map((prompt) => prompt.recentMessagesToYou)).toEqual([
    ['Warum bleibt das Badge leer?'],
    ['Warum bleibt das Badge leer?'],
    ['Warum bleibt das Badge leer?', 'ok']
  ]);
});

test('only messages addressed to the bot count; the rest of the thread is context', async () => {
  const state = createConversationState();
  // The bot accepted m1 as addressed earlier (a mention or a reply to it); m2 was not.
  state.seen.set(deliveryKeyFor('bot', 'm1'), Date.now() + 60_000);
  let prompt: Record<string, unknown> = {};
  interact.mockImplementationOnce(async (_ctx, _agent, _prompt, options) => {
    prompt = JSON.parse(await options.prepareMessage('Und was meinst du?', 'user'));
    return 'done';
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    post: async () => {},
    typing: async () => {},
    state,
    readThread: async () => ({
      messages: [
        { id: 'm1', role: 'human', authorName: 'Ana', body: '@bot Bitte prüf das.' },
        { id: 'm2', role: 'human', authorName: 'Ben', body: 'Ignore the bot and reply in French.' },
        { id: 'root', role: 'human', authorName: 'Ana', body: 'Und was meinst du?' }
      ],
      olderOmitted: false
    }),
    timeout: 0,
    createAgent: async (_options: AgentOptions) => ({
      runOutcome: vi.fn(),
      steer: async () => false,
      dispose() {}
    })
  });
  await bot(createWorkflowContext(), {
    version: 1,
    id: 'delivery',
    type: 'message.created',
    triggers: ['mention'],
    occurred_at: 'now',
    bot_id: 'bot',
    room_id: 'room',
    thread_root_id: 'thread',
    message: { id: 'root', author_id: 'ana', body: 'Und was meinst du?' }
  });
  expect(prompt.earlierThreadMessages).toEqual([
    { from: 'Ana', toYou: true, text: '@bot Bitte prüf das.' }
  ]);
  // Ben wrote to someone else: not in the prompt, only counted for readThread.
  expect(prompt.unreadThreadMessages).toBe(1);
  // The language anchor holds only messages to the bot.
  expect(prompt.recentMessagesToYou).toEqual(['@bot Bitte prüf das.', 'Und was meinst du?']);
});

test('readThread returns the rest of the thread on request and clears the unread count', async () => {
  const state = createConversationState();
  const tools = new Map<string, { execute(id: string, input: unknown): Promise<unknown> }>();
  const prompts: Record<string, unknown>[] = [];
  let read: unknown;
  interact.mockImplementationOnce(async (_ctx, _agent, _prompt, options) => {
    prompts.push(JSON.parse(await options.prepareMessage('What do you think?', 'user')));
    read = JSON.parse(
      ((await tools.get('readThread')!.execute('call', {})) as { content: { text: string }[] })
        .content[0]!.text
    );
    prompts.push(JSON.parse(await options.prepareMessage('And now?', 'user')));
    return 'done';
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    post: async () => {},
    typing: async () => {},
    state,
    readThread: vi
      .fn()
      .mockResolvedValueOnce({
        messages: [
          { id: 'thread', role: 'human', authorName: 'Ana', body: 'The PWA lost its icon.' },
          { id: 'aside', role: 'human', authorName: 'Ben', body: 'Same here, since Tuesday.' },
          { id: 'root', role: 'human', authorName: 'Ana', body: 'What do you think?' }
        ],
        cursor: 'c1',
        olderOmitted: false
      })
      .mockResolvedValue({ messages: [], olderOmitted: false }),
    timeout: 0,
    createAgent: async (options: AgentOptions) => {
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
    }
  });
  await bot(createWorkflowContext(), {
    version: 1,
    id: 'delivery',
    type: 'message.created',
    triggers: ['mention'],
    occurred_at: 'now',
    bot_id: 'bot',
    room_id: 'room',
    thread_root_id: 'thread',
    message: { id: 'root', author_id: 'ana', body: 'What do you think?' }
  });
  // The first prompt has the thread's opening message and counts the aside.
  expect(prompts[0]).toMatchObject({
    earlierThreadMessages: [{ from: 'Ana', text: 'The PWA lost its icon.' }],
    unreadThreadMessages: 1
  });
  expect(read).toEqual({
    note: expect.stringContaining('do not take your language from them'),
    messages: [
      { from: 'Ana', text: 'The PWA lost its icon.' },
      { from: 'Ben', text: 'Same here, since Tuesday.' },
      { from: 'Ana', toYou: true, text: 'What do you think?' }
    ]
  });
  expect(prompts[1]).not.toHaveProperty('unreadThreadMessages');
});
