/** Botto's thread root and source routing. Pending turns are never replayed. */
import { randomUUID } from 'node:crypto';
import { task, Type, type TaskContext } from 'runling';
import type { EventSourceContext, RunHandle } from 'runling/web';

interface Work {
  runId: string;
  signal: AbortSignal;
  run(ctx: TaskContext): Promise<void>;
  done: ReturnType<typeof Promise.withResolvers<void>>;
  started: boolean;
  failure?: { error: unknown };
}

// Only verified source work gets a private ticket. Console text cannot create work.
// This table retains only unsettled submissions and is never serialized.
const pending = new Map<string, Work>();

const turn = task(
  { name: 'Botto turn', input: Type.String(), result: Type.Null() },
  async (ctx, ticket) => {
    const work = pending.get(ticket);
    if (!work || work.runId !== ctx.run?.id) return null;
    try {
      await work.run(ctx);
      ctx.signal.throwIfAborted();
    } catch (error) {
      work.failure = { error };
      // Private callback errors must not enter the journal.
      throw new Error('Conversation turn failed');
    }
    return null;
  }
);
/** One root per Chatto thread. Inbox admits one turn at a time; the root awaits its child. */
export const conversation = task(
  {
    name: 'Botto conversation',
    input: Type.String(),
    result: Type.Null(),
    steering: Type.String()
  },
  async (ctx) => {
    try {
      for await (const ticket of ctx.steering) {
        const work = pending.get(ticket);
        if (!work || work.started || work.runId !== ctx.run?.id || work.signal.aborted) continue;
        work.started = true;
        try {
          const child = ctx.start(turn, ticket);
          const abort = () => child.cancel(work.signal.reason);
          work.signal.addEventListener('abort', abort, { once: true });
          if (work.signal.aborted) abort();
          try {
            await child.result;
          } finally {
            try {
              await child[Symbol.asyncDispose]();
            } finally {
              work.signal.removeEventListener('abort', abort);
            }
          }
          // Acknowledge only after child cleanup has finished.
          work.done.resolve();
        } catch (error) {
          work.done.reject(work.failure ? work.failure.error : error);
        } finally {
          pending.delete(ticket);
        }
      }
    } finally {
      for (const work of pending.values())
        if (work.runId === ctx.run?.id && !work.started)
          work.done.reject(new Error('Conversation closed before the turn started'));
    }
    return null;
  }
);

/** Inbox serializes calls for each thread, so registration needs no second routing cache. */
async function open(source: EventSourceContext, key: string): Promise<RunHandle> {
  let run: RunHandle | undefined = source
    .runs()
    .find((run) => run.workflow === 'bottoConversation' && run.input === key);
  if (!run)
    await source.dispatch(async (ctx) => {
      run = await ctx.start(conversation, { input: key });
    }, null);
  if (!run) throw new Error('Conversation was not registered');
  return run;
}

/** Called by Inbox, once at a time per thread. Wait for the child and its cleanup.
 * Only this API can admit executable work;
 * arbitrary console steering cannot supply a callback or replay a previous turn. */
export async function runTurn(
  source: EventSourceContext,
  key: string,
  callback: (ctx: TaskContext) => Promise<void>
): Promise<void> {
  source.signal.throwIfAborted();
  const run = await open(source, key);
  source.signal.throwIfAborted();
  const ticket = randomUUID();
  const work: Work = {
    runId: run.id,
    signal: source.signal,
    run: callback,
    done: Promise.withResolvers<void>(),
    started: false
  };
  void work.done.promise.catch(() => {});
  pending.set(ticket, work);
  const abort = () => {
    if (!work.started) work.done.reject(source.signal.reason);
  };
  source.signal.addEventListener('abort', abort, { once: true });
  try {
    await run.steer(ticket);
    await work.done.promise;
  } finally {
    source.signal.removeEventListener('abort', abort);
    pending.delete(ticket);
  }
}
