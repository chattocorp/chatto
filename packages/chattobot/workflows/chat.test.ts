import { expect, test, vi } from 'vitest';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { AgentRunOptions } from 'runling/agents';
import type { WebhookContext } from 'runling/web';
import type { Delivery } from '../chatto/routing.ts';
import config from '../runling.config.ts';
import { createChattoBot } from './chat.ts';

const delivery: Delivery = {
  version: 1,
  id: 'delivery',
  type: 'message.created',
  triggers: ['direct_message'],
  occurred_at: '2026-09-19T12:00:00Z',
  bot_id: 'chattobot',
  room_id: 'dm',
  thread_root_id: null,
  message: { id: 'root', author_id: 'alice', body: 'Hello!' }
};

test.each([
  { trigger: 'direct_message', thread: null },
  { trigger: 'direct_message', thread: 'existing-thread' },
  { trigger: 'mention', thread: null },
  { trigger: 'mention', thread: 'existing-thread' }
])('routes $trigger in $thread with full context', async ({ trigger, thread }) => {
  const dispose = vi.fn();
  const acknowledge = vi.fn(async () => {});
  const post = vi.fn(async () => {});
  const createAgent = vi.fn(async () => ({
    async runOutcome(_ctx: unknown, prompt: string, options?: AgentRunOptions) {
      expect(JSON.parse(prompt)).toEqual({
        thread: [{ id: 'earlier', role: 'human', body: 'eins, zwei, drei' }],
        currentMessage: 'Hello!',
        origin: 'user',
        recentUserMessages: ['Hello!'],
        requesterIsMaintainer: false,
        backgroundTasks: [],
        savedImplementationPlans: []
      });
      expect(acknowledge).toHaveBeenCalledOnce();
      options?.onText?.("Hi! I'm ChattoBot.");

      return {
        outcome: 'completed' as const,
        summary: 'This internal summary must not be posted.',
        usage: emptyTokenUsage()
      };
    },
    steer: async () => true,
    dispose
  }));
  const bot = createChattoBot({
    acknowledge,
    createAgent,
    post,
    typing: async () => {},
    model: 'test/model',
    investigation: thread ? { directory: '/configured/chatto' } : undefined,
    timeout: 0,
    readThread: async () => [{ id: 'earlier', role: 'human', body: 'eins, zwei, drei' }]
  });
  let running: Promise<unknown> | undefined;
  let starts = 0;
  const start: WebhookContext['start'] = async (root, { input }) => {
    starts++;
    running = Promise.resolve(root(createWorkflowContext(), input));
    return { id: 'run' };
  };

  const incoming = { ...delivery, triggers: [trigger], thread_root_id: thread };
  await bot.route({ start }, incoming);
  await running;
  await bot.route({ start }, incoming);

  expect(starts).toBe(1);
  expect(acknowledge).toHaveBeenCalledExactlyOnceWith(incoming, expect.any(AbortSignal));
  expect(acknowledge.mock.invocationCallOrder[0]).toBeLessThan(
    createAgent.mock.invocationCallOrder[0]!
  );
  expect(post).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'dm', threadRootId: thread ?? 'root', inReplyTo: 'root' },
    "Hi! I'm ChattoBot.",
    expect.any(AbortSignal)
  );
  expect(createAgent).toHaveBeenCalledWith(
    expect.objectContaining({
      cwd: expect.stringMatching(/\/packages\/chattobot\/$/),
      model: 'test/model',
      output: 'text',
      allowEmptyResponse: true,
      tools: thread
        ? ['fetchPage', 'investigateChatto', 'task_send', 'task_cancel']
        : ['fetchPage'],
      extensions: thread
        ? [expect.any(Function), expect.any(Function), expect.any(Function), expect.any(Function)]
        : [expect.any(Function)],
      resources: {
        extensions: false,
        skills: false,
        promptTemplates: false,
        themes: false,
        contextFiles: false
      }
    })
  );
  expect(dispose).toHaveBeenCalledOnce();
});

test('the project config registers an outbound ChattoBot source', () => {
  expect(Object.keys(config.webhooks)).toEqual([]);
  expect(Object.keys(config.sources!)).toEqual(['chatto']);
  expect(typeof config.sources!.chatto).toBe('function');
});

