/** Own a thread's conversation and decide when ordinary replies need another turn. */
import { setTimeout as delay } from 'node:timers/promises';
import { log, task, Type, type Static, type TaskContext, type TaskInvocation } from 'runling';
import { IncomingMessage, type ConversationInput } from '../chatto/message.ts';
import type { createConversation } from './conversation.ts';
import type { createIntakeClassifier, ReportInput } from './classify-intake.ts';
import type { ReadFollowUp } from '../chatto/follow-up.ts';

/** Wait long enough to collect short replies into one classification and one turn. */
export const FOLLOW_UP_INTERVAL_MS = 90_000;
/** Empty or ignored checks do not extend this lifetime. */
export const IDLE_MS = 60 * 60_000;

interface ThreadOptions {
  classifier: ReturnType<typeof createIntakeClassifier>;
  conversation: ReturnType<typeof createConversation>;
  /** Read current content and verify its human author. Missing or deleted posts are ignored. */
  read: (
    message: IncomingMessage,
    signal: AbortSignal
  ) => Promise<Static<typeof ReportInput> | undefined>;
  readFollowUp: ReadFollowUp;
  /** Injectable clock interval for lifecycle tests. */
  intervalMs?: number;
  idleMs?: number;
}

/** The router only starts this workflow and delivers messages. Classification, silence, and
 * the child conversation belong here. An addressed message can bypass pending classification. */
export function createThreadWorkflow(options: ThreadOptions) {
  const { classifier, conversation, read } = options;
  return task(
    {
      name: 'Chatto thread',
      input: IncomingMessage,
      output: Type.Null(),
      receives: IncomingMessage
    },
    async (ctx, message) => {
      const reader = ctx.inbox[Symbol.asyncIterator]();
      let queued: IteratorResult<IncomingMessage> | undefined;
      const nextMessage = reader.next().then((item) => {
        queued = item;
        return item;
      });
      // Cancellation can fail the inbox while metadata is still being read.
      void nextMessage.catch(() => {});
      let first: IncomingMessage | undefined = message.trigger === 'intake' ? undefined : message;
      let usedNextMessage = false;
      if (!first) {
        const input = await read(message, ctx.signal).catch(() => {
          ctx.signal.throwIfAborted();
          log.error('Report intake could not read the post; no automatic reply sent');
          return undefined;
        });
        if (queued && !queued.done) {
          first = queued.value;
          usedNextMessage = true;
        } else {
          await using classification = input ? ctx.start(classifier, input) : undefined;
          const decision = (classification?.result ?? Promise.resolve({ engage: false })).then(
            (value) => ({ kind: 'decision' as const, engage: value.engage, failed: false }),
            () => {
              ctx.signal.throwIfAborted();
              return { kind: 'decision' as const, engage: false, failed: true };
            }
          );
          const outcome = await Promise.race([
            nextMessage.then(async (item) =>
              item.done ? await decision : { kind: 'addressed' as const, message: item.value }
            ),
            decision
          ]);
          ctx.signal.throwIfAborted();
          if (outcome.kind === 'addressed') {
            first = outcome.message;
            usedNextMessage = true;
          } else {
            if (outcome.failed) log.error('Report intake failed; no automatic reply sent');
            else log.info(outcome.engage ? 'Report intake accepted' : 'Report intake ignored');
            if (outcome.engage && input)
              first = { ...message, body: input.body, trigger: 'intake' };
          }
          // Disposing the classifier cancels it if an addressed message won the race.
        }
      }
      if (!first) return null;

      await using child = ctx.start(conversation, first);
      const monitor = monitorConversation(ctx, child, first, options);
      const finished = child.result.then(() => ({ done: true as const, value: undefined }));
      const forward = async () => {
        let pending = usedNextMessage ? reader.next() : nextMessage;
        while (true) {
          const item = await Promise.race([pending, finished]);
          if (item.done) break;
          await monitor.send(item.value);
          pending = reader.next();
        }
        monitor.stop();
        child.closeInput();
      };
      try {
        await Promise.all([child.result, forward(), monitor.done]);
      } finally {
        monitor.stop();
        if (child.status === 'running') child.cancel();
        await monitor.done;
      }
      return null;
    }
  );
}

