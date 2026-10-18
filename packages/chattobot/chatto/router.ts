/** Deliver messages to one workflow run per thread. The selected workflow owns its behavior. */
import { createDeliveryTracker } from '@chatto/client';
import { log, type Task, type TSchema } from 'runling';
import type { RunHandle, SourceRun, WebhookRouter } from 'runling/web';
import { IncomingMessage } from './message.ts';

interface Thread {
  run: RunHandle<IncomingMessage>;
}

/** Process-local routing state. Keep it across configuration reloads of the same bot. */
export function createRouterState() {
  return {
    threads: new Map<string, Thread>(),
    accepted: new Map<string, number>(),
    routing: new Map<string, Promise<unknown>>()
  };
}
export type RouterState = ReturnType<typeof createRouterState>;

export interface RouterOptions {
  botId: string;
  /** The thread workflow owns intake, participation checks, and the conversation child. */
  workflow: Task<typeof IncomingMessage, TSchema, typeof IncomingMessage>;
  state: RouterState;
  /** Active runs returned by the source host after recovery. */
  recovered?: SourceRun[];
  connection?: string;
}

/**
 * Send each message to the run of its thread, or start one. Everyone who addresses the bot in a
 * thread shares its conversation; the conversation checks each GitHub request's author.
 */
export function createChattoRouter({
  botId,
  workflow,
  state,
  recovered = [],
  connection
}: RouterOptions): WebhookRouter<IncomingMessage> {
  const { threads } = state;
  const deliveries = createDeliveryTracker({ accepted: state.accepted });

  const forget = (key: string, run: RunHandle<IncomingMessage>) => {
    const thread = threads.get(key);

    if (thread?.run !== run) return;
    threads.delete(key);
  };

  for (const run of recovered) {
    const input = run.input as Partial<IncomingMessage> | null;
    if (
      !input ||
      !connection ||
      input.connection !== connection ||
      typeof input.roomId !== 'string' ||
      typeof input.threadRootId !== 'string'
    )
      continue;
    const key = JSON.stringify([botId, input.roomId, input.threadRootId]);
    if (threads.has(key)) continue;
    threads.set(key, { run });
    if (typeof input.id === 'string') deliveries.accept(JSON.stringify([botId, input.id]));
    void run.finished.then(() => forget(key, run));
  }

  const deliver: WebhookRouter<IncomingMessage> = async (ctx, message) => {
    const delivery = JSON.stringify([botId, message.id]);
    if (deliveries.has(delivery)) return;
    deliveries.accept(delivery);
    const key = JSON.stringify([botId, message.roomId, message.threadRootId]);
    const thread = threads.get(key);

    // An existing workflow already owns the thread, including any pending classification.
    if (thread && message.trigger === 'intake') return;

    if (thread) {
      try {
        await thread.run.send(message);
        return;
      } catch {
        // The run has finished or no longer accepts messages: start a new one.
        forget(key, thread.run);
      }
    }

    const run = await ctx.start(workflow, { input: message });
    threads.set(key, { run });
    void run.finished.then(() => forget(key, run));
    log.debug('Started a thread workflow');
  };
  // Concurrent deliveries must not register two runs for one thread.
  const routing = (state.routing ??= new Map());
  const route: WebhookRouter<IncomingMessage> = async (ctx, message) => {
    const key = JSON.stringify([botId, message.roomId, message.threadRootId]);
    const pending = (routing.get(key) ?? Promise.resolve())
      .catch(() => {})
      .then(() => deliver(ctx, message));
    routing.set(key, pending);
    try {
      await pending;
    } finally {
      if (routing.get(key) === pending) routing.delete(key);
    }
  };
  route.label = 'Chatto conversation';
  return route;
}