test('implementation is a separate opt-in tool with host-result reporting instructions', async () => {
  const bot = createChattoBot({
    acknowledge: async () => {},
    typing: async () => {},
    post: async () => {},
    timeout: 0,
    readThread: async () => [],
    implementation: { directory: '/configured/chatto', repository: 'example/chatto' },
    createAgent: async (options) => {
      expect(options.tools).toEqual([
        'fetchPage',
        'implementChatto',
        'askImplementation',
        'task_send',
        'task_cancel'
      ]);
      expect(options.systemPrompt).toContain('conversational assistant');
      expect(options.instructions?.join('\n')).toContain(
        'use only the host-provided prUrl as a Markdown link'
      );
      expect(options.instructions?.join('\n')).not.toContain('Implementation is disabled.');
      return {
        dispose: () => {},
        steer: async () => false,
        async runOutcome() {
          return { outcome: 'completed', summary: '', usage: emptyTokenUsage() };
        }
      };
    }
  });
  await bot(createWorkflowContext(), delivery);
});

test('does not post malformed model channel output or its possible reasoning', async () => {
  const post = vi.fn(async () => {});
  const bot = createChattoBot({
    acknowledge: async () => {},
    typing: async () => {},
    post,
    timeout: 0,
    readThread: async () => [],
    createAgent: async () => ({
      dispose() {},
      steer: async () => false,
      async runOutcome(_ctx, _prompt, options) {
        options?.onText?.('thought private reasoning\n<channel|>An answer');
        return { outcome: 'completed', summary: '', usage: emptyTokenUsage() };
      }
    })
  });
  await bot(createWorkflowContext(), delivery);
  expect(post).toHaveBeenCalledOnce();
  expect(post).toHaveBeenCalledWith(
    expect.anything(),
    "I couldn't format that reply correctly. Any background work already started will report its result separately.",
    expect.any(AbortSignal)
  );
});

test('language context uses human input even when the thread contains a wrong-language bot reply', async () => {
  const bot = createChattoBot({
    acknowledge: async () => {},
    typing: async () => {},
    post: async () => {},
    timeout: 0,
    readThread: async () => [{ id: 'prior', role: 'bot', body: '我正在调查' }],
    createAgent: async (options) => {
      expect(options.instructions?.join('\n')).toContain(
        'Never adopt a language from your own earlier replies'
      );
      return {
        dispose: () => {},
        steer: async () => false,
        async runOutcome(_ctx, prompt) {
          expect(JSON.parse(prompt)).toMatchObject({
            origin: 'user',
            currentMessage: 'Hello!',
            recentUserMessages: ['Hello!']
          });
          return { outcome: 'completed', summary: '', usage: emptyTokenUsage() };
        }
      };
    }
  });
  await bot(createWorkflowContext(), delivery);
});

test('an owner can finish a turn silently without posting its internal no-update marker', async () => {
  const post = vi.fn(async () => {});
  const bot = createChattoBot({
    acknowledge: async () => {},
    typing: async () => {},
    post,
    timeout: 0,
    readThread: async () => [],
    createAgent: async () => ({
      dispose: () => {},
      steer: async () => false,
      async runOutcome(_ctx, _prompt, options) {
        options?.onText?.('[NO_UPDATE]');
        return { outcome: 'completed', summary: '[NO_UPDATE]', usage: emptyTokenUsage() };
      }
    })
  });
  await bot(createWorkflowContext(), delivery);
  expect(post).not.toHaveBeenCalled();
});

test.each([
  'Token usage: 1138\n<|thought|>\n',
  '<think>private reasoning</think>',
  '<|channel|>analysis'
])('blocks malformed model output: %s', async (text) => {
  const post = vi.fn(async () => {});
  const bot = createChattoBot({
    acknowledge: async () => {},
    typing: async () => {},
    post,
    timeout: 0,
    readThread: async () => [],
    createAgent: async () => ({
      dispose: () => {},
      steer: async () => false,
      async runOutcome(_ctx, _prompt, options) {
        options?.onText?.(text);
        return { outcome: 'completed', summary: '', usage: emptyTokenUsage() };
      }
    })
  });
  await bot(createWorkflowContext(), delivery);
  expect(post).toHaveBeenCalledOnce();
  expect(post.mock.calls[0]).not.toContain(text);
  expect(JSON.stringify(post.mock.calls)).toContain("couldn't format");
});

