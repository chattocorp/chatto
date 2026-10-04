import { expect, test, vi } from 'vitest';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { AgentRunOptions, RunlingAgent } from 'runling/agents';
import { createChattoBot } from './chat.ts';
import { createTurnCompletion } from './turn-completion.ts';
import { completionTool } from '../evaluations/scripted-supervisor.ts';

const result = () => ({ outcome: 'completed' as const, summary: '', usage: emptyTokenUsage() });
const delivery = {
  version: 1 as const,
  id: 'delivery',
  type: 'message.created' as const,
  triggers: ['mention'],
  occurred_at: '2026-10-04T12:00:00Z',
  bot_id: 'bot',
  room_id: 'room',
  thread_root_id: 'root',
  message: { id: 'message', author_id: 'human', body: 'Thanks' }
};

test.each(['text', 'failure', 'throw'])(
  'a successful reaction cannot produce another reply after %s',
  async (continuation) => {
    const post = vi.fn(async (_target: unknown, _text: string) => {});
    const react = vi.fn(async () => {});
    const bot = createChattoBot({
      acknowledge: async () => {},
      typing: async () => {},
      post,
      react,
      timeout: 0,
      readThread: async () => ({ messages: [], olderOmitted: false }),
      web: { tavilyApiKey: 'unused' },
      createAgent: async (options) => {
        const finish = await completionTool(options);
        return {
          steer: async () => false,
          dispose() {},
          async runOutcome(_ctx, _prompt, runOptions) {
            await finish({ kind: 'react', emoji: 'thumbsup' }, runOptions?.signal);
            await expect(finish({ kind: 'reply', text: '👍' })).rejects.toThrow('final action');
            // Trust hooks run before the application gate. They must not post a refusal now.
            await options.trust?.onBlocked?.('implementChatto');
            runOptions?.onText?.('👍');
            if (continuation === 'throw') throw new Error('Unnecessary continuation failed');
            if (continuation === 'failure')
              return { ...result(), outcome: 'failed', summary: 'Failed' };
            return result();
          }
        };
      }
    });
    await bot(createWorkflowContext(), delivery);
    expect(react).toHaveBeenCalledOnce();
    expect(post).not.toHaveBeenCalled();
  }
);

test('a failed reaction allows a model-written reply but no second reaction attempt', async () => {
  const post = vi.fn(async (_target: unknown, _text: string) => {});
  const react = vi.fn(async () => {
    throw new Error('Permission denied');
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    typing: async () => {},
    post,
    react,
    timeout: 0,
    readThread: async () => ({ messages: [], olderOmitted: false }),
    createAgent: async (options) => {
      const finish = await completionTool(options);
      return {
        steer: async () => false,
        dispose() {},
        async runOutcome() {
          await expect(finish({ kind: 'react', emoji: 'heart' })).rejects.toThrow('Permission');
          await expect(finish({ kind: 'react', emoji: 'heart' })).rejects.toThrow('one reaction');
          await finish({ kind: 'reply', text: 'You’re welcome.' });
          return result();
        }
      };
    }
  });
  await bot(createWorkflowContext(), delivery);
  expect(react).toHaveBeenCalledOnce();
  expect(post).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', threadRootId: 'root', inReplyTo: 'message' },
    'You’re welcome.',
    expect.any(AbortSignal)
  );
});

test('missing completion gets one repair, and raw assistant text never reaches delivery', async () => {
  const completion = createTurnCompletion({ react: async () => {} });
  const finish = await completionTool({ extensions: [completion.extension] });
  const onText = vi.fn();
  let turns = 0;
  const runOutcome: RunlingAgent['runOutcome'] = async (_ctx, prompt, options) => {
    turns++;
    options?.onText?.('Unselected answer');
    if (turns === 2) {
      expect(prompt).toContain('Call finishTurn');
      await finish({ kind: 'reply', text: 'Selected answer' });
    }
    return { ...result(), usage: { ...emptyTokenUsage(), input: 3 } };
  };
  const output = await completion
    .wrap({ runOutcome, steer: async () => false })
    .runOutcome(createWorkflowContext(), 'Question', { onText });
  expect(turns).toBe(2);
  expect(output.usage.input).toBe(6);
  expect(onText).toHaveBeenCalledExactlyOnceWith('Selected answer');
});

test('missing completion stops after one repair', async () => {
  const completion = createTurnCompletion({ react: async () => {} });
  const runOutcome = vi.fn(async () => result());
  const output = await completion
    .wrap({ runOutcome, steer: async () => false })
    .runOutcome(createWorkflowContext(), 'Question');
  expect(runOutcome).toHaveBeenCalledTimes(2);
  expect(output.outcome).toBe('failed');
});

test('cancellation after selection never delivers the selected reply or starts repair', async () => {
  const completion = createTurnCompletion({ react: async () => {} });
  const finish = await completionTool({ extensions: [completion.extension] });
  const controller = new AbortController();
  const onText = vi.fn();
  const runOutcome = vi.fn(async () => {
    await finish({ kind: 'reply', text: 'Too late' });
    controller.abort(new Error('Cancelled'));
    return result();
  });
  await expect(
    completion
      .wrap({ runOutcome, steer: async () => false })
      .runOutcome(createWorkflowContext(), 'Question', { signal: controller.signal, onText })
  ).rejects.toThrow('Cancelled');
  expect(runOutcome).toHaveBeenCalledOnce();
  expect(onText).not.toHaveBeenCalled();
});

test('successive feedback and question turns each deliver only their selected action', async () => {
  const post = vi.fn(async (_target: unknown, _text: string) => {});
  const react = vi.fn(async () => {});
  const observed: string[] = [];
  const inputs = [
    'Thanks',
    'Eval note: you answered twice',
    'You answered twice... twice!',
    'What can you do?'
  ];
  let advance!: () => void;
  let finished = new Promise<void>((resolve) => {
    advance = resolve;
  });
  const bot = createChattoBot({
    acknowledge: async () => {},
    typing: async () => {},
    post,
    react,
    timeout: 0.1,
    readThread: async () => ({ messages: [], olderOmitted: false }),
    createAgent: async (options) => {
      const finish = await completionTool(options);
      return {
        steer: async () => false,
        dispose() {},
        async runOutcome(_ctx, prompt, runOptions?: AgentRunOptions) {
          const text = JSON.parse(prompt).message.text as string;
          observed.push(text);
          if (text === 'Thanks') await finish({ kind: 'react', emoji: 'heart' });
          else await finish({ kind: 'reply', text: `Answer ${observed.length}` });
          runOptions?.onText?.('😅');
          advance();
          return result();
        }
      };
    }
  });
  const running = bot(createWorkflowContext(), delivery);
  for (let index = 1; index < inputs.length; index++) {
    await finished;
    finished = new Promise<void>((resolve) => {
      advance = resolve;
    });
    const id = `message-${index}`;
    await bot.route(
      {
        start: async () => {
          throw new Error('Unexpected new conversation');
        }
      },
      {
        ...delivery,
        id,
        message: { ...delivery.message, id, body: inputs[index]! }
      }
    );
  }
  await running;
  expect(observed).toEqual(inputs);
  expect(react).toHaveBeenCalledOnce();
  expect(post.mock.calls).toHaveLength(3);
  expect(post.mock.calls.map((call) => call[1])).toEqual(['Answer 2', 'Answer 3', 'Answer 4']);
});
