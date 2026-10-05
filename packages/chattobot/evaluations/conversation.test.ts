/** Opt-in model evaluation of real supervisor turns with synthetic messages and local delivery. */
import { expect, test, vi } from 'vitest';
import { createChannel, createWorkflowContext, Type } from 'runling';
import { agent, defineAgentExtension } from 'runling/agents';
import { conversation } from '../workflows/chat.ts';

// Reference reads are fixtures. Only the selected model provider receives network requests.
vi.mock('../docs.ts', async (original) => ({
  ...(await original<typeof import('../docs.ts')>()),
  docsExtension: defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'fetchPage',
      label: 'Read reference',
      description: 'Read a Chatto reference page.',
      parameters: Type.Object({ url: Type.String() }),
      async execute(_id, { url }) {
        return {
          content: [
            {
              type: 'text',
              text: JSON.stringify({
                url,
                text: 'Chatto supports self-hosting with a standalone binary or Docker Compose. These synthetic references do not compare hosting providers.'
              })
            }
          ],
          details: {}
        };
      }
    });
  })
}));

test.skipIf(!process.env.CHATTO_EVAL_MODEL)(
  'live supervisor conversation delivery',
  async () => {
    const messages = [
      'What can you help me with?',
      'Thanks!',
      'Eval note: you answered twice. Please send just one response.',
      'You answered twice... twice! Please acknowledge this correction briefly in words.',
      'What does self-hosting mean? Answer in one sentence without research.',
      'Please check the Chatto references and tell me how I can self-host it.'
    ];
    const outputs: { turn: number; kind: 'reply' | 'reaction'; text: string }[] = [];
    const inbox = createChannel<string>();
    const controller = new AbortController();
    const ctx = { ...createWorkflowContext(), signal: controller.signal };
    let turn = 0;
    let busy = false;
    let completed = 0;
    let wake: (() => void) | undefined;
    const running = conversation(
      {
        ...ctx,
        inbox,
        emit: async (text) => {
          outputs.push({ turn, kind: 'reply', text });
        }
      },
      messages[0]!,
      {
        model: process.env.CHATTO_EVAL_MODEL,
        createAgent: agent,
        timeout: 5,
        delivery: {
          version: 1,
          id: 'delivery',
          type: 'message.created',
          triggers: ['mention'],
          occurred_at: '2026-10-04T12:00:00Z',
          bot_id: 'bot',
          room_id: 'room',
          thread_root_id: 'root',
          message: { id: 'message-0', author_id: 'human', body: messages[0]! }
        },
        readThread: async () => ({ messages: [], olderOmitted: false }),
        requester: () => 'human',
        currentMessageId: () => `message-${turn}`,
        isAddressed: () => true,
        setReplyContext(text) {
          turn = messages.indexOf(text);
        },
        announce: async (text) => {
          outputs.push({ turn, kind: 'reply', text });
        },
        react: async (_target, emoji) => {
          outputs.push({ turn, kind: 'reaction', text: emoji });
        },
        onBusy(value) {
          if (busy && !value) {
            completed++;
            wake?.();
          }
          busy = value;
        }
      }
    );
    // Keep provider failures observable even while the scenario waits for a completed turn.
    const stopped = running.then(() => {
      throw new Error('Conversation ended before the scenario');
    });
    void stopped.catch(() => {});
    try {
      for (let index = 0; index < messages.length; index++) {
        if (index) await inbox.send(messages[index]!);
        await Promise.race([
          new Promise<void>((resolve) => {
            wake = resolve;
            if (completed > index) resolve();
          }),
          stopped
        ]);
      }
      // Synthetic output is safe to retain for a human review of tone and relevance.
      console.log(JSON.stringify({ model: process.env.CHATTO_EVAL_MODEL, outputs }, null, 2));
      for (let index = 0; index < 5; index++) {
        const delivered = outputs.filter((output) => output.turn === index);
        expect(delivered, `turn ${index}: ${messages[index]}`).toHaveLength(1);
        if (index !== 1) expect(delivered[0]?.kind).toBe('reply');
      }
      const research = outputs.filter((output) => output.turn === 5);
      expect(research).toHaveLength(2); // One acknowledgement, then the answer.
      expect(research.every((output) => output.kind === 'reply')).toBe(true);
      expect(research[1]?.text).toMatch(/https:\/\/(?:dev-)?docs\.chatto\.run/);
    } finally {
      controller.abort(new Error('Evaluation complete'));
      inbox.close();
      await running.catch(() => {});
    }
  },
  180_000
);