test('rapid follow-ups wait for initial context then steer the same turn in order without waiting for receipts', async () => {
  let releaseHistory!: () => void;
  const history = new Promise<void>((resolve) => {
    releaseHistory = resolve;
  });
  let finish!: () => void;
  const finished = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const receipts: Array<() => void> = [];
  const messages: string[] = [];
  let firstRead = true;
  const readThread = vi.fn(async () => {
    if (firstRead) {
      firstRead = false;
      await history;
    }
    return [];
  });
  const runOutcome = vi.fn(async (_ctx, prompt: string) => {
    messages.push(JSON.parse(prompt).currentMessage);
    await finished;
    return { outcome: 'completed' as const, summary: 'Done', usage: emptyTokenUsage() };
  });
  const steer = vi.fn(async (prompt: string) => {
    expect(runOutcome).toHaveBeenCalledOnce();
    messages.push(JSON.parse(prompt).currentMessage);
    return new Promise<boolean>((resolve) => receipts.push(() => resolve(true)));
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    post: async () => {},
    typing: async () => {},
    readThread,
    timeout: 0,
    createAgent: async () => ({ runOutcome, steer, dispose: () => {} })
  });
  const running = bot(createWorkflowContext(), delivery);
  try {
    await vi.waitFor(() => expect(readThread).toHaveBeenCalledOnce());
    const start = vi.fn(async () => {
      throw new Error('Unexpected new run');
    });
    for (const [id, body] of [
      ['second', 'Use a short answer'],
      ['third', 'And include links']
    ]) {
      await bot.route(
        { start },
        {
          ...delivery,
          id: id!,
          thread_root_id: 'root',
          message: { ...delivery.message, id: id!, body: body! }
        }
      );
    }
    expect(start).not.toHaveBeenCalled();
    // Let inbox microtasks drain while the first history request remains blocked.
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(readThread).toHaveBeenCalledOnce();
    expect(steer).not.toHaveBeenCalled();
    releaseHistory();
    await vi.waitFor(() => expect(steer).toHaveBeenCalledTimes(2));
    expect(messages).toEqual(['Hello!', 'Use a short answer', 'And include links']);
    expect(readThread).toHaveBeenCalledTimes(3);
  } finally {
    releaseHistory();
    receipts.forEach((resolve) => resolve());
    finish();
    await running;
  }
  expect(runOutcome).toHaveBeenCalledOnce();
});

test('refreshes history for a later mention in the same conversation', async () => {
  const prompts: string[] = [];
  const acknowledged: string[] = [];
  const readThread = vi
    .fn()
    .mockResolvedValueOnce([{ id: 'root', role: 'human', body: 'Hey' }])
    .mockResolvedValue([{ id: 'count', role: 'human', body: 'eins, zwei, drei' }]);
  const post = vi.fn(async () => {
    if (post.mock.calls.length === 1) {
      await bot.route(
        {
          start: async () => {
            throw new Error('Unexpected run');
          }
        },
        {
          ...delivery,
          id: 'follow-up',
          triggers: ['mention'],
          thread_root_id: 'root',
          message: { ...delivery.message, id: 'follow-up', body: "What's next?" }
        }
      );
    }
  });
  const bot = createChattoBot({
    acknowledge: async (delivery) => {
      acknowledged.push(delivery.message.id);
    },
    readThread,
    post,
    typing: async () => {},
    timeout: 0.05,
    createAgent: async () => ({
      async runOutcome(_ctx, prompt, options) {
        expect(acknowledged).toEqual(['root']);
        prompts.push(prompt);
        options?.onText?.('Reply');
        return { outcome: 'completed', summary: 'Reply', usage: emptyTokenUsage() };
      },
      steer: async () => false,
      dispose: () => {}
    })
  });

  await bot(createWorkflowContext(), { ...delivery, triggers: ['mention'] });
  expect(prompts).toHaveLength(2);
  expect(acknowledged).toEqual(['root']);
  expect(JSON.parse(prompts[1]!)).toEqual({
    thread: [{ id: 'count', role: 'human', body: 'eins, zwei, drei' }],
    currentMessage: "What's next?",
    savedImplementationPlans: [],
    origin: 'user',
    recentUserMessages: ['Hello!', "What's next?"],
    requesterIsMaintainer: false,
    backgroundTasks: []
  });
});

