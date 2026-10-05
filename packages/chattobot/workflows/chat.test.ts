import { expect, test, vi } from 'vitest';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { AgentRunOptions } from 'runling/agents';
import type { WebhookContext } from 'runling/web';
import type { Delivery } from '../chatto/routing.ts';
import config from '../runling.config.ts';
import { scriptedChattoBot as createChattoBot } from '../evaluations/scripted-supervisor.ts';
import type { AgentExtensionAPI } from 'runling/agents';

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

test.each([null, 'existing-thread'])(
  'acknowledges a request in %s before continuing with the answer',
  async (threadRootId) => {
    const post = vi.fn(async () => {});
    let acknowledge:
      | ((id: string, input: { acknowledgement: string }, signal: AbortSignal) => Promise<unknown>)
      | undefined;
    const bot = createChattoBot({
      acknowledge: async () => {},
      post,
      typing: async () => {},
      timeout: 0,
      readThread: async () => ({ messages: [], olderOmitted: false }),
      createAgent: async (options) => {
        for (const extension of options.extensions ?? []) {
          const factory = typeof extension === 'function' ? extension : extension.factory;
          await factory({
            on() {},
            registerTool(definition: { name: string; execute: NonNullable<typeof acknowledge> }) {
              if (definition.name === 'acknowledgeRequest') acknowledge = definition.execute;
            }
          } as unknown as AgentExtensionAPI);
        }
        return {
          async runOutcome(_ctx, _prompt, runOptions) {
            const signal = new AbortController().signal;
            await acknowledge!('ack', { acknowledgement: 'I’ll check that.' }, signal);
            expect(post).toHaveBeenCalledOnce();
            await expect(
              acknowledge!('again', { acknowledgement: 'Checking.' }, signal)
            ).rejects.toThrow('already acknowledged');
            runOptions?.onText?.('Here is the answer.');
            return { outcome: 'completed' as const, summary: '', usage: emptyTokenUsage() };
          },
          steer: async () => false,
          dispose() {}
        };
      }
    });
    await bot(createWorkflowContext(), { ...delivery, thread_root_id: threadRootId });
    expect(post).toHaveBeenCalledTimes(2);
    expect(post).toHaveBeenNthCalledWith(
      2,
      expect.any(Object),
      'Here is the answer.',
      expect.any(AbortSignal)
    );
    expect(post).toHaveBeenNthCalledWith(
      1,
      { roomId: 'dm', threadRootId: threadRootId ?? 'root', inReplyTo: 'root' },
      'I’ll check that.',
      expect.any(AbortSignal)
    );
  }
);

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
      // The message to answer is apart from the thread, which is background.
      expect(JSON.parse(prompt)).toEqual({
        // Another person's message is not in the prompt; readThread returns it on request.
        unreadThreadMessages: 1,
        message: { from: 'someone', fromMaintainer: false, text: 'Hello!' },
        recentMessagesToYou: ['Hello!'],
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
    readThread: async () => ({
      messages: [{ id: 'earlier', role: 'human' as const, body: 'eins, zwei, drei' }],
      olderOmitted: false
    })
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
        ? [
            'readThread',
            'acknowledgeRequest',
            'finishTurn',
            'fetchPage',
            'investigateChatto',
            'task_send',
            'task_cancel'
          ]
        : ['readThread', 'acknowledgeRequest', 'finishTurn', 'fetchPage'],
      // The prompt hygiene extension comes first, then the gates and tools.
      extensions: Array(thread ? 7 : 5).fill(expect.any(Function)),
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
    readThread: async () => ({ messages: [], olderOmitted: false }),
    implementation: { directory: '/configured/chatto', repository: 'example/chatto' },
    createAgent: async (options) => {
      expect(options.tools).toEqual([
        'readThread',
        'acknowledgeRequest',
        'finishTurn',
        'fetchPage',
        'implementChatto',
        'askImplementation',
        'decideApproval',
        'task_send',
        'task_cancel'
      ]);
      expect(options.systemPrompt).toContain('You are ChattoBot');
      expect(options.instructions?.join('\n')).toContain('implementChatto starts a worker');
      expect(options.instructions?.join('\n')).not.toContain('Implementation is not available.');
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
    readThread: async () => ({ messages: [], olderOmitted: false }),
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
    readThread: async () => ({
      messages: [{ id: 'prior', role: 'bot' as const, body: '我正在调查' }],
      olderOmitted: false
    }),
    createAgent: async (options) => {
      expect(options.instructions?.join('\n')).toContain('Nothing else chooses the language.');
      return {
        dispose: () => {},
        steer: async () => false,
        async runOutcome(_ctx, prompt) {
          // The bot's own earlier reply is marked as its own and does not set the language.
          expect(JSON.parse(prompt)).toMatchObject({
            message: { text: 'Hello!' },
            recentMessagesToYou: ['Hello!'],
            earlierThreadMessages: [{ from: 'you', text: '我正在调查' }]
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
    readThread: async () => ({ messages: [], olderOmitted: false }),
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
    readThread: async () => ({ messages: [], olderOmitted: false }),
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

test('rapid follow-ups wait for initial context and the active answer before starting ordered turns', async () => {
  let releaseHistory!: () => void;
  const history = new Promise<void>((resolve) => {
    releaseHistory = resolve;
  });
  let finish!: () => void;
  const finished = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const messages: string[] = [];
  let firstRead = true;
  const readThread = vi.fn(async () => {
    if (firstRead) {
      firstRead = false;
      await history;
    }
    return { messages: [], olderOmitted: false };
  });
  const runOutcome = vi.fn(async (_ctx, prompt: string) => {
    messages.push(JSON.parse(prompt).message.text);
    await finished;
    return { outcome: 'completed' as const, summary: 'Done', usage: emptyTokenUsage() };
  });
  const steer = vi.fn(async () => true);
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
    await vi.waitFor(() => expect(runOutcome).toHaveBeenCalledOnce());
    expect(messages).toEqual(['Hello!']);
    expect(readThread).toHaveBeenCalledOnce();
    expect(steer).not.toHaveBeenCalled();
    finish();
    await vi.waitFor(() => expect(runOutcome).toHaveBeenCalledTimes(3));
    expect(messages).toEqual(['Hello!', 'Use a short answer', 'And include links']);
    expect(readThread).toHaveBeenCalledTimes(3);
  } finally {
    releaseHistory();
    finish();
    await running;
  }
  expect(runOutcome).toHaveBeenCalledTimes(3);
});

test('a later turn gets only new thread messages, read after the saved cursor', async () => {
  const prompts: string[] = [];
  const acknowledged: string[] = [];
  const readThread = vi
    .fn()
    .mockResolvedValueOnce({
      messages: [{ id: 'root', role: 'human', body: 'Hey' }],
      cursor: 'c1',
      olderOmitted: true
    })
    .mockResolvedValue({
      messages: [
        { id: 'reply', role: 'bot', body: 'Reply' },
        { id: 'count', role: 'human', authorName: 'Bob', body: 'eins, zwei, drei' },
        { id: 'follow-up', role: 'human', authorName: 'Alice', body: "What's next?" }
      ],
      cursor: 'c2',
      olderOmitted: false
    });
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
        expect(acknowledged).toEqual(['first']);
        prompts.push(prompt);
        options?.onText?.('Reply');
        return { outcome: 'completed', summary: 'Reply', usage: emptyTokenUsage() };
      },
      steer: async () => false,
      dispose: () => {}
    })
  });

  await bot(
    { ...createWorkflowContext(), run: { id: 'run', reference: 'funky-comics-8426' } },
    {
      ...delivery,
      triggers: ['mention'],
      thread_root_id: 'root',
      message: { ...delivery.message, id: 'first' }
    }
  );
  expect(prompts).toHaveLength(2);
  expect(acknowledged).toEqual(['first']);
  // The prompt does not name the run.
  expect(JSON.parse(prompts[0]!)).not.toHaveProperty('runName');
  expect(JSON.parse(prompts[0]!)).toMatchObject({
    earlierThreadMessages: [{ from: 'someone', text: 'Hey' }],
    olderThreadMessagesOmitted: true
  });
  // The second turn reads after the cursor and gets only what the supervisor has not seen:
  // not its own reply, and not the message that woke it.
  expect(readThread.mock.calls[1]![2]).toBe('c1');
  expect(JSON.parse(prompts[1]!)).toEqual({
    // Bob wrote to someone else: counted, and available through readThread.
    unreadThreadMessages: 1,
    message: { from: 'Alice', fromMaintainer: false, text: "What's next?" },
    savedImplementationPlans: [],
    recentMessagesToYou: ['Hello!', "What's next?"],
    backgroundTasks: []
  });
});

test('a queued message read in an earlier turn keeps its author', async () => {
  const prompts: string[] = [];
  const readThread = vi
    .fn()
    .mockResolvedValueOnce({
      messages: [
        { id: 'root', role: 'human', authorName: 'Carol', body: 'Hey' },
        { id: 'queued', role: 'human', authorName: 'Dana', body: 'Und jetzt?' }
      ],
      cursor: 'c1',
      olderOmitted: false
    })
    .mockResolvedValue({ messages: [], cursor: 'c1', olderOmitted: false });
  const post = vi.fn(async () => {
    if (post.mock.calls.length === 1) {
      await bot.route(
        { start: async () => Promise.reject(new Error('Unexpected run')) },
        {
          ...delivery,
          id: 'queued',
          triggers: ['mention'],
          thread_root_id: 'root',
          message: { ...delivery.message, id: 'queued', body: 'Und jetzt?' }
        }
      );
    }
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    readThread,
    post,
    typing: async () => {},
    timeout: 0.05,
    createAgent: async () => ({
      async runOutcome(_ctx, prompt, options) {
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
  const second = JSON.parse(prompts[1]!);
  expect(second).toMatchObject({ message: { text: 'Und jetzt?', from: 'Dana' } });
  expect(second).not.toHaveProperty('newThreadMessages');
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
    readThread: async () => ({ messages: [], olderOmitted: false }),
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
    blockAfterUntrusted: ['implementChatto', 'askImplementation', 'task_send', 'decideApproval']
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
    readThread: async () => ({ messages: [], olderOmitted: false }),
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
    readThread: async () => ({ messages: [], olderOmitted: false }),
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
            if (name === 'tool_call') gate ??= handler; // The maintainer gate comes first.
          },
          registerTool() {}
        } as unknown as import('runling/agents').AgentExtensionAPI);
      }
      return {
        async runOutcome(_ctx: unknown, prompt: string) {
          decisions.push({
            flag: JSON.parse(prompt).message.fromMaintainer,
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
      open: { block: true, reason: expect.stringContaining('acknowledgeRequest') }
    },
    {
      flag: true,
      gate: undefined,
      open: { block: true, reason: expect.stringContaining('acknowledgeRequest') }
    }
  ]);
  expect(post.mock.calls.filter(([, text]) => text.startsWith('Only a maintainer'))).toHaveLength(
    1
  );
});

test.each([false, true])(
  'general web research requires a maintainer and a posted acknowledgement (%s)',
  async (maintainer) => {
    let gate:
      | ((event: {
          toolName: string;
          input: unknown;
          parentToolCallId?: string;
        }) => Promise<unknown>)
      | undefined;
    let ack:
      | ((id: string, input: { acknowledgement: string }, signal: AbortSignal) => Promise<unknown>)
      | undefined;
    const bot = createChattoBot({
      acknowledge: async () => {},
      post: async () => {},
      typing: async () => {},
      timeout: 0,
      maintainers: maintainer ? ['alice'] : [],
      web: { tavilyApiKey: 'unused' },
      readThread: async () => ({ messages: [], olderOmitted: false }),
      createAgent: async (options) => {
        for (const extension of options.extensions ?? []) {
          await (typeof extension === 'function' ? extension : extension.factory)({
            on(name: string, handler: NonNullable<typeof gate>) {
              if (name === 'tool_call') gate ??= handler;
            },
            registerTool(tool: { name: string; execute: NonNullable<typeof ack> }) {
              if (tool.name === 'acknowledgeRequest') ack = tool.execute;
            }
          } as unknown as AgentExtensionAPI);
        }
        expect(options.trust?.blockAfterUntrusted).not.toContain('finishTurn');
        return {
          async runOutcome() {
            const call = { toolName: 'researchWeb', input: { question: 'HN today' } };
            expect(await gate!(call)).toMatchObject({ block: true });
            await ack!('ack', { acknowledgement: 'I’ll check.' }, new AbortController().signal);
            expect(await gate!({ toolName: 'fetchPage', input: {} })).toBeUndefined();
            if (maintainer) expect(await gate!(call)).toBeUndefined();
            else
              expect(await gate!(call)).toMatchObject({
                block: true,
                reason: expect.stringContaining('maintainer')
              });
            expect(await gate!({ ...call, parentToolCallId: 'script' })).toMatchObject({
              block: true
            });
            return { outcome: 'completed' as const, summary: '', usage: emptyTokenUsage() };
          },
          steer: async () => false,
          dispose() {}
        };
      }
    });
    await bot(createWorkflowContext(), delivery);
  }
);
