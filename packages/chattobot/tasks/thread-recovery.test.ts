/** Exercise the bot's real root, conversation, router, and Runling disk recovery together. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { expect, test, vi } from 'vitest';
import { emptyTokenUsage, task, type AgentOptions, type RunlingAgent } from 'runling';
import { RunStore } from '../../runling/src/lib/server/run-store.ts';
import { createChattoRouter, createRouterState } from '../chatto/router.ts';
import type { IncomingMessage } from '../chatto/message.ts';
import { IntakeDecision, IntakeInput } from './classify-intake.ts';
import { createConversation } from './conversation.ts';
import { createThreadWorkflow } from './thread.ts';

test('restores one thread, retains intake and follow-up state, and waits without replaying work', async () => {
  await mkdir(resolve('../../.context'), { recursive: true });
  const directory = await mkdtemp(resolve('../../.context/chattobot-recovery-'));
  const stores: RunStore[] = [];
  const sessions = new Set<string>();
  const prompts: string[] = [];
  const options: AgentOptions[] = [];
  const post = vi.fn(async () => 'bot-post');
  const connection = 'test-connection';
  const first: IncomingMessage = {
    id: 'root',
    roomId: 'room',
    threadRootId: 'root',
    authorId: 'human',
    body: 'Images fail',
    trigger: 'intake',
    connection
  };
  const reply: IncomingMessage = {
    ...first,
    id: 'reply',
    body: 'It happens in Firefox',
    trigger: 'follow-up'
  };
  const classify = vi.fn(async () => ({ action: 'engage' as const, reason: 'relevant' }));
  const classifier = task(
    { name: 'Classifier', input: IntakeInput, result: IntakeDecision },
    classify
  );
  const conversation = createConversation(
    { model: 'test/model', maintainers: new Set() },
    {
      post,
      typing: async () => {},
      acknowledge: async () => {},
      setHandled: async () => {},
      readThread: async () => ({ messages: [], olderOmitted: false }),
      readAttachment: async () => {
        throw new Error('unused');
      }
    },
    async (value) => {
      options.push(value);
      sessions.add(value.sessionFile!);
      return {
        id: 'fixture',
        async runOutcome(_ctx, prompt, runOptions) {
          prompts.push(prompt);
          runOptions?.onText?.('What version do you use?', { hasToolCalls: false });
          return {
            outcome: 'completed',
            summary: 'What version do you use?',
            usage: emptyTokenUsage()
          };
        },
        async run() {
          throw new Error('unused');
        },
        async steer() {
          return false;
        },
        async fork() {
          throw new Error('unused');
        },
        async dispose() {},
        async [Symbol.asyncDispose]() {}
      } satisfies RunlingAgent;
    }
  );
  const workflow = createThreadWorkflow({
    classifier,
    conversation,
    read: async () => ({ body: first.body, attachmentTypes: [] }),
    readContext: async () => [],
    react: async () => {},
    readFollowUp: async () => ({
      boundary: 'bot-post',
      input: { context: [{ role: 'bot', body: 'What browser?' }] },
      messages: [{ message: reply, attachmentTypes: [] }]
    }),
    intervalMs: 5,
    batchMs: 0
  });
  const open = async () => {
    const store = new RunStore(directory);
    stores.push(store);
    await store.init();
    await store.restore({ chattoThread: workflow });
    return store;
  };
  try {
    const original = await open();
    const run = await original.start('chatto', workflow, first, 'source');
    await vi.waitFor(() => expect(post).toHaveBeenCalledTimes(2));
    expect(classify).toHaveBeenCalledTimes(2);
    await original.close();
    const restored = await open();
    await vi.waitFor(() => expect(options).toHaveLength(2));
    expect(sessions.size).toBe(1);
    expect(options[1]!.tools).toContain('setHandled');
    const recovered = restored.sourceRuns('chatto');
    expect(recovered.map((item) => item.id)).toEqual([run.id]);
    const route = createChattoRouter({
      botId: 'bot',
      connection,
      workflow,
      state: createRouterState(),
      recovered
    });
    const start = vi.fn(async () => {
      throw new Error('must reuse the restored run');
    });
    await route({ start }, first);
    await route({ start }, reply);
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(post).toHaveBeenCalledTimes(2);
    expect(classify).toHaveBeenCalledTimes(2);
    await route({ start }, { ...reply, id: 'new', body: 'Version 0.5', trigger: undefined });
    await vi.waitFor(() => expect(post).toHaveBeenCalledTimes(3));
    expect(prompts[2]).toContain('Message ID: new');
    expect(prompts[2]).toContain('reopened after a bot restart');
    expect(prompts[2]).not.toContain('Message ID: root');
    expect(start).not.toHaveBeenCalled();
  } finally {
    for (const store of stores) await store.close();
    for (const path of sessions) await rm(path, { force: true });
    await rm(directory, { recursive: true, force: true });
  }
});