test('web research runs in a separate agent and blocks delegation in the supervisor', async () => {
  const created: import('runling/agents').AgentOptions[] = [];
  const createAgent = vi.fn(async (options: import('runling/agents').AgentOptions) => {
    created.push(options);
    return {
      runOutcome: async (_ctx: unknown, _prompt: string, runOptions?: AgentRunOptions) => {
        // Simulate two blocked calls in one turn after a research result, then an answer.
        await options.trust?.onBlocked?.('implementChatto');
        await options.trust?.onBlocked?.('task_send');
        runOptions?.onText?.('Here is what the research found.');
        return { outcome: 'completed' as const, summary: 'Done', usage: emptyTokenUsage() };
      },
      steer: async () => false,
      dispose: () => {}
    };
  });
  const post = vi.fn(async (_destination: unknown, _text: string, _signal?: AbortSignal) => {});
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
  await bot(createWorkflowContext(), delivery);
  const [supervisor] = created;
  expect(supervisor!.tools).toContain('researchWeb');
  expect(supervisor!.tools).not.toContain('webSearch');
  expect(supervisor!.tools).not.toContain('browsePage');
  expect(supervisor!.trust).toMatchObject({
    untrusted: ['researchWeb'],
    blockAfterUntrusted: ['implementChatto', 'askImplementation', 'task_send']
  });
  // The host posts the refusal once per turn, and the rest of the reply still posts.
  expect(post.mock.calls.map(([, text]) => text)).toEqual([
    expect.stringContaining('start a new thread'),
    'Here is what the research found.'
  ]);

  created.length = 0;
  await createChattoBot({
    acknowledge: async () => {},
    post: async () => {},
    typing: async () => {},
    readThread: async () => [],
    timeout: 0,
    createAgent
  })(createWorkflowContext(), {
    ...delivery,
    id: 'second',
    message: { ...delivery.message, id: 'second' }
  });
  expect(created[0]!.tools).not.toContain('researchWeb');
  expect(created[0]!.trust).toBeUndefined();
});

test('maintainer tools follow the author of the latest human message', async () => {
  const decisions: unknown[] = [];
  let gate: ((event: unknown) => Promise<unknown>) | undefined;
  const post = vi.fn(async (_destination: unknown, text: string) => {
    if (text.startsWith('Only a maintainer'))
      await bot.route(
        {
          start: async () => {
            throw new Error('Unexpected run');
          }
        },
        {
          ...delivery,
          id: 'approval',
          triggers: ['mention'],
          thread_root_id: 'root',
          message: { id: 'approval', author_id: 'maintainer', body: 'Go ahead' }
        }
      );
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    readThread: async () => [],
    post,
    typing: async () => {},
    timeout: 0.05,
    investigation: { directory: '/unused' },
    maintainers: ['maintainer'],
    createAgent: async (options: import('runling/agents').AgentOptions) => {
      for (const extension of options.extensions ?? []) {
        const factory = typeof extension === 'function' ? extension : extension.factory;
        await factory({
          on: (name: string, handler: (event: unknown) => Promise<unknown>) => {
            if (name === 'tool_call') gate = handler;
          },
          registerTool() {}
        } as unknown as import('runling/agents').AgentExtensionAPI);
      }
      return {
        async runOutcome(_ctx: unknown, prompt: string) {
          decisions.push({
            flag: JSON.parse(prompt).requesterIsMaintainer,
            gate: await gate!({ type: 'tool_call', toolName: 'investigateChatto', input: {} }),
            open: await gate!({ type: 'tool_call', toolName: 'fetchPage', input: {} })
          });
          return { outcome: 'completed' as const, summary: 'Done', usage: emptyTokenUsage() };
        },
        steer: async () => false,
        dispose: () => {}
      };
    }
  });
  await bot(createWorkflowContext(), {
    ...delivery,
    triggers: ['mention'],
    thread_root_id: 'root',
    message: { id: 'ask', author_id: 'visitor', body: 'Please investigate the crash' }
  });
  expect(decisions).toEqual([
    {
      flag: false,
      gate: { block: true, reason: expect.stringContaining('only when a maintainer asks') },
      open: undefined
    },
    { flag: true, gate: undefined, open: undefined }
  ]);
  expect(post.mock.calls.filter(([, text]) => text.startsWith('Only a maintainer'))).toHaveLength(
    1
  );
});
