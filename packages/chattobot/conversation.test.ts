import { expect, test, vi } from 'vitest';
import {
  createChannel,
  emptyTokenUsage,
  runWorkflow,
  type AgentExtensionAPI,
  type AgentOptions,
  type AgentResult,
  type RunlingAgent
} from 'runling';
import type { Destination } from '@chatto/client';
import type { IncomingMessage } from './chatto/message.ts';
import { createConversation, type ChattoAccess } from './conversation.ts';
import type { ThreadMessage } from './thread.ts';
import type { Investigation } from './investigation/tool.ts';

const message = (id: string, body = id, authorId = 'alice'): IncomingMessage => ({
  id,
  roomId: 'room',
  threadRootId: 'root',
  authorId,
  body
});

const threadMessage = (
  id: string,
  body: string,
  authorName = 'Alice',
  role: 'human' | 'bot' = 'human'
): ThreadMessage => ({ id, body, authorName, role });

type Tool = {
  name: string;
  execute(id: string, input: unknown, signal?: AbortSignal): Promise<any>;
};

/** A conversation with a fake agent and a fake Chatto. The agent answers each prompt with
 * `answer(prompt)`, after `turn` resolves when given. */
function harness({
  answer = (prompt: string) => `Reply ${prompt.length}`,
  thread = [] as ThreadMessage[][],
  maintainers = [] as string[],
  web,
  investigation,
  turn
}: {
  web?: { tavilyApiKey: string };
  investigation?: Investigation;
  answer?: (prompt: string) => string | AgentResult;
  thread?: ThreadMessage[][];
  maintainers?: string[];
  turn?: () => Promise<void>;
} = {}) {
  const prompts: string[] = [];
  const posts: { destination: Destination; body: string }[] = [];
  const reactions: string[] = [];
  const reads: (string | undefined)[] = [];
  const tools = new Map<string, Tool>();
  let options!: AgentOptions;
  let disposed = false;
  const chatto: ChattoAccess = {
    post: async (destination, body) => void posts.push({ destination, body }),
    typing: async () => {},
    acknowledge: async (incoming) => void reactions.push(incoming.id),
    readThread: async (_thread, _signal, after) => {
      reads.push(after);
      return { messages: thread.shift() ?? [], cursor: `c${reads.length}`, olderOmitted: false };
    },
    readAttachment: async () => ({
      filename: 'a',
      contentType: 'text/plain',
      data: new Uint8Array()
    })
  };
  const createAgent = vi.fn(async (agentOptions: AgentOptions) => {
    options = agentOptions;
    for (const extension of agentOptions.extensions ?? []) {
      const factory = typeof extension === 'function' ? extension : extension.factory;
      await factory({
        registerTool: (tool: Tool) => void tools.set(tool.name, tool),
        on: () => {}
      } as unknown as AgentExtensionAPI);
    }
    return {
      id: 'agent',
      async runOutcome(_ctx, prompt) {
        prompts.push(prompt);
        await turn?.();
        const result = answer(prompt);
        return typeof result === 'string'
          ? { outcome: 'completed', summary: result, usage: emptyTokenUsage() }
          : result;
      },
      run: () => Promise.reject(new Error('unused')),
      steer: async () => false,
      fork: () => Promise.reject(new Error('unused')),
      dispose: () => void (disposed = true),
      async [Symbol.asyncDispose]() {
        disposed = true;
      }
    } as RunlingAgent;
  });
  const conversation = createConversation(
    {
      model: 'test/model',
      maintainers: new Set(maintainers),
      ...(web ? { web } : {}),
      ...(investigation ? { investigation } : {})
    },
    chatto,
    createAgent
  );
  const messages = createChannel<unknown>();
  const start = (first: IncomingMessage) => runWorkflow(conversation, { input: first, messages });
  return {
    start,
    messages,
    prompts,
    posts,
    reactions,
    reads,
    tools,
    options: () => options,
    disposed: () => disposed,
    createAgent
  };
}

