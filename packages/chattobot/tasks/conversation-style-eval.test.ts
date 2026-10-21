/** Opt-in response-style checks with synthetic thread history. Only the model provider is contacted. */
import { expect, test } from 'vitest';
import { createAgentSession, runWorkflow } from 'runling';
import { createConversation } from './conversation.ts';
import { setting, thinkingSetting } from '../settings.ts';

test.skipIf(process.env.CHATTO_EVAL_STYLE !== '1').each([
  { request: 'Yes, every time.', maxWords: 60, summary: false },
  { request: 'Summarize what we know and the next step.', maxWords: 90, summary: true }
])(
  'concise conversation: $request',
  async ({ request, maxWords, summary }) => {
    const model = setting('CHATTO_AGENT_MODEL');
    if (!model) throw new Error('Set CHATTO_AGENT_MODEL for the opt-in evaluation');
    const posts: string[] = [];
    const conversation = createConversation(
      {
        model,
        thinkingLevel: thinkingSetting('CHATTO_AGENT_THINKING', 'medium'),
        maintainers: new Set()
      },
      {
        post: async (_destination, body) => {
          posts.push(body);
          return `post-${posts.length}`;
        },
        typing: async () => {},
        acknowledge: async () => {},
        setHandled: async () => {},
        readThread: async () => ({
          messages: [
            {
              id: 'report',
              authorId: 'member',
              authorName: 'Reporter',
              role: 'human' as const,
              body: 'In Chatto 0.5 beta on Firefox 145, an unsent draft disappears when I switch rooms. I do not know whether it comes back when I return.'
            },
            {
              id: 'question',
              authorId: 'bot',
              authorName: 'ChattoBot',
              role: 'bot' as const,
              body: 'Does the draft disappear every time you switch rooms?'
            }
          ],
          olderOmitted: false
        }),
        readAttachment: async () => {
          throw new Error('No fixture attachments');
        }
      },
      async (options) =>
        createAgentSession({ ...options, cwd: process.cwd(), tools: ['readThread'] })
    );
    const result = await runWorkflow(conversation, {
      signal: AbortSignal.timeout(90_000),
      input: {
        id: 'answer',
        roomId: 'room',
        threadRootId: 'report',
        authorId: 'member',
        body: request
      }
    });
    if (!result.ok) throw result.error;
    expect(posts).toHaveLength(1);
    const reply = posts[0]!;
    expect(reply.trim().split(/\s+/).length, reply).toBeLessThanOrEqual(maxWords);
    expect(reply.match(/\?/g)?.length ?? 0).toBeLessThanOrEqual(1);
    expect(reply).not.toMatch(/^#{1,6}\s/m);
    expect(reply).not.toMatch(/anything else|let me know if|would you like me to|Paseo|Runling/i);
    if (summary) {
      expect(reply).toMatch(/draft/i);
      expect(reply).toMatch(/Firefox/i);
      expect(reply).toMatch(/0\.5/);
    }
    expect(reply).toMatch(/draft|room/i);
  },
  100_000
);
