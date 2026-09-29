import { createApi } from '@chatto/client';
import { UserService } from '@chatto/client/types';
import { readFile } from 'node:fs/promises';
import { Type, task, step } from 'runling';
import { generateReply } from './agent.ts';
import { createReplySender } from './sender.ts';
import { startTyping } from './typing.ts';

/** The same v1 body is used for direct mentions and direct messages. */
export const webhookInput = Type.Object({
  version: Type.Literal(1),
  id: Type.String({ minLength: 1 }),
  type: Type.Literal('message.created'),
  triggers: Type.Array(Type.Union([Type.Literal('mention'), Type.Literal('direct_message')]), {
    minItems: 1
  }),
  occurred_at: Type.String(),
  bot_id: Type.String({ minLength: 1 }),
  room_id: Type.String({ minLength: 1 }),
  thread_root_id: Type.Union([Type.String({ minLength: 1 }), Type.Null()]),
  message: Type.Object({
    id: Type.String({ minLength: 1 }),
    author_id: Type.String({ minLength: 1 }),
    body: Type.String()
  })
});

interface BotConfig {
  serverUrl: string;
  apiKey: string;
}

/** Read credentials at execution time. Never include them in workflow output. */
async function readEnvironment(): Promise<BotConfig> {
  const serverUrl = process.env.CHATTO_RUNLING_SERVER_URL;
  const keyFile = process.env.CHATTO_RUNLING_API_KEY_FILE;

  if (!serverUrl || !keyFile) {
    throw new Error('Set CHATTO_RUNLING_SERVER_URL and CHATTO_RUNLING_API_KEY_FILE');
  }

  const apiKey = (await readFile(keyFile, 'utf8')).trim();
  if (!apiKey) throw new Error('The bot API key file is empty');

  return { serverUrl, apiKey };
}

/** Timeout of one Chatto request, like the bot client's own requests. */
const REQUEST_TIMEOUT_MS = 10_000;

/** Build the reply workflow. Injectable I/O allows tests without a live server. */
export function createReplyWorkflow(
  loadConfig: () => Promise<BotConfig> = readEnvironment,
  request: typeof fetch = globalThis.fetch,
  answer: typeof generateReply = generateReply
) {
  return task(
    {
      name: 'Reply to Chatto',
      input: webhookInput,
      output: Type.Object({
        deliveryId: Type.String(),
        status: Type.Union([Type.Literal('replied'), Type.Literal('skipped')]),
        replyId: Type.Optional(Type.String())
      })
    },
    async (r, input) => {
      const { serverUrl, apiKey } = await loadConfig();
      // Confirm the configured credentials belong to the intended bot. Step
      // results keep the JSON shape of earlier versions, so resumed runs can
      // replay journaled results.
      const viewer = await step('Check bot identity', async () => {
        const identity = createApi({ serverUrl, apiKey, fetch: request });
        const { viewerId: id } = await identity.ready({ signal: r.signal });
        return { user: { profile: { id } } };
      });
      if (viewer.user?.profile?.id !== input.bot_id) {
        throw new Error('The webhook bot does not match the API key');
      }
      // The viewer is known now: requests need no further viewer reads, also
      // when a resumed run replays the identity check from the journal.
      const api = createApi({ serverUrl, apiKey, fetch: request, viewerId: input.bot_id });

      // Ignore bot authors to prevent automatic reply loops.
      if (input.message.author_id === input.bot_id) {
        return { deliveryId: input.id, status: 'skipped' as const };
      }

      const author = await step('Check message author', async () => {
        const response = await api
          .service(UserService)
          .getUser(
            { target: { case: 'userId', value: input.message.author_id } },
            { signal: r.signal, timeoutMs: REQUEST_TIMEOUT_MS }
          );
        const user = response.user?.user;
        const bot = user?.bot ? { ownerUserId: user.bot.ownerUserId } : undefined;
        return { user: user ? { user: { bot } } : undefined };
      });
      if (!author.user?.user) {
        throw new Error('The message author is unavailable');
      }

      if (author.user.user.bot) {
        return { deliveryId: input.id, status: 'skipped' as const };
      }

      // The agent needs conversation text, not server event or author IDs.
      const readThread = async () =>
        (
          await api.readThread(
            {
              roomId: input.room_id,
              threadRootId: input.thread_root_id ?? input.message.id
            },
            { signal: r.signal }
          )
        ).messages.map(({ fromViewer, body }) => ({
          role: fromViewer ? ('bot' as const) : ('human' as const),
          body
        }));

      // Start typing before loading context, and keep it active during composition.
      const stopTyping = await step('Start typing', () =>
        startTyping(() =>
          api.refreshTyping(
            {
              roomId: input.room_id,
              threadRootId: input.thread_root_id ?? input.message.id
            },
            { signal: r.signal }
          )
        )
      );

      // Each run owns one final-answer or error-notification attempt.
      const sender = createReplySender((text, stepName) =>
        step(stepName, async () => {
          const { id } = await api.createMessage(
            {
              roomId: input.room_id,
              threadRootId: input.thread_root_id ?? input.message.id
            },
            text,
            { signal: r.signal, inReplyTo: input.message.id }
          );
          return id;
        })
      );

      try {
        const thread = await step('Load thread', readThread);

        await step('Compose reply', () =>
          answer(r, {
            message: input.message.body,
            thread,
            readThread,
            sender
          })
        );

        if (!sender.id) {
          throw new Error('The agent did not send a final answer');
        }
      } catch (error) {
        // Notify the user if possible, but keep the run marked as failed.
        await sender.notifyFailure();
        throw error;
      } finally {
        stopTyping();
      }

      return {
        deliveryId: input.id,
        status: 'replied' as const,
        replyId: sender.id
      };
    }
  );
}

export default createReplyWorkflow();
