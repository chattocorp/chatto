/** Route addressed messages to one conversation run per thread. */
import { createDeliveryTracker } from '@chatto/client';
import { log, type Task, type TSchema } from 'runling';
import type { RunHandle, WebhookRouter } from 'runling/web';
import { ConversationInput, IncomingMessage } from './message.ts';

/** A conversation ends when nobody writes to the bot in its thread for this long. A later
 * message starts a new conversation, which reads the thread again. */
export const IDLE_MS = 60 * 60_000;

interface Thread {
  run: RunHandle<ConversationInput>;
  idle?: ReturnType<typeof setTimeout>;
}

/** Process-local routing state. Keep it across configuration reloads of the same bot. */
export function createRouterState() {
  return { threads: new Map<string, Thread>(), accepted: new Map<string, number>() };
}
export type RouterState = ReturnType<typeof createRouterState>;

export interface RouterOptions {
  botId: string;
  /** The conversation task. Its input and messages are `IncomingMessage`. */
  conversation: Task<typeof IncomingMessage, TSchema, typeof ConversationInput>;
  state: RouterState;
  idleMs?: number;
}

/**
 * Send each message to the run of its thread, or start one. Everyone who addresses the bot in a
 * thread shares its conversation; the conversation checks each GitHub request's author.
 */
export function createChattoRouter({
  botId,
  conversation,
  state,
  idleMs = IDLE_MS
}: RouterOptions): WebhookRouter<IncomingMessage> {
  const { threads } = state;
  const deliveries = createDeliveryTracker({ accepted: state.accepted });

  const forget = (key: string, run: RunHandle<ConversationInput>) => {
    const thread = threads.get(key);
    if (thread?.run !== run) return;
    clearTimeout(thread.idle);
    threads.delete(key);
  };
  // Close the conversation's input after a quiet period. It answers what it has, then ends.
  const touch = (key: string) => {
    const thread = threads.get(key)!;
    clearTimeout(thread.idle);
    thread.idle = setTimeout(() => {
      thread.run.closeInput();
      forget(key, thread.run);
    }, idleMs);
    thread.idle.unref?.();
  };

  const route: WebhookRouter<IncomingMessage> = async (ctx, message) => {
    const delivery = JSON.stringify([botId, message.id]);
    if (deliveries.has(delivery)) return;
    deliveries.accept(delivery);
    const key = JSON.stringify([botId, message.roomId, message.threadRootId]);
    const thread = threads.get(key);

    if (thread) {
      try {
        await thread.run.send(message);
        touch(key);
        return;
      } catch {
        // The run has finished or no longer accepts messages: start a new one.
        forget(key, thread.run);
      }
    }

    const run = await ctx.start(conversation, { input: message });
    threads.set(key, { run });
    touch(key);
    void run.finished.then(() => forget(key, run));
    log.debug('Started a conversation');
  };
  route.label = 'Chatto conversation';
  return route;
}