test('answers each message in its thread with the final text and ends when the input closes', async () => {
  const bot = harness({ answer: (prompt) => `Answer to: ${prompt.split('\n').at(-1)}` });
  const run = bot.start(message('one', 'What is Chatto?'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  await bot.messages.send(message('two', 'And Runling?'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(2));
  bot.messages.close();
  const execution = await run;

  expect(execution).toMatchObject({ ok: true, output: null });
  expect(bot.posts).toEqual([
    {
      destination: { roomId: 'room', threadRootId: 'root', inReplyTo: 'one' },
      body: 'Answer to: What is Chatto?'
    },
    {
      destination: { roomId: 'room', threadRootId: 'root', inReplyTo: 'two' },
      body: 'Answer to: And Runling?'
    }
  ]);
  expect(bot.reactions).toEqual(['one', 'two']);
  expect(bot.createAgent).toHaveBeenCalledOnce();
  expect(bot.disposed()).toBe(true);
});

test('answers messages that arrive during a turn together in the next turn', async () => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let turns = 0;
  const bot = harness({ turn: async () => void (turns++ === 0 && (await gate)) });
  const run = bot.start(message('one'));
  await vi.waitFor(() => expect(bot.prompts).toHaveLength(1));
  await bot.messages.send(message('two', 'second'));
  await bot.messages.send(message('three', 'third'));
  // Let the host forward both messages before the first turn ends.
  await new Promise((resolve) => setTimeout(resolve, 20));
  release();
  await vi.waitFor(() => expect(bot.posts).toHaveLength(2));
  bot.messages.close();
  await run;

  expect(bot.prompts[1]).toContain('second');
  expect(bot.prompts[1]).toContain('third');
  expect(bot.posts[1]!.destination.inReplyTo).toBe('three');
});

test('shows other thread messages as context and reads only newer ones later', async () => {
  const bot = harness({
    thread: [
      [
        threadMessage('root', 'We deploy with Docker.', 'Bob'),
        threadMessage('one', 'How do I upgrade?'),
        threadMessage('old', 'Earlier answer', 'ChattoBot', 'bot')
      ],
      [threadMessage('aside', 'I use Kubernetes.', 'Carol'), threadMessage('two', 'Thanks')]
    ]
  });
  const run = bot.start(message('one', 'How do I upgrade?'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  await bot.messages.send(message('two', 'Thanks'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(2));
  bot.messages.close();
  await run;

  expect(bot.reads).toEqual([undefined, 'c1']);
  expect(bot.prompts[0]).toContain('Earlier messages in this thread');
  expect(bot.prompts[0]).toContain('Bob: We deploy with Docker.');
  expect(bot.prompts[0]).not.toContain('Earlier answer');
  expect(bot.prompts[0]).toContain('Message to you from Alice:\nHow do I upgrade?');
  expect(bot.prompts[1]).toContain('Other new messages in this thread');
  expect(bot.prompts[1]).toContain('Carol: I use Kubernetes.');
  expect(bot.prompts[1]).not.toContain('Bob');
});

test('marks messages from maintainers', async () => {
  const bot = harness({ maintainers: ['maintainer'] });
  const run = bot.start(message('one', 'Hi', 'maintainer'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  bot.messages.close();
  await run;
  expect(bot.prompts[0]).toContain('from user maintainer, a maintainer:');
});

test('posts a short notice when a turn fails and keeps the conversation', async () => {
  let turns = 0;
  const bot = harness({
    answer: () =>
      turns++ === 0
        ? { outcome: 'failed', summary: 'Provider error', usage: emptyTokenUsage() }
        : 'Better now'
  });
  const run = bot.start(message('one'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  await bot.messages.send(message('two'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(2));
  bot.messages.close();
  await run;
  expect(bot.posts.map((post) => post.body)).toEqual([
    'Sorry, I could not answer that. Please try again.',
    'Better now'
  ]);
});

test('gives the agent only Chatto tools, in a private empty directory', async () => {
  const bot = harness();
  const run = bot.start(message('one'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  bot.messages.close();
  await run;
  expect(bot.options()).toMatchObject({
    output: 'text',
    tools: ['fetchPage', 'readThread', 'viewAttachment'],
    resources: { extensions: false, contextFiles: false }
  });
  expect([...bot.tools.keys()].sort()).toEqual(['fetchPage', 'readThread', 'viewAttachment']);
});

test('readThread lists the thread with the bot as "You"', async () => {
  const bot = harness({
    thread: [
      [threadMessage('one', 'Hi')],
      [threadMessage('one', 'Hi'), threadMessage('two', 'Hello!', 'ChattoBot', 'bot')]
    ],
    answer: () => 'ok'
  });
  const run = bot.start(message('one', 'Hi'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  const result = await bot.tools.get('readThread')!.execute('1', {});
  bot.messages.close();
  await run;
  expect(result.content[0].text).toBe('- Alice: Hi\n- You: Hello!');
});

test('cancellation stops the conversation without a notice', async () => {
  const controller = new AbortController();
  const bot = harness({
    turn: () =>
      new Promise((_resolve, reject) =>
        controller.signal.addEventListener('abort', () => reject(controller.signal.reason))
      )
  });
  const conversation = createConversation(
    { model: 'test/model', maintainers: new Set() },
    {
      post: async (destination, body) => void bot.posts.push({ destination, body }),
      typing: async () => {},
      acknowledge: async () => {},
      readThread: async () => ({ messages: [], olderOmitted: false }),
      readAttachment: async () => {
        throw new Error('unused');
      }
    },
    bot.createAgent as (options: AgentOptions) => Promise<RunlingAgent>
  );
  const run = runWorkflow(conversation, { input: message('one'), signal: controller.signal });
  await vi.waitFor(() => expect(bot.prompts).toHaveLength(1));
  controller.abort(new Error('Cancelled'));
  expect(await run).toMatchObject({ ok: false, error: 'Cancelled' });
  expect(bot.posts).toEqual([]);
});

test('the web tools refuse turns that are not from maintainers', async () => {
  const fetch = vi.spyOn(globalThis, 'fetch');
  const bot = harness({ web: { tavilyApiKey: 'key' }, maintainers: ['maintainer'] });
  const run = bot.start(message('one', 'Search the web', 'alice'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  await expect(bot.tools.get('webSearch')!.execute('1', { query: 'chatto' })).rejects.toThrow(
    'Only maintainers can ask you to search or read the web'
  );
  expect(fetch).not.toHaveBeenCalled();
  expect(bot.options().systemPrompt).toContain('Only maintainers can ask you to search');
  bot.messages.close();
  await run;
  fetch.mockRestore();
});

test('offers investigate with a scope for the thread when investigation is set up', async () => {
  const scopes: string[] = [];
  const bot = harness({
    investigation: {
      directory: '/repo',
      investigator: (scope) => {
        scopes.push(scope);
        return (async () => ({ session: 's', answer: 'a', citations: [] })) as never;
      }
    }
  });
  const run = bot.start(message('one'));
  await vi.waitFor(() => expect(bot.posts).toHaveLength(1));
  bot.messages.close();
  expect(await run).toMatchObject({ ok: true });
  expect(bot.options().tools).toContain('investigate');
  expect(scopes).toEqual([expect.stringMatching(/^[0-9a-f]{32}$/)]);
});
