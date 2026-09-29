import { expect, test, vi } from 'vitest';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { AddReactionRequest } from '@chatto/api-types/api/v1/reactions_pb';
import { fakeChatto } from './chatto/fake-chatto.ts';
import { createEyesReaction } from './reaction.ts';
import { createChattoBot } from './workflows/chat.ts';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { WebhookContext } from 'runling/web';
import type { Delivery } from './chatto/routing.ts';

const delivery: Delivery = {
  version: 1,
  id: 'delivery',
  type: 'message.created',
  triggers: ['mention'],
  occurred_at: 'now',
  bot_id: 'bot',
  room_id: 'room',
  thread_root_id: 'root',
  message: { id: 'ping', author_id: 'alice', body: 'Hello' }
};

test('reacts to the pinging message, not the thread root', async () => {
  const requests: AddReactionRequest[] = [];
  const { createApi } = fakeChatto({
    viewerId: 'bot',
    routes: (router) =>
      router.service(MessageService, {
        addReaction(request) {
          requests.push(request);
          return { added: true };
        }
      })
  });
  const api = createApi({ serverUrl: 'https://chat.example', apiKey: 'key' });
  await createEyesReaction(api)(delivery);

  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({ roomId: 'room', messageEventId: 'ping', emoji: 'eyes' });
});

test('registers the run before acknowledgement and still replies if reactions fail', async () => {
  const pending = Promise.withResolvers<void>();
  const acknowledge = vi.fn(() => pending.promise);
  const createAgent = vi.fn(async () => ({
    async runOutcome() {
      return { outcome: 'completed' as const, summary: 'Done', usage: emptyTokenUsage() };
    },
    steer: async () => false,
    dispose: () => {}
  }));
  const bot = createChattoBot({
    acknowledge,
    createAgent,
    readThread: async () => ({ messages: [], olderOmitted: false }),
    post: async () => {},
    typing: async () => {},
    timeout: 0
  });
  let running: Promise<unknown> | undefined;
  const start: WebhookContext['start'] = async (root, { input }) => {
    running = Promise.resolve(root(createWorkflowContext(), input));
    return { id: 'run' };
  };
  await bot.route({ start }, delivery);
  expect(acknowledge).toHaveBeenCalledOnce();
  expect(createAgent).not.toHaveBeenCalled();
  pending.reject(new Error('Reaction unavailable'));
  await running;
  await bot.route({ start }, delivery);
  expect(createAgent).toHaveBeenCalledOnce();
  expect(acknowledge).toHaveBeenCalledOnce();
});

test('cancellation during the initial reaction prevents queued messages reaching Pi', async () => {
  const pending = Promise.withResolvers<void>();
  let reactionSignal: AbortSignal | undefined;
  const acknowledged: string[] = [];
  const steer = vi.fn(async () => true);
  const bot = createChattoBot({
    acknowledge: async (message, signal) => {
      acknowledged.push(message.message.id);
      reactionSignal = signal;
      await pending.promise;
    },
    readThread: async () => ({ messages: [], olderOmitted: false }),
    post: async () => {},
    typing: async () => {},
    createAgent: async () => ({
      runOutcome: async () => ({ outcome: 'completed', summary: 'Done', usage: emptyTokenUsage() }),
      steer,
      dispose: () => {}
    })
  });
  const running = bot(createWorkflowContext(), delivery);
  const cancelled = expect(running).rejects.toThrow('Cancelled from Chatto');
  await vi.waitFor(() => expect(acknowledged).toEqual(['ping']));
  const start = async () => {
    throw new Error('Unexpected second run');
  };
  const send = (id: string, body = id) =>
    bot.route(
      { start },
      {
        ...delivery,
        id,
        message: { ...delivery.message, id, body }
      }
    );
  await send('second');
  await send('third');
  await send('cancel', '/cancel');
  expect(reactionSignal?.aborted).toBe(true);
  pending.resolve();
  await cancelled;
  expect(acknowledged).toEqual(['ping']);
  expect(steer).not.toHaveBeenCalled();
});