/** A poll has no visible effects until classification accepts a batch. One snapshot key skips
 * unchanged replies. Already-delivered messages are context only; the next check reconsiders
 * the remaining human replies since the last bot post when another reply arrives.
 * Direct input or a new turn cancels an obsolete check before it can send a duplicate batch.
 * The same polling loop expires idle conversations after an hour without delivered messages;
 * active turns and background jobs can finish first. */
function monitorConversation(
  ctx: TaskContext<IncomingMessage, unknown>,
  child: TaskInvocation<ReturnType<typeof createConversation>>,
  first: IncomingMessage,
  { classifier, readFollowUp, intervalMs = FOLLOW_UP_INTERVAL_MS, idleMs = IDLE_MS }: ThreadOptions
) {
  const lifetime = new AbortController();
  const signal = AbortSignal.any([ctx.signal, lifetime.signal]);
  let state: 'busy' | 'waiting' | 'idle' = 'busy';
  let lastActivity = performance.now();
  let checked: string | undefined;
  // A turn can finish without a bot post. The post boundary alone cannot deduplicate delivery.
  const delivered = new Set([first.id]);
  let check: AbortController | undefined;
  let classification: TaskInvocation<ReturnType<typeof createIntakeClassifier>> | undefined;
  const interrupt = () => {
    state = 'busy';
    check?.abort();
    classification?.cancel();
  };
  const stop = () => {
    lifetime.abort();
    interrupt();
  };
  const send = async (message: ConversationInput) => {
    interrupt();
    await child.send(message);
    for (const item of Array.isArray(message) ? message : [message]) delivered.add(item.id);
    lastActivity = performance.now();
  };
  const updates = (async () => {
    try {
      for await (const update of child.updates) {
        if (update === 'busy') interrupt();
        else state = update;
      }
    } finally {
      stop();
    }
  })();
  const polls = (async () => {
    while (!signal.aborted) {
      try {
        await delay(intervalMs, undefined, { signal });
        if (state === 'busy') continue;
        if (state === 'idle' && performance.now() - lastActivity >= idleMs) {
          stop();
          child.closeInput();
          break;
        }
        check = new AbortController();
        const checkSignal = AbortSignal.any([signal, check.signal]);
        const snapshot = await readFollowUp(first, checkSignal);
        checkSignal.throwIfAborted();
        if (!snapshot) continue;
        const pending = snapshot.messages.flatMap((item, index) =>
          delivered.has(item.id) ? [] : [index]
        );
        if (!pending.length) continue;
        const messages = pending.map((index) => snapshot.messages[index]!);
        const key = JSON.stringify([snapshot.boundary, messages.map((item) => item.id)]);
        if (key === checked) continue;
        checked = key;
        {
          await using decision = ctx.start(classifier, {
            ...snapshot.input,
            context: [
              ...snapshot.input.context,
              ...snapshot.messages
                .filter((item) => delivered.has(item.id))
                .map(({ body }) => ({ role: 'human' as const, body }))
            ],
            messages: pending.map((index) => snapshot.input.messages[index]!)
          });
          classification = decision;
          const { engage } = await decision.result;
          checkSignal.throwIfAborted();
          if (engage) {
            await send(messages);
            log.info('Thread follow-up accepted');
          } else log.info('Thread follow-up ignored');
        }
      } catch {
        if (!signal.aborted && !check?.signal.aborted)
          log.error('Thread follow-up check failed; no automatic reply sent');
      } finally {
        classification = undefined;
        check = undefined;
      }
    }
  })();
  return { send, stop, done: Promise.all([updates, polls]).then(() => {}) };
}
